package pathparse

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Segments struct {
	Cloud       string
	Profile     string
	Region      string
	Environment string
	Class       string
	Component   string
}

// Parse extracts path variables from the absolute leaf file path given the
// project root (directory containing twig.yaml, which sits beside infra/).
//
// Expected structure: <root>/infra/<cloud>/<profile>/<region>/<env>/<class>/<component>.yaml
func Parse(root, leafAbs string) (*Segments, error) {
	rel, err := filepath.Rel(root, leafAbs)
	if err != nil {
		return nil, fmt.Errorf("cannot make %s relative to %s: %w", leafAbs, root, err)
	}

	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".yaml") {
		return nil, fmt.Errorf("leaf file must have .yaml extension, got: %s", rel)
	}

	// strip .yaml and split
	parts := strings.Split(strings.TrimSuffix(rel, ".yaml"), "/")
	// parts: ["infra", cloud, profile, region, environment, class, component]
	if len(parts) != 7 || parts[0] != "infra" {
		return nil, fmt.Errorf(
			"leaf must be at infra/<cloud>/<profile>/<region>/<env>/<class>/<component>.yaml below %s, got: %s",
			root, rel,
		)
	}

	return &Segments{
		Cloud:       parts[1],
		Profile:     parts[2],
		Region:      parts[3],
		Environment: parts[4],
		Class:       parts[5],
		Component:   parts[6],
	}, nil
}

// SubstitutePathVars replaces ${cloud}, ${profile}, ${region}, ${environment},
// ${class}, and ${component} in s with the corresponding segment values.
func SubstitutePathVars(s string, seg *Segments) string {
	r := strings.NewReplacer(
		"${cloud}", seg.Cloud,
		"${profile}", seg.Profile,
		"${region}", seg.Region,
		"${environment}", seg.Environment,
		"${class}", seg.Class,
		"${component}", seg.Component,
	)
	return r.Replace(s)
}

// StateKey returns the S3 backend key for this leaf.
func (s *Segments) StateKey() string {
	return fmt.Sprintf("infra/%s/%s/%s/%s/%s/%s/terraform.tfstate",
		s.Cloud, s.Profile, s.Region, s.Environment, s.Class, s.Component)
}
