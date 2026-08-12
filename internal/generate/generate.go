package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/waldman/twig/internal/config"
	"github.com/waldman/twig/internal/leaf"
	"github.com/waldman/twig/internal/pathparse"
)

var crossRefRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\}`)

type providerEntry struct {
	Source string                 `yaml:"source"`
	Config map[string]interface{} `yaml:"config"`
}

type providerReq struct {
	hclName string
	source  string
	version string
	config  map[string]interface{}
}

// providerHCLName derives the HCL provider name from a registry source URL.
// "hashicorp/aws" → "aws", "hashicorp/google" → "google", "digitalocean/digitalocean" → "digitalocean"
func providerHCLName(source string) string {
	parts := strings.Split(source, "/")
	return strings.ToLower(parts[len(parts)-1])
}

// loadProvidersFile reads infra/<cloud>/providers.yaml relative to the project root.
func loadProvidersFile(root, cloud string) (map[string]providerEntry, error) {
	path := filepath.Join(root, "infra", cloud, "providers.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("providers.yaml not found for cloud %q (expected at infra/%s/providers.yaml): %w", cloud, cloud, err)
	}
	var entries map[string]providerEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return entries, nil
}

// collectProviders scans module sources to derive cloud→major version, loads
// infra/<cloud>/providers.yaml, and builds one providerReq per distinct cloud.
func collectProviders(cfg *config.Config, seg *pathparse.Segments, l *leaf.Leaf) ([]providerReq, error) {
	majors := make(map[string]string)
	for _, key := range l.ModuleKeys {
		src := l.Modules[key].Source
		parts := strings.SplitN(src, "/", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("module %q: source %q does not match <cloud>/<major>/... format", key, src)
		}
		cloud, major := parts[0], parts[1]
		if prev, ok := majors[cloud]; ok && prev != major {
			return nil, fmt.Errorf("module %q: conflicting provider major versions for %q: %s and %s", key, cloud, prev, major)
		}
		majors[cloud] = major
	}

	if len(majors) == 0 {
		return nil, nil
	}

	entries, err := loadProvidersFile(cfg.Root, seg.Cloud)
	if err != nil {
		return nil, err
	}

	clouds := make([]string, 0, len(majors))
	for cloud := range majors {
		clouds = append(clouds, cloud)
	}
	sort.Strings(clouds)

	var reqs []providerReq
	for _, cloud := range clouds {
		entry, ok := entries[cloud]
		if !ok {
			return nil, fmt.Errorf("module uses cloud %q but %q is not declared in infra/%s/providers.yaml", cloud, cloud, seg.Cloud)
		}
		reqs = append(reqs, providerReq{
			hclName: providerHCLName(entry.Source),
			source:  entry.Source,
			version: fmt.Sprintf("~> %s.0", majors[cloud]),
			config:  entry.Config,
		})
	}
	return reqs, nil
}

func substitutePathVars(s string, seg *pathparse.Segments) string {
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

func writeProviderBlocks(b *strings.Builder, providers []providerReq, seg *pathparse.Segments) {
	for _, p := range providers {
		b.WriteString(fmt.Sprintf("provider %q {\n", p.hclName))
		keys := make([]string, 0, len(p.config))
		for k := range p.config {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("  %s = %s\n", k, providerValToHCL(p.config[k], seg)))
		}
		b.WriteString("}\n\n")
	}
}

func providerValToHCL(v interface{}, seg *pathparse.Segments) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", substitutePathVars(val, seg))
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}

// Generate produces the content of main.tf for the given leaf.
func Generate(cfg *config.Config, seg *pathparse.Segments, l *leaf.Leaf) (string, error) {
	resolve := makeResolver(l.Modules, l.RemoteState)

	if err := validateInheritedVars(l.InheritedVars); err != nil {
		return "", err
	}

	for _, key := range l.ModuleKeys {
		if err := validateRefs(key, l.Modules[key].Vars, l.Modules, l.RemoteState); err != nil {
			return "", err
		}
	}

	providers, err := collectProviders(cfg, seg, l)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	writeTerraformBlock(&b, cfg, seg, providers)
	writeProviderBlocks(&b, providers, seg)

	if err := writeRemoteStateBlocks(&b, cfg, l); err != nil {
		return "", err
	}

	for _, key := range l.ModuleKeys {
		if err := writeModuleBlock(&b, key, l.Modules[key], l.InheritedVars, seg, cfg, resolve); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

func validateInheritedVars(vars map[string]interface{}) error {
	return walkRefs(vars, func(ref, field string) error {
		return fmt.Errorf("inherited vars: cross-references (${%s.%s}) are not supported in vars.yaml", ref, field)
	})
}

// makeResolver returns a function that maps (alias, field) to the correct HCL
// expression — module ref for module keys, remote state ref for remote_state aliases.
func makeResolver(modules map[string]*leaf.Module, remoteState map[string]string) func(string, string) string {
	return func(alias, field string) string {
		if _, ok := remoteState[alias]; ok {
			return "data.terraform_remote_state." + alias + ".outputs." + field
		}
		return "module." + alias + "." + field
	}
}

func writeTerraformBlock(b *strings.Builder, cfg *config.Config, seg *pathparse.Segments, providers []providerReq) {
	b.WriteString("terraform {\n")
	b.WriteString("  required_version = \">= 1.1\"\n")

	if len(providers) > 0 {
		b.WriteString("  required_providers {\n")
		for _, p := range providers {
			b.WriteString(fmt.Sprintf("    %s = {\n", p.hclName))
			b.WriteString(fmt.Sprintf("      source  = %q\n", p.source))
			b.WriteString(fmt.Sprintf("      version = %q\n", p.version))
			b.WriteString("    }\n")
		}
		b.WriteString("  }\n")
	}

	b.WriteString("  backend \"s3\" {\n")

	keys := make([]string, 0, len(cfg.Backend))
	for k := range cfg.Backend {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("    %-16s= %q\n", k, cfg.Backend[k]))
	}
	b.WriteString(fmt.Sprintf("    %-16s= %q\n", "key", seg.StateKey()))
	b.WriteString("  }\n")
	b.WriteString("}\n\n")
}

func writeRemoteStateBlocks(b *strings.Builder, cfg *config.Config, l *leaf.Leaf) error {
	for _, alias := range l.RemoteStateKeys {
		leafPath := l.RemoteState[alias]
		absLeaf := filepath.Join(cfg.Root, leafPath)
		remoteSeg, err := pathparse.Parse(cfg.Root, absLeaf)
		if err != nil {
			return fmt.Errorf("remote_state %q: invalid leaf path %q: %w", alias, leafPath, err)
		}

		b.WriteString(fmt.Sprintf("data \"terraform_remote_state\" %q {\n", alias))
		b.WriteString("  backend = \"s3\"\n")
		b.WriteString("  config = {\n")

		bkeys := make([]string, 0, len(cfg.Backend))
		for k := range cfg.Backend {
			if k != "dynamodb_table" && k != "key" {
				bkeys = append(bkeys, k)
			}
		}
		sort.Strings(bkeys)
		for _, k := range bkeys {
			b.WriteString(fmt.Sprintf("    %-12s= %q\n", k, cfg.Backend[k]))
		}
		b.WriteString(fmt.Sprintf("    %-12s= %q\n", "key", remoteSeg.StateKey()))
		b.WriteString("  }\n")
		b.WriteString("}\n\n")
	}
	return nil
}

func writeModuleBlock(b *strings.Builder, key string, mod *leaf.Module, inherited map[string]interface{}, seg *pathparse.Segments, cfg *config.Config, resolve func(string, string) string) error {
	srcPath := cfg.ModuleSource(mod.Source)

	b.WriteString(fmt.Sprintf("module %q {\n", key))
	b.WriteString(fmt.Sprintf("  source = %q\n\n", srcPath))

	b.WriteString(fmt.Sprintf("  cloud       = %q\n", seg.Cloud))
	b.WriteString(fmt.Sprintf("  profile     = %q\n", seg.Profile))
	b.WriteString(fmt.Sprintf("  region      = %q\n", seg.Region))
	b.WriteString(fmt.Sprintf("  environment = %q\n", seg.Environment))
	b.WriteString(fmt.Sprintf("  class       = %q\n", seg.Class))
	b.WriteString(fmt.Sprintf("  component   = %q\n", seg.Component))
	b.WriteString(fmt.Sprintf("  module      = %q\n", key))

	// Merge inherited vars and module vars; module vars override inherited.
	allVars := make(map[string]interface{}, len(inherited)+len(mod.Vars))
	for k, v := range inherited {
		allVars[k] = v
	}
	for k, v := range mod.Vars {
		allVars[k] = v
	}

	if len(allVars) > 0 {
		b.WriteString("\n")
		keys := make([]string, 0, len(allVars))
		for k := range allVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hcl, err := toHCL(allVars[k], resolve)
			if err != nil {
				return fmt.Errorf("module %q var %q: %w", key, k, err)
			}
			b.WriteString(fmt.Sprintf("  %s = %s\n", k, hcl))
		}
	}

	b.WriteString("}\n\n")
	return nil
}

func toHCL(v interface{}, resolve func(string, string) string) (string, error) {
	switch val := v.(type) {
	case string:
		return stringToHCL(val, resolve), nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case int:
		return fmt.Sprintf("%d", val), nil
	case float64:
		return fmt.Sprintf("%g", val), nil
	case []interface{}:
		return sliceToHCL(val, resolve)
	case map[string]interface{}:
		return mapToHCL(val, resolve)
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("unsupported type %T", v)
	}
}

func stringToHCL(s string, resolve func(string, string) string) string {
	// pure reference: entire string is exactly one ${x.y} token
	if m := crossRefRe.FindStringSubmatch(s); m != nil && m[0] == s {
		return resolve(m[1], m[2])
	}

	// mixed or plain string — substitute refs and emit as quoted HCL
	resolved := crossRefRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := crossRefRe.FindStringSubmatch(match)
		return "${" + resolve(sub[1], sub[2]) + "}"
	})
	return fmt.Sprintf("%q", resolved)
}

func sliceToHCL(items []interface{}, resolve func(string, string) string) (string, error) {
	var parts []string
	for _, item := range items {
		h, err := toHCL(item, resolve)
		if err != nil {
			return "", err
		}
		parts = append(parts, h)
	}
	return "[\n    " + strings.Join(parts, ",\n    ") + ",\n  ]", nil
}

func mapToHCL(m map[string]interface{}, resolve func(string, string) string) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		h, err := toHCL(m[k], resolve)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("      %s = %s", k, h))
	}
	return "{\n" + strings.Join(lines, "\n") + "\n    }", nil
}

func validateRefs(moduleKey string, vars map[string]interface{}, modules map[string]*leaf.Module, remoteState map[string]string) error {
	return walkRefs(vars, func(ref, field string) error {
		if _, ok := modules[ref]; ok {
			return nil
		}
		if _, ok := remoteState[ref]; ok {
			return nil
		}
		return fmt.Errorf("module %q: cross-ref ${%s.%s} references undeclared instance or remote state %q", moduleKey, ref, field, ref)
	})
}

func walkRefs(v interface{}, fn func(ref, field string) error) error {
	switch val := v.(type) {
	case string:
		for _, m := range crossRefRe.FindAllStringSubmatch(val, -1) {
			if err := fn(m[1], m[2]); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range val {
			if err := walkRefs(item, fn); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for _, item := range val {
			if err := walkRefs(item, fn); err != nil {
				return err
			}
		}
	}
	return nil
}
