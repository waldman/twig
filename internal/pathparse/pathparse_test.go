package pathparse

import (
	"testing"
)

func TestParse_valid(t *testing.T) {
	seg, err := Parse(
		"/project",
		"/project/infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seg.Cloud != "aws" {
		t.Errorf("Cloud = %q, want %q", seg.Cloud, "aws")
	}
	if seg.Profile != "waldman" {
		t.Errorf("Profile = %q, want %q", seg.Profile, "waldman")
	}
	if seg.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", seg.Region, "us-east-1")
	}
	if seg.Environment != "production" {
		t.Errorf("Environment = %q, want %q", seg.Environment, "production")
	}
	if seg.Class != "services" {
		t.Errorf("Class = %q, want %q", seg.Class, "services")
	}
	if seg.Component != "ansible-anchor" {
		t.Errorf("Component = %q, want %q", seg.Component, "ansible-anchor")
	}
}

func TestParse_stateKey(t *testing.T) {
	seg, _ := Parse(
		"/project",
		"/project/infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml",
	)
	want := "infra/aws/waldman/us-east-1/production/services/ansible-anchor/terraform.tfstate"
	if seg.StateKey() != want {
		t.Errorf("StateKey = %q, want %q", seg.StateKey(), want)
	}
}

func TestParse_errors(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		leaf    string
		wantErr string
	}{
		{
			name:    "no yaml extension",
			root:    "/project",
			leaf:    "/project/infra/aws/waldman/us-east-1/production/services/ansible-anchor",
			wantErr: ".yaml extension",
		},
		{
			name:    "wrong segment count",
			root:    "/project",
			leaf:    "/project/infra/aws/waldman/us-east-1/production/ansible-anchor.yaml",
			wantErr: "infra/<cloud>",
		},
		{
			name:    "missing infra prefix",
			root:    "/project",
			leaf:    "/project/other/aws/waldman/us-east-1/production/services/ansible-anchor.yaml",
			wantErr: "infra/<cloud>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.root, tc.leaf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.wantErr != "" {
				if !contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
