package leaf

import (
	"os"
	"path/filepath"
	"testing"
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
