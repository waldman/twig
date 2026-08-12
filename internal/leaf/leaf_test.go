package leaf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/waldman/twig/internal/pathparse"
)

func writeLeaf(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "my-component.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_valid(t *testing.T) {
	path := writeLeaf(t, `
modules:
  s3_bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: my-bucket

  dynamodb:
    source: aws/5/dynamodb
    vars:
      dynamodb_hash_key: node
      dynamodb_ttl_enabled: true
`)
	l, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.ModuleKeys) != 2 {
		t.Fatalf("want 2 modules, got %d", len(l.ModuleKeys))
	}
	// order preserved
	if l.ModuleKeys[0] != "s3_bucket" || l.ModuleKeys[1] != "dynamodb" {
		t.Errorf("unexpected order: %v", l.ModuleKeys)
	}
	if l.Modules["s3_bucket"].Source != "aws/5/s3-bucket" {
		t.Errorf("unexpected source: %q", l.Modules["s3_bucket"].Source)
	}
}

func TestLoad_noVars(t *testing.T) {
	path := writeLeaf(t, `
modules:
  iam_cicd:
    source: aws/5/iam-user
`)
	l, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.Modules["iam_cicd"].Vars != nil && len(l.Modules["iam_cicd"].Vars) != 0 {
		t.Errorf("expected empty vars, got %v", l.Modules["iam_cicd"].Vars)
	}
}

func TestLoad_reservedVar(t *testing.T) {
	reserved := []string{"cloud", "profile", "region", "environment", "class", "component", "module"}
	for _, v := range reserved {
		t.Run(v, func(t *testing.T) {
			path := writeLeaf(t, "modules:\n  foo:\n    source: aws/5/x\n    vars:\n      "+v+": bad\n")
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected error for reserved var %q, got nil", v)
			}
		})
	}
}

func TestLoad_reservedInstanceKey(t *testing.T) {
	for _, key := range []string{"module", "remote", "var"} {
		t.Run(key, func(t *testing.T) {
			path := writeLeaf(t, "modules:\n  "+key+":\n    source: aws/5/x\n")
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected error for reserved instance key %q, got nil", key)
			}
		})
	}
}

func TestLoad_missingSource(t *testing.T) {
	path := writeLeaf(t, `
modules:
  foo:
    vars:
      key: val
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestLoad_fileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/component.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_remoteState_valid(t *testing.T) {
	path := writeLeaf(t, `
remote_state:
  vpc: infra/aws/waldman/us-east-1/base/vpc/main.yaml
  keys: infra/aws/waldman/us-east-1/base/ec2/default-key-pair.yaml

modules:
  ec2:
    source: aws/5/ec2
    vars:
      ec2_vpc_id: ${vpc.vpc_id}
`)
	l, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.RemoteStateKeys) != 2 {
		t.Fatalf("want 2 remote state aliases, got %d", len(l.RemoteStateKeys))
	}
	if l.RemoteStateKeys[0] != "vpc" || l.RemoteStateKeys[1] != "keys" {
		t.Errorf("unexpected remote state key order: %v", l.RemoteStateKeys)
	}
	if l.RemoteState["vpc"] != "infra/aws/waldman/us-east-1/base/vpc/main.yaml" {
		t.Errorf("unexpected path: %q", l.RemoteState["vpc"])
	}
}

func TestLoad_remoteState_aliasConflict(t *testing.T) {
	path := writeLeaf(t, `
remote_state:
  ec2: infra/aws/waldman/us-east-1/base/vpc/main.yaml

modules:
  ec2:
    source: aws/5/ec2
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for alias/module key conflict, got nil")
	}
}

// helpers for LoadInheritedVars tests

func makeInfraTree(t *testing.T) (root string, seg *pathparse.Segments) {
	t.Helper()
	root = t.TempDir()
	seg = &pathparse.Segments{
		Cloud: "aws", Profile: "myprofile", Region: "us-east-1",
		Environment: "production", Class: "services", Component: "app",
	}
	return root, seg
}

func writeVarsYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vars.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadInheritedVars_noFiles(t *testing.T) {
	root, seg := makeInfraTree(t)
	vars, err := LoadInheritedVars(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 0 {
		t.Errorf("expected empty map, got %v", vars)
	}
}

func TestLoadInheritedVars_singleLevel(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "cost_center: engineering\n")

	vars, err := LoadInheritedVars(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if vars["cost_center"] != "engineering" {
		t.Errorf("expected cost_center=engineering, got %v", vars["cost_center"])
	}
}

func TestLoadInheritedVars_lowerWins(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra"), "tier: base\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1", "production"), "tier: prod-override\n")

	vars, err := LoadInheritedVars(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if vars["tier"] != "prod-override" {
		t.Errorf("expected lower level to win, got %v", vars["tier"])
	}
}

func TestLoadInheritedVars_mergeAcrossLevels(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra"), "cost_center: engineering\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1"), "vpc_id: vpc-abc123\n")

	vars, err := LoadInheritedVars(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if vars["cost_center"] != "engineering" {
		t.Errorf("expected cost_center from root level, got %v", vars["cost_center"])
	}
	if vars["vpc_id"] != "vpc-abc123" {
		t.Errorf("expected vpc_id from region level, got %v", vars["vpc_id"])
	}
}

func TestLoadInheritedVars_reservedVarRejected(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "region: us-west-2\n")

	_, err := LoadInheritedVars(root, seg)
	if err == nil {
		t.Fatal("expected error for reserved var name, got nil")
	}
}
