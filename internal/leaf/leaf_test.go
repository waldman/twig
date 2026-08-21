package leaf

import (
	"os"
	"path/filepath"
	"strings"
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
	for _, key := range []string{"modules", "remotes", "vars"} {
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
remotes:
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
remotes:
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

// helpers for LoadInherited tests

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

func TestLoadInherited_noFiles(t *testing.T) {
	root, seg := makeInfraTree(t)
	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if len(inh.Vars) != 0 || len(inh.RemoteState) != 0 || len(inh.ModuleDefaults) != 0 {
		t.Errorf("expected all sections empty, got %+v", inh)
	}
}

func TestLoadInherited_singleLevel(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "vars:\n  cost_center: engineering\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if inh.Vars["cost_center"] != "engineering" {
		t.Errorf("expected cost_center=engineering, got %v", inh.Vars["cost_center"])
	}
	if got := inh.VarsOrigins["cost_center"]; !strings.HasSuffix(got, "infra/aws/vars.yaml") {
		t.Errorf("expected origin infra/aws/vars.yaml, got %q", got)
	}
}

func TestLoadInherited_lowerWins(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra"), "vars:\n  tier: base\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1", "production"), "vars:\n  tier: prod-override\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if inh.Vars["tier"] != "prod-override" {
		t.Errorf("expected lower level to win, got %v", inh.Vars["tier"])
	}
	if got := inh.VarsOrigins["tier"]; !strings.HasSuffix(got, "production/vars.yaml") {
		t.Errorf("expected origin to be the production-level file, got %q", got)
	}
}

func TestLoadInherited_mergeAcrossLevels(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra"), "vars:\n  cost_center: engineering\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1"), "vars:\n  vpc_id: vpc-abc123\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if inh.Vars["cost_center"] != "engineering" {
		t.Errorf("expected cost_center from root level, got %v", inh.Vars["cost_center"])
	}
	if inh.Vars["vpc_id"] != "vpc-abc123" {
		t.Errorf("expected vpc_id from region level, got %v", inh.Vars["vpc_id"])
	}
}

func TestLoadInherited_reservedVarRejected(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "vars:\n  region: us-west-2\n")

	_, err := LoadInherited(root, seg)
	if err == nil {
		t.Fatal("expected error for reserved var name, got nil")
	}
}

func TestLoadInherited_unknownTopLevelKeyRejected(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "cost_center: engineering\n")

	_, err := LoadInherited(root, seg)
	if err == nil {
		t.Fatal("expected error for unknown top-level key, got nil")
	}
}

func TestLoadInherited_emptyFile(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatalf("empty vars.yaml should be accepted, got: %v", err)
	}
	if len(inh.Vars) != 0 {
		t.Errorf("expected empty vars, got %v", inh.Vars)
	}
}

func TestLoadInherited_emptyVarsBlock(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), "vars:\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatalf("empty vars: block should be accepted, got: %v", err)
	}
	if len(inh.Vars) != 0 {
		t.Errorf("expected empty vars, got %v", inh.Vars)
	}
}

func TestLoadInherited_remoteStateInherited(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1"),
		"remotes:\n  network: infra/aws/myprofile/us-east-1/base/network/main.yaml\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if got := inh.RemoteState["network"]; got != "infra/aws/myprofile/us-east-1/base/network/main.yaml" {
		t.Errorf("expected inherited remotes network alias, got %q", got)
	}
	if got := inh.RemoteStateOrigins["network"]; !strings.HasSuffix(got, "us-east-1/vars.yaml") {
		t.Errorf("expected origin to be the region-level file, got %q", got)
	}
}

func TestLoadInherited_remoteStateLowerWins(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra"),
		"remotes:\n  network: infra/aws/myprofile/us-east-1/base/high/main.yaml\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile", "us-east-1"),
		"remotes:\n  network: infra/aws/myprofile/us-east-1/base/low/main.yaml\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inh.RemoteState["network"], "/low/") {
		t.Errorf("expected lower level to win, got %q", inh.RemoteState["network"])
	}
}

func TestLoadInherited_remoteStateReservedAliasRejected(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"remotes:\n  vars: infra/aws/myprofile/us-east-1/base/x/main.yaml\n")

	_, err := LoadInherited(root, seg)
	if err == nil {
		t.Fatal("expected error for reserved alias in inherited remotes, got nil")
	}
}

func TestLoadInherited_moduleDefaultsSingleSource(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"module_defaults:\n  aws/5/vpc:\n    vpc_cidr_block: 10.0.0.0/16\n    vpc_availability_zones: 2\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	got := inh.ModuleDefaults["aws/5/vpc"]
	if got["vpc_cidr_block"] != "10.0.0.0/16" {
		t.Errorf("expected vpc_cidr_block=10.0.0.0/16, got %v", got["vpc_cidr_block"])
	}
	if got["vpc_availability_zones"] != 2 {
		t.Errorf("expected vpc_availability_zones=2, got %v", got["vpc_availability_zones"])
	}
	if o := inh.ModuleDefaultsOrigins["aws/5/vpc"]["vpc_cidr_block"]; !strings.HasSuffix(o, "aws/vars.yaml") {
		t.Errorf("expected origin infra/aws/vars.yaml, got %q", o)
	}
}

func TestLoadInherited_moduleDefaultsMergeAcrossLevels(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"module_defaults:\n  aws/5/vpc:\n    vpc_cidr_block: 10.0.0.0/16\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile"),
		"module_defaults:\n  aws/5/vpc:\n    vpc_availability_zones: 3\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	got := inh.ModuleDefaults["aws/5/vpc"]
	if got["vpc_cidr_block"] != "10.0.0.0/16" || got["vpc_availability_zones"] != 3 {
		t.Errorf("expected both keys merged from different levels, got %v", got)
	}
}

func TestLoadInherited_moduleDefaultsLowerWinsPerKey(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"module_defaults:\n  aws/5/vpc:\n    vpc_cidr_block: 10.0.0.0/16\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile"),
		"module_defaults:\n  aws/5/vpc:\n    vpc_cidr_block: 10.99.0.0/16\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if got := inh.ModuleDefaults["aws/5/vpc"]["vpc_cidr_block"]; got != "10.99.0.0/16" {
		t.Errorf("expected lower level to win per key, got %v", got)
	}
}

func TestLoadInherited_moduleDefaultsReservedVarRejected(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"module_defaults:\n  aws/5/vpc:\n    region: us-west-2\n")

	_, err := LoadInherited(root, seg)
	if err == nil {
		t.Fatal("expected error for reserved var inside module_defaults, got nil")
	}
}

func TestLoadInherited_allSectionsCoexist(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"), `vars:
  vpn_cidr: 10.30.0.0/16
remotes:
  network: infra/aws/myprofile/us-east-1/base/network/main.yaml
module_defaults:
  aws/5/ec2:
    ec2_instance_type: t3.small
`)

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if inh.Vars["vpn_cidr"] != "10.30.0.0/16" {
		t.Errorf("vars missing: %v", inh.Vars)
	}
	if inh.RemoteState["network"] == "" {
		t.Errorf("remotes missing: %v", inh.RemoteState)
	}
	if inh.ModuleDefaults["aws/5/ec2"]["ec2_instance_type"] != "t3.small" {
		t.Errorf("module_defaults missing: %v", inh.ModuleDefaults)
	}
}

func TestLoad_envFiles(t *testing.T) {
	path := writeLeaf(t, `
env_files:
  - ~/.azure/profiles/myprofile
  - /etc/twig/extra.env
modules:
  web:
    source: azure/3/app
`)
	l, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.EnvFiles) != 2 {
		t.Fatalf("want 2 env_files, got %d: %v", len(l.EnvFiles), l.EnvFiles)
	}
	if l.EnvFiles[0] != "~/.azure/profiles/myprofile" {
		t.Errorf("unexpected EnvFiles[0]: %q", l.EnvFiles[0])
	}
}

func TestLoad_envFiles_empty(t *testing.T) {
	path := writeLeaf(t, "modules:\n  web:\n    source: azure/3/app\n")
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.EnvFiles != nil {
		t.Errorf("expected nil EnvFiles, got %v", l.EnvFiles)
	}
}

func TestLoadInherited_envFilesConcatenated(t *testing.T) {
	root, seg := makeInfraTree(t)
	writeVarsYAML(t, filepath.Join(root, "infra", "aws"),
		"env_files:\n  - ~/.azure/base.env\n")
	writeVarsYAML(t, filepath.Join(root, "infra", "aws", "myprofile"),
		"env_files:\n  - ~/.azure/profiles/myprofile\n")

	inh, err := LoadInherited(root, seg)
	if err != nil {
		t.Fatal(err)
	}
	if len(inh.EnvFiles) != 2 {
		t.Fatalf("want 2 env_files, got %d: %v", len(inh.EnvFiles), inh.EnvFiles)
	}
	// shallowest first
	if inh.EnvFiles[0] != "~/.azure/base.env" {
		t.Errorf("unexpected order: %v", inh.EnvFiles)
	}
	if inh.EnvFiles[1] != "~/.azure/profiles/myprofile" {
		t.Errorf("unexpected order: %v", inh.EnvFiles)
	}
}
