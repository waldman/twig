package generate

import (
	"strings"
	"testing"

	"github.com/waldman/twig/internal/config"
	"github.com/waldman/twig/internal/leaf"
	"github.com/waldman/twig/internal/pathparse"
)

var testSeg = &pathparse.Segments{
	Provider:    "aws",
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`provider    = "aws"`,
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
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

	_, err := Generate(testCfg, testSeg, l, "/modules")
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
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

	out, err := Generate(testCfg, testSeg, l, "/modules")
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

func TestStringToHCL(t *testing.T) {
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
		got := stringToHCL(tc.input)
		if got != tc.want {
			t.Errorf("stringToHCL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
