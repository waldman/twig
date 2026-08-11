package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "twig.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_valid(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
modules_path: ./modules
backend:
  bucket: my-state
  region: us-east-1
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ModulesPath != "./modules" {
		t.Errorf("ModulesPath = %q, want %q", cfg.ModulesPath, "./modules")
	}
	if cfg.Backend["bucket"] != "my-state" {
		t.Errorf("Backend[bucket] = %q, want %q", cfg.Backend["bucket"], "my-state")
	}
	if cfg.Root != root {
		t.Errorf("Root = %q, want %q", cfg.Root, root)
	}
}

func TestLoad_walksUp(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
modules_path: ./modules
backend:
  bucket: my-state
  region: us-east-1
`)
	deep := filepath.Join(root, "infra", "aws", "production", "services")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(deep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Root != root {
		t.Errorf("Root = %q, want %q", cfg.Root, root)
	}
}

func TestLoad_backendKeyForbidden(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
modules_path: ./modules
backend:
  bucket: my-state
  region: us-east-1
  key: forbidden
`)
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for backend.key, got nil")
	}
}

func TestLoad_missingModulesPath(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
backend:
  bucket: my-state
  region: us-east-1
`)
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for missing modules_path, got nil")
	}
}

func TestLoad_missingBackend(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
modules_path: ./modules
`)
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for missing backend, got nil")
	}
}

func TestLoad_notFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when twig.yaml not found, got nil")
	}
}

func TestModulesRoot_envOverride(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `
modules_path: ./modules
backend:
  bucket: my-state
  region: us-east-1
`)
	cfg, _ := Load(root)

	t.Setenv("TWIG_MODULES_PATH", "/override/path")
	got := cfg.ModulesRoot()
	if got != "/override/path" {
		t.Errorf("ModulesRoot = %q, want /override/path", got)
	}
}

func TestIsGitSource(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"./modules", false},
		{"/abs/path/modules", false},
		{"../relative/modules", false},
		{"github.com/org/repo//modules", true},
		{"https://github.com/org/repo.git//modules", true},
		{"git@github.com:org/repo.git//modules", true},
		{"git::https://github.com/org/repo.git//modules", true},
		{"gitlab.com/org/repo//modules", true},
		{"bitbucket.org/org/repo//modules", true},
	}

	for _, tc := range cases {
		cfg := &Config{ModulesPath: tc.path}
		if got := cfg.IsGitSource(); got != tc.want {
			t.Errorf("IsGitSource(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestModuleSource_local(t *testing.T) {
	cfg := &Config{ModulesPath: "/modules", Root: "/project"}
	got := cfg.ModuleSource("aws/5/vpc")
	if got != "/modules/aws/5/vpc" {
		t.Errorf("ModuleSource = %q, want /modules/aws/5/vpc", got)
	}
}

func TestModuleSource_gitBareHostname(t *testing.T) {
	cfg := &Config{
		ModulesPath: "github.com/waldman/terraform//modules",
		ModulesRef:  "v1.0.0",
		Root:        "/project",
	}
	got := cfg.ModuleSource("aws/5/vpc")
	want := "git::https://github.com/waldman/terraform.git//modules/aws/5/vpc?ref=v1.0.0"
	if got != want {
		t.Errorf("ModuleSource = %q, want %q", got, want)
	}
}

func TestModuleSource_gitHTTPS(t *testing.T) {
	cfg := &Config{
		ModulesPath: "https://github.com/waldman/terraform.git//modules",
		ModulesRef:  "v2.3.1",
		Root:        "/project",
	}
	got := cfg.ModuleSource("aws/5/s3-bucket")
	want := "git::https://github.com/waldman/terraform.git//modules/aws/5/s3-bucket?ref=v2.3.1"
	if got != want {
		t.Errorf("ModuleSource = %q, want %q", got, want)
	}
}

func TestModuleSource_gitSSH(t *testing.T) {
	cfg := &Config{
		ModulesPath: "git@github.com:waldman/terraform.git//modules",
		Root:        "/project",
	}
	got := cfg.ModuleSource("aws/5/vpc")
	want := "git::git@github.com:waldman/terraform.git//modules/aws/5/vpc"
	if got != want {
		t.Errorf("ModuleSource = %q, want %q", got, want)
	}
}

func TestModuleSource_gitNoSubdir(t *testing.T) {
	cfg := &Config{
		ModulesPath: "github.com/waldman/terraform-modules",
		ModulesRef:  "v1.0.0",
		Root:        "/project",
	}
	got := cfg.ModuleSource("aws/5/vpc")
	want := "git::https://github.com/waldman/terraform-modules.git//aws/5/vpc?ref=v1.0.0"
	if got != want {
		t.Errorf("ModuleSource = %q, want %q", got, want)
	}
}

func TestModuleSource_gitNoRef(t *testing.T) {
	cfg := &Config{
		ModulesPath: "github.com/waldman/terraform//modules",
		Root:        "/project",
	}
	got := cfg.ModuleSource("aws/5/vpc")
	want := "git::https://github.com/waldman/terraform.git//modules/aws/5/vpc"
	if got != want {
		t.Errorf("ModuleSource = %q, want %q", got, want)
	}
}
