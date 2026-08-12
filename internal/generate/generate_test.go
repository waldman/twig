package generate

import (
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

var testCfg = &config.Config{
	ModulesPath: "/modules",
	Backend:     config.Backend{"bucket": "my-state", "region": "us-east-1"},
	Root:        "/project",
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
			"target_arn": "${bucket.bucket_arn}",
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
			"wildcard_arn": "${bucket.bucket_arn}/*",
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
			"arn": "${nonexistent.arn}",
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
		Root:        "/project",
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
	moduleResolve := func(alias, field string) string { return "module." + alias + "." + field }

	cases := []struct {
		input string
		want  string
	}{
		{"plain string", `"plain string"`},
		{"${x.arn}", "module.x.arn"},
		{"${x.arn}/*", `"${module.x.arn}/*"`},
		{"prefix-${x.arn}-suffix", `"prefix-${module.x.arn}-suffix"`},
	}

	for _, tc := range cases {
		got := stringToHCL(tc.input, moduleResolve)
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

func TestGenerate_awsProviderBlock(t *testing.T) {
	l := makeLeaf([]string{"vpc"}, map[string]*leaf.Module{"vpc": {Source: "aws/5/vpc"}})
	out, err := Generate(testCfg, testSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `provider "aws"`) {
		t.Error("missing aws provider block")
	}
	if !strings.Contains(out, `profile = "waldman"`) {
		t.Error("missing profile")
	}
	if strings.Contains(out, "required_providers") {
		t.Error("required_providers must not appear in generated output — modules own it")
	}
}

func TestGenerate_gcpProviderBlock(t *testing.T) {
	gcpSeg := &pathparse.Segments{
		Cloud: "gcp", Profile: "my-project", Region: "us-central1",
		Environment: "dev", Class: "compute", Component: "web",
	}
	l := makeLeaf([]string{"vm"}, map[string]*leaf.Module{"vm": {Source: "gcp/5/compute-instance"}})
	out, err := Generate(testCfg, gcpSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `provider "google"`) {
		t.Error("missing google provider block")
	}
	if !strings.Contains(out, `project = "my-project"`) {
		t.Error("missing project")
	}
	if !strings.Contains(out, `region  = "us-central1"`) {
		t.Error("missing region")
	}
}

func TestGenerate_digitaloceanProviderBlock(t *testing.T) {
	doSeg := &pathparse.Segments{
		Cloud: "digitalocean", Profile: "myteam", Region: "nyc3",
		Environment: "prod", Class: "droplet", Component: "web",
	}
	l := makeLeaf([]string{"droplet"}, map[string]*leaf.Module{"droplet": {Source: "digitalocean/2/droplet"}})
	out, err := Generate(testCfg, doSeg, l)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `provider "digitalocean"`) {
		t.Error("missing digitalocean provider block")
	}
}

func TestGenerate_unknownCloud(t *testing.T) {
	badSeg := &pathparse.Segments{
		Cloud: "azure", Profile: "myprofile", Region: "eastus",
		Environment: "dev", Class: "vm", Component: "web",
	}
	l := makeLeaf([]string{"vm"}, map[string]*leaf.Module{"vm": {Source: "azure/3/vm"}})
	_, err := Generate(testCfg, badSeg, l)
	if err == nil {
		t.Fatal("expected error for unsupported cloud, got nil")
	}
	if !strings.Contains(err.Error(), "azure") {
		t.Errorf("error should mention unsupported cloud, got: %v", err)
	}
}

func TestGenerate_remoteStateRef(t *testing.T) {
	l := &leaf.Leaf{
		ModuleKeys: []string{"ec2"},
		Modules: map[string]*leaf.Module{
			"ec2": {
				Source: "aws/5/ec2",
				Vars:   map[string]interface{}{"ec2_vpc_id": "${vpc.vpc_id}"},
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
				Vars:   map[string]interface{}{"ec2_tag": "${vpc.name}/ec2"},
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
