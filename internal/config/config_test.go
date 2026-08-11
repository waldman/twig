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
