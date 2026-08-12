package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldman/twig/internal/config"
	"github.com/waldman/twig/internal/leaf"
	"github.com/waldman/twig/internal/pathparse"
)

var testSeg = &pathparse.Segments{
	Cloud:       "aws",
	Profile:     "waldman",
	Region:      "us-east-1",
	Environment: "production",
	Class:       "services",
	Component:   "my-app",
}

var testCfg *config.Config

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "twig-generate-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	// providers.yaml for the aws cloud context used by most tests
	awsDir := filepath.Join(root, "infra", "aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		panic(err)
	}
	awsProviders := `aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region: "${region}"
datadog:
  source: datadog/datadog
  config:
    api_key: "test-key"
`
	if err := os.WriteFile(filepath.Join(awsDir, "providers.yaml"), []byte(awsProviders), 0o644); err != nil {
		panic(err)
	}

	// providers.yaml for the gcp cloud context
	gcpDir := filepath.Join(root, "infra", "gcp")
	if err := os.MkdirAll(gcpDir, 0o755); err != nil {
		panic(err)
	}
	gcpProviders := `gcp:
  source: hashicorp/google
  config:
    project: "${profile}"
    region: "${region}"
`
	if err := os.WriteFile(filepath.Join(gcpDir, "providers.yaml"), []byte(gcpProviders), 0o644); err != nil {
		panic(err)
	}

	testCfg = &config.Config{
		ModulesPath: "/modules",
		Backend:     config.Backend{"bucket": "my-state", "region": "us-east-1"},
		Root:        root,
	}

	os.Exit(m.Run())
}

func makeLeaf(keys []string, modules map[string]*leaf.Module) *leaf.Leaf {
	return &leaf.Leaf{ModuleKeys: keys, Modules: modules}
}

func TestGenerate_pathVarsInjected(t *testing.T) {
	l := makeLeaf([]string{"s3_bucket"}, map[string]*leaf.Module{
		"s3_bucket": {Source: "aws/5/s3-bucket"},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`cloud       = "aws"`,
		`profile     = "waldman"`,
		`region      = "us-east-1"`,
		`environment = "production"`,
		`class       = "services"`,
		`component   = "my-app"`,
		`module      = "s3_bucket"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerate_moduleVarIsInstanceKey(t *testing.T) {
	l := makeLeaf([]string{"iam_cicd", "iam_infra"}, map[string]*leaf.Module{
		"iam_cicd":  {Source: "aws/5/iam-user"},
		"iam_infra": {Source: "aws/5/iam-user"},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, `module      = "iam_cicd"`) {
		t.Error("missing module = iam_cicd")
	}
	if !strings.Contains(out, `module      = "iam_infra"`) {
		t.Error("missing module = iam_infra")
	}
}

func TestGenerate_crossRefPure(t *testing.T) {
	l := makeLeaf([]string{"bucket", "policy"}, map[string]*leaf.Module{
		"bucket": {Source: "aws/5/s3-bucket"},
		"policy": {Source: "aws/5/iam-policy", Vars: map[string]interface{}{
			"target_arn": "${module.bucket.bucket_arn}",
		}},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	// pure ref → unquoted
	if !strings.Contains(out, "target_arn = module.bucket.bucket_arn") {
		t.Errorf("expected unquoted cross-ref, got:\n%s", out)
	}
}

func TestGenerate_crossRefMixed(t *testing.T) {
	l := makeLeaf([]string{"bucket", "policy"}, map[string]*leaf.Module{
		"bucket": {Source: "aws/5/s3-bucket"},
		"policy": {Source: "aws/5/iam-policy", Vars: map[string]interface{}{
			"wildcard_arn": "${module.bucket.bucket_arn}/*",
		}},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	// mixed → interpolated string
	if !strings.Contains(out, `wildcard_arn = "${module.bucket.bucket_arn}/*"`) {
		t.Errorf("expected interpolated cross-ref, got:\n%s", out)
	}
}

func TestGenerate_crossRefUndeclared(t *testing.T) {
	l := makeLeaf([]string{"policy"}, map[string]*leaf.Module{
		"policy": {Source: "aws/5/iam-policy", Vars: map[string]interface{}{
			"arn": "${module.nonexistent.arn}",
		}},
	})

	_, err := Generate(testCfg, testSeg, l)
	if err == nil {
		t.Fatal("expected error for undeclared cross-ref, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention undeclared instance, got: %v", err)
	}
}

func TestGenerate_backendKey(t *testing.T) {
	l := makeLeaf([]string{"s3_bucket"}, map[string]*leaf.Module{
		"s3_bucket": {Source: "aws/5/s3-bucket"},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	want := `"infra/aws/waldman/us-east-1/production/services/my-app/terraform.tfstate"`
	if !strings.Contains(out, want) {
		t.Errorf("backend key missing %s in:\n%s", want, out)
	}
}

func TestGenerate_declarationOrder(t *testing.T) {
	l := makeLeaf([]string{"zzz", "aaa"}, map[string]*leaf.Module{
		"zzz": {Source: "aws/5/s3-bucket"},
		"aaa": {Source: "aws/5/dynamodb"},
	})

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	zIdx := strings.Index(out, `module "zzz"`)
	aIdx := strings.Index(out, `module "aaa"`)
	if zIdx == -1 || aIdx == -1 {
		t.Fatal("module blocks not found")
	}
	if zIdx > aIdx {
		t.Error("declaration order not preserved: zzz should appear before aaa")
	}
}

func TestGenerate_gitModuleSource(t *testing.T) {
	gitCfg := &config.Config{
		ModulesPath: "github.com/waldman/terraform//modules",
		ModulesRef:  "v1.0.0",
		Backend:     config.Backend{"bucket": "my-state", "region": "us-east-1"},
		Root:        testCfg.Root,
	}
	l := makeLeaf([]string{"vpc"}, map[string]*leaf.Module{
		"vpc": {Source: "aws/5/vpc"},
	})

	out, err := Generate(gitCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	want := `source = "git::https://github.com/waldman/terraform.git//modules/aws/5/vpc?ref=v1.0.0"`
	if !strings.Contains(out, want) {
		t.Errorf("expected git source URL, got:\n%s", out)
	}
}

func TestStringToHCL(t *testing.T) {
	resolve := func(ns, key, field string) string { return "module." + key + "." + field }
	inherited := map[string]interface{}{"vpn_cidr": "10.30.0.0/16"}

	cases := []struct {
		input string
		want  string
	}{
		{"plain string", `"plain string"`},
		{"${module.x.arn}", "module.x.arn"},
		{"${module.x.arn}/*", `"${module.x.arn}/*"`},
		{"prefix-${module.x.arn}-suffix", `"prefix-${module.x.arn}-suffix"`},
		{"${var.vpn_cidr}", `"10.30.0.0/16"`},
		{"cidr:${var.vpn_cidr}/32", `"cidr:10.30.0.0/16/32"`},
	}

	for _, tc := range cases {
		got := stringToHCL(tc.input, resolve, inherited)
		if got != tc.want {
			t.Errorf("stringToHCL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGenerate_remoteStateBlocks(t *testing.T) {
	l := &leaf.Leaf{
		ModuleKeys:      []string{"ec2"},
		Modules:         map[string]*leaf.Module{"ec2": {Source: "aws/5/ec2"}},
		RemoteStateKeys: []string{"vpc"},
		RemoteState:     map[string]string{"vpc": "infra/aws/waldman/us-east-1/base/vpc/test.yaml"},
	}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`data "terraform_remote_state" "vpc"`,
		`backend = "s3"`,
		`key         = "infra/aws/waldman/us-east-1/base/vpc/test/terraform.tfstate"`,
		`bucket      = "my-state"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestGenerate_requiredProviders(t *testing.T) {
	l := makeLeaf([]string{"vpc"}, map[string]*leaf.Module{"vpc": {Source: "aws/5/vpc"}})
	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`required_providers`,
		`aws = {`,
		`source  = "hashicorp/aws"`,
		`version = "~> 5.0"`,
		`provider "aws"`,
		`profile = "waldman"`,
		`region = "us-east-1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestGenerate_gcpProviderHCLName(t *testing.T) {
	gcpSeg := &pathparse.Segments{
		Cloud: "gcp", Profile: "my-project", Region: "us-central1",
		Environment: "dev", Class: "compute", Component: "web",
	}
	gcpCfg := &config.Config{
		ModulesPath: "/modules",
		Backend:     config.Backend{"bucket": "my-state", "region": "us-east-1"},
		Root:        testCfg.Root,
	}
	l := makeLeaf([]string{"vm"}, map[string]*leaf.Module{"vm": {Source: "gcp/5/compute-instance"}})
	out, err := Generate(gcpCfg, gcpSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	// providers.yaml key is "gcp" but source is hashicorp/google → HCL name is "google"
	if !strings.Contains(out, `google = {`) {
		t.Errorf("HCL provider name should be 'google' (last segment of hashicorp/google)\ngot:\n%s", out)
	}
	if !strings.Contains(out, `provider "google"`) {
		t.Errorf("missing provider google block\ngot:\n%s", out)
	}
	if !strings.Contains(out, `project = "my-project"`) {
		t.Errorf("missing project substitution\ngot:\n%s", out)
	}
}

func TestGenerate_missingProviderEntry(t *testing.T) {
	// providers.yaml for aws exists but has no "nonexistent" entry
	l := makeLeaf([]string{"x"}, map[string]*leaf.Module{"x": {Source: "nonexistent/5/thing"}})
	_, err := Generate(testCfg, testSeg, l)
	if err == nil {
		t.Fatal("expected error when cloud missing from providers.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention missing cloud, got: %v", err)
	}
}

func TestGenerate_missingProvidersFile(t *testing.T) {
	// cloud path has no providers.yaml
	noProvSeg := &pathparse.Segments{
		Cloud: "noproviders", Profile: "p", Region: "r",
		Environment: "e", Class: "c", Component: "x",
	}
	l := makeLeaf([]string{"x"}, map[string]*leaf.Module{"x": {Source: "noproviders/1/thing"}})
	_, err := Generate(testCfg, noProvSeg, l)
	if err == nil {
		t.Fatal("expected error for missing providers.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "providers.yaml") {
		t.Errorf("error should mention providers.yaml, got: %v", err)
	}
}

func TestGenerate_conflictingMajors(t *testing.T) {
	l := makeLeaf([]string{"vpc", "ec2"}, map[string]*leaf.Module{
		"vpc": {Source: "aws/5/vpc"},
		"ec2": {Source: "aws/4/ec2"},
	})
	_, err := Generate(testCfg, testSeg, l)
	if err == nil {
		t.Fatal("expected error for conflicting major versions, got nil")
	}
	if !strings.Contains(err.Error(), "aws") {
		t.Errorf("error should mention cloud, got: %v", err)
	}
}

func TestGenerate_multiProvider(t *testing.T) {
	// aws + datadog both declared in infra/aws/providers.yaml
	l := makeLeaf([]string{"ec2", "monitor"}, map[string]*leaf.Module{
		"ec2":     {Source: "aws/5/ec2"},
		"monitor": {Source: "datadog/1/monitor"},
	})
	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`provider "aws"`,
		`provider "datadog"`,
		`aws = {`,
		`datadog = {`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestGenerate_remoteStateRef(t *testing.T) {
	l := &leaf.Leaf{
		ModuleKeys: []string{"ec2"},
		Modules: map[string]*leaf.Module{
			"ec2": {
				Source: "aws/5/ec2",
				Vars:   map[string]interface{}{"ec2_vpc_id": "${remote.vpc.vpc_id}"},
			},
		},
		RemoteStateKeys: []string{"vpc"},
		RemoteState:     map[string]string{"vpc": "infra/aws/waldman/us-east-1/base/vpc/test.yaml"},
	}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "ec2_vpc_id = data.terraform_remote_state.vpc.outputs.vpc_id") {
		t.Errorf("expected remote state ref, got:\n%s", out)
	}
}

func TestGenerate_remoteStateRefMixed(t *testing.T) {
	l := &leaf.Leaf{
		ModuleKeys: []string{"ec2"},
		Modules: map[string]*leaf.Module{
			"ec2": {
				Source: "aws/5/ec2",
				Vars:   map[string]interface{}{"ec2_tag": "${remote.vpc.name}/ec2"},
			},
		},
		RemoteStateKeys: []string{"vpc"},
		RemoteState:     map[string]string{"vpc": "infra/aws/waldman/us-east-1/base/vpc/test.yaml"},
	}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, `ec2_tag = "${data.terraform_remote_state.vpc.outputs.name}/ec2"`) {
		t.Errorf("expected interpolated remote state ref, got:\n%s", out)
	}
}

func TestGenerate_inheritedVars(t *testing.T) {
	// Write a vars.yaml at the cloud level for this test
	cloudDir := filepath.Join(testCfg.Root, "infra", "aws")
	if err := os.WriteFile(filepath.Join(cloudDir, "vars.yaml"), []byte("cost_center: engineering\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(filepath.Join(cloudDir, "vars.yaml")) })

	l := makeLeaf([]string{"ec2"}, map[string]*leaf.Module{"ec2": {Source: "aws/5/ec2"}})
	l.InheritedVars = map[string]interface{}{"cost_center": "engineering"}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `cost_center = "engineering"`) {
		t.Errorf("inherited var missing from output:\n%s", out)
	}
}

func TestGenerate_inheritedVarsModuleWins(t *testing.T) {
	l := makeLeaf([]string{"ec2"}, map[string]*leaf.Module{
		"ec2": {Source: "aws/5/ec2", Vars: map[string]interface{}{"cost_center": "override"}},
	})
	l.InheritedVars = map[string]interface{}{"cost_center": "inherited"}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `cost_center = "override"`) {
		t.Errorf("module var should override inherited:\n%s", out)
	}
	if strings.Contains(out, `cost_center = "inherited"`) {
		t.Errorf("inherited value should be overridden:\n%s", out)
	}
}

func TestGenerate_inheritedVarsCrossRefRejected(t *testing.T) {
	l := makeLeaf([]string{"ec2"}, map[string]*leaf.Module{"ec2": {Source: "aws/5/ec2"}})
	l.InheritedVars = map[string]interface{}{"bad": "${module.ec2.output}"}

	_, err := Generate(testCfg, testSeg, l)
	if err == nil {
		t.Fatal("expected error for cross-ref in inherited vars, got nil")
	}
	if !strings.Contains(err.Error(), "vars.yaml") {
		t.Errorf("error should mention vars.yaml, got: %v", err)
	}
}

func TestGenerate_remoteStateBlocksBeforeModules(t *testing.T) {
	l := &leaf.Leaf{
		ModuleKeys: []string{"ec2"},
		Modules:    map[string]*leaf.Module{"ec2": {Source: "aws/5/ec2"}},
		RemoteStateKeys: []string{"vpc"},
		RemoteState:     map[string]string{"vpc": "infra/aws/waldman/us-east-1/base/vpc/test.yaml"},
	}

	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}

	dataIdx := strings.Index(out, `data "terraform_remote_state"`)
	modIdx := strings.Index(out, `module "ec2"`)
	if dataIdx == -1 || modIdx == -1 {
		t.Fatal("expected both data and module blocks")
	}
	if dataIdx > modIdx {
		t.Error("remote state blocks must appear before module blocks")
	}
}
