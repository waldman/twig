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

// refRe matches the three explicit ref forms:
//   ${module.ec2.vpc_id}   → ns=module, key=ec2,       field=vpc_id
//   ${remote.vpc.vpc_id}   → ns=remote, key=vpc,       field=vpc_id
//   ${vars.vpn_cidr}       → ns=vars,   key=vpn_cidr,  field=""
var refRe = regexp.MustCompile(`\$\{(module|remote|vars)\.([a-zA-Z_][a-zA-Z0-9_]*)(?:\.([a-zA-Z_][a-zA-Z0-9_]*))?\}`)

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

// emptyInherited is used when a Leaf has no Inherited populated (e.g. in tests).
func emptyInherited() *leaf.Inherited {
	return &leaf.Inherited{
		Vars:                  map[string]interface{}{},
		VarsOrigins:           map[string]string{},
		RemoteState:           map[string]string{},
		RemoteStateOrigins:    map[string]string{},
		ModuleDefaults:        map[string]map[string]interface{}{},
		ModuleDefaultsOrigins: map[string]map[string]string{},
	}
}

// effectiveRemoteState merges inherited remote_state overlaid by the leaf's
// own remote_state block. Leaf wins per alias.
func effectiveRemoteState(l *leaf.Leaf, inh *leaf.Inherited) map[string]string {
	m := make(map[string]string, len(inh.RemoteState)+len(l.RemoteState))
	for k, v := range inh.RemoteState {
		m[k] = v
	}
	for k, v := range l.RemoteState {
		m[k] = v
	}
	return m
}

// effectiveModuleVars merges inherited module_defaults[mod.Source] overlaid by
// the leaf's own modules.<instance>.vars. Leaf wins per key.
func effectiveModuleVars(mod *leaf.Module, inh *leaf.Inherited) map[string]interface{} {
	result := make(map[string]interface{})
	if defaults, ok := inh.ModuleDefaults[mod.Source]; ok {
		for k, v := range defaults {
			result[k] = v
		}
	}
	for k, v := range mod.Vars {
		result[k] = v
	}
	return result
}

// effectiveModuleVarsWithOrigins is like effectiveModuleVars but also returns
// per-key origin strings for provenance comments.
func effectiveModuleVarsWithOrigins(instanceKey string, mod *leaf.Module, inh *leaf.Inherited) (map[string]interface{}, map[string]string) {
	values := make(map[string]interface{})
	origins := make(map[string]string)
	if defaults, ok := inh.ModuleDefaults[mod.Source]; ok {
		defaultsOrigins := inh.ModuleDefaultsOrigins[mod.Source]
		for k, v := range defaults {
			values[k] = v
			origins[k] = fmt.Sprintf("%s: module_defaults.%q", defaultsOrigins[k], mod.Source)
		}
	}
	leafOrigin := fmt.Sprintf("leaf: modules.%s.vars", instanceKey)
	for k, v := range mod.Vars {
		values[k] = v
		origins[k] = leafOrigin
	}
	return values, origins
}

// collectReferencedRemoteAliases walks the effective vars of every module in
// the leaf and returns the set of remote_state aliases actually referenced.
// Only referenced aliases produce data blocks in the generated main.tf.
func collectReferencedRemoteAliases(l *leaf.Leaf, inh *leaf.Inherited) map[string]bool {
	referenced := make(map[string]bool)
	for _, key := range l.ModuleKeys {
		effVars := effectiveModuleVars(l.Modules[key], inh)
		_ = walkRefs(effVars, func(ns, alias, field string) error {
			if ns == "remote" {
				referenced[alias] = true
			}
			return nil
		})
	}
	return referenced
}

// Generate produces the content of main.tf for the given leaf.
func Generate(cfg *config.Config, seg *pathparse.Segments, l *leaf.Leaf) (string, error) {
	inh := l.Inherited
	if inh == nil {
		inh = emptyInherited()
	}

	if err := validateInheritedVars(inh.Vars); err != nil {
		return "", err
	}

	effRemote := effectiveRemoteState(l, inh)
	resolve := makeResolver(l.Modules, effRemote)

	// Validate refs in every module's effective vars (defaults + leaf).
	for _, key := range l.ModuleKeys {
		effVars := effectiveModuleVars(l.Modules[key], inh)
		if err := validateRefs(key, effVars, l.Modules, effRemote, inh.Vars); err != nil {
			return "", err
		}
	}

	providers, err := collectProviders(cfg, seg, l)
	if err != nil {
		return "", err
	}

	referencedRemotes := collectReferencedRemoteAliases(l, inh)

	var b strings.Builder

	writeTerraformBlock(&b, cfg, seg, providers)
	writeProviderBlocks(&b, providers, seg)

	if err := writeReferencedRemoteStateBlocks(&b, cfg, effRemote, referencedRemotes); err != nil {
		return "", err
	}

	for _, key := range l.ModuleKeys {
		if err := writeModuleBlock(&b, key, l.Modules[key], inh, seg, cfg, resolve); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

// validateInheritedVars enforces that ${...} references do not appear inside
// vars.yaml `vars:` values. References ARE permitted inside `module_defaults`
// values (which are validated per-consuming-leaf via validateRefs).
func validateInheritedVars(vars map[string]interface{}) error {
	return walkRefs(vars, func(ns, key, field string) error {
		return fmt.Errorf("inherited vars: references (${%s.%s...}) are not supported in vars.yaml", ns, key)
	})
}

// makeResolver returns a function that maps (ns, key, field) to the correct HCL
// expression for module and remote namespaces.
func makeResolver(modules map[string]*leaf.Module, remoteState map[string]string) func(string, string, string) string {
	return func(ns, key, field string) string {
		switch ns {
		case "remote":
			return "data.terraform_remote_state." + key + ".outputs." + field
		default: // "module"
			return "module." + key + "." + field
		}
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

// writeReferencedRemoteStateBlocks emits one data "terraform_remote_state" block
// per alias in `referenced`, resolving each alias against the effective merged
// remote_state map. Emission order is alphabetical among referenced aliases.
// Aliases in `referenced` but missing from `effRemote` fail with a validation
// error — that case is normally caught earlier by validateRefs.
func writeReferencedRemoteStateBlocks(b *strings.Builder, cfg *config.Config, effRemote map[string]string, referenced map[string]bool) error {
	aliases := make([]string, 0, len(referenced))
	for a := range referenced {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	for _, alias := range aliases {
		leafPath, ok := effRemote[alias]
		if !ok {
			return fmt.Errorf("remote_state %q: referenced but no matching alias in effective remote_state", alias)
		}
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

func writeModuleBlock(b *strings.Builder, key string, mod *leaf.Module, inh *leaf.Inherited, seg *pathparse.Segments, cfg *config.Config, resolve func(string, string, string) string) error {
	srcPath := cfg.ModuleSource(mod.Source)

	b.WriteString(fmt.Sprintf("module %q {\n", key))
	b.WriteString(fmt.Sprintf("  source = %q\n\n", srcPath))

	// Seven path variables, always present, always origin "path".
	b.WriteString(fmt.Sprintf("  cloud       = %q  # from: path\n", seg.Cloud))
	b.WriteString(fmt.Sprintf("  profile     = %q  # from: path\n", seg.Profile))
	b.WriteString(fmt.Sprintf("  region      = %q  # from: path\n", seg.Region))
	b.WriteString(fmt.Sprintf("  environment = %q  # from: path\n", seg.Environment))
	b.WriteString(fmt.Sprintf("  class       = %q  # from: path\n", seg.Class))
	b.WriteString(fmt.Sprintf("  component   = %q  # from: path\n", seg.Component))
	b.WriteString(fmt.Sprintf("  module      = %q  # from: path\n", key))

	// Effective vars: inherited module_defaults[mod.Source] overlaid by the
	// leaf's own modules.<key>.vars. Leaf wins per key.
	values, origins := effectiveModuleVarsWithOrigins(key, mod, inh)

	if len(values) > 0 {
		b.WriteString("\n")
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hcl, err := toHCL(values[k], resolve, inh.Vars)
			if err != nil {
				return fmt.Errorf("module %q var %q: %w", key, k, err)
			}
			b.WriteString(fmt.Sprintf("  %s = %s  # from: %s\n", k, hcl, origins[k]))
		}
	}

	b.WriteString("}\n\n")
	return nil
}

func toHCL(v interface{}, resolve func(string, string, string) string, inherited map[string]interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return stringToHCL(val, resolve, inherited), nil
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
		return sliceToHCL(val, resolve, inherited)
	case map[string]interface{}:
		return mapToHCL(val, resolve, inherited)
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("unsupported type %T", v)
	}
}

func stringToHCL(s string, resolve func(string, string, string) string, inherited map[string]interface{}) string {
	// pure reference: entire string is exactly one ref token
	if m := refRe.FindStringSubmatch(s); m != nil && m[0] == s {
		ns, key, field := m[1], m[2], m[3]
		if ns == "vars" {
			hcl, err := toHCL(inherited[key], resolve, inherited)
			if err != nil {
				return fmt.Sprintf("%q", fmt.Sprintf("%v", inherited[key]))
			}
			return hcl
		}
		return resolve(ns, key, field)
	}

	// mixed or plain string — substitute refs inline and emit as quoted HCL
	result := refRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := refRe.FindStringSubmatch(match)
		ns, key, field := sub[1], sub[2], sub[3]
		if ns == "vars" {
			return fmt.Sprintf("%v", inherited[key])
		}
		return "${" + resolve(ns, key, field) + "}"
	})
	return fmt.Sprintf("%q", result)
}

func sliceToHCL(items []interface{}, resolve func(string, string, string) string, inherited map[string]interface{}) (string, error) {
	var parts []string
	for _, item := range items {
		h, err := toHCL(item, resolve, inherited)
		if err != nil {
			return "", err
		}
		parts = append(parts, h)
	}
	return "[\n    " + strings.Join(parts, ",\n    ") + ",\n  ]", nil
}

func mapToHCL(m map[string]interface{}, resolve func(string, string, string) string, inherited map[string]interface{}) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		h, err := toHCL(m[k], resolve, inherited)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("      %s = %s", k, h))
	}
	return "{\n" + strings.Join(lines, "\n") + "\n    }", nil
}

func validateRefs(moduleKey string, vars map[string]interface{}, modules map[string]*leaf.Module, remoteState map[string]string, inherited map[string]interface{}) error {
	return walkRefs(vars, func(ns, key, field string) error {
		switch ns {
		case "module":
			if _, ok := modules[key]; !ok {
				return fmt.Errorf("module %q: ${module.%s.%s} references undeclared module %q", moduleKey, key, field, key)
			}
		case "remote":
			if _, ok := remoteState[key]; !ok {
				return fmt.Errorf("module %q: ${remote.%s.%s} references undeclared remote_state alias %q", moduleKey, key, field, key)
			}
		case "vars":
			if _, ok := inherited[key]; !ok {
				return fmt.Errorf("module %q: ${vars.%s} references undeclared inherited variable %q", moduleKey, key, key)
			}
		}
		return nil
	})
}

func walkRefs(v interface{}, fn func(ns, key, field string) error) error {
	switch val := v.(type) {
	case string:
		for _, m := range refRe.FindAllStringSubmatch(val, -1) {
			if err := fn(m[1], m[2], m[3]); err != nil {
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
