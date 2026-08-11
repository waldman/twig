package generate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/waldman/twig/internal/config"
	"github.com/waldman/twig/internal/leaf"
	"github.com/waldman/twig/internal/pathparse"
)

var crossRefRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// Generate produces the content of main.tf for the given leaf.
func Generate(cfg *config.Config, seg *pathparse.Segments, l *leaf.Leaf, modulesRoot string) (string, error) {
	for _, key := range l.ModuleKeys {
		if err := validateRefs(key, l.Modules[key].Vars, l.Modules); err != nil {
			return "", err
		}
	}

	var b strings.Builder

	writeTerraformBlock(&b, cfg, seg)
	writeProviderBlock(&b, seg)

	for _, key := range l.ModuleKeys {
		if err := writeModuleBlock(&b, key, l.Modules[key], seg, modulesRoot); err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

func writeTerraformBlock(b *strings.Builder, cfg *config.Config, seg *pathparse.Segments) {
	b.WriteString("terraform {\n")
	b.WriteString("  required_version = \">= 1.1\"\n")
	b.WriteString("  required_providers {\n")
	b.WriteString("    aws = {\n")
	b.WriteString("      source  = \"hashicorp/aws\"\n")
	b.WriteString("      version = \"~> 5.0\"\n")
	b.WriteString("    }\n")
	b.WriteString("  }\n")
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

func writeProviderBlock(b *strings.Builder, seg *pathparse.Segments) {
	b.WriteString("provider \"aws\" {\n")
	b.WriteString(fmt.Sprintf("  profile = %q\n", seg.Profile))
	b.WriteString(fmt.Sprintf("  region  = %q\n", seg.Region))
	b.WriteString("}\n\n")
}

func writeModuleBlock(b *strings.Builder, key string, mod *leaf.Module, seg *pathparse.Segments, modulesRoot string) error {
	srcPath := modulesRoot + "/" + mod.Source

	b.WriteString(fmt.Sprintf("module %q {\n", key))
	b.WriteString(fmt.Sprintf("  source = %q\n\n", srcPath))

	b.WriteString(fmt.Sprintf("  cloud       = %q\n", seg.Cloud))
	b.WriteString(fmt.Sprintf("  profile     = %q\n", seg.Profile))
	b.WriteString(fmt.Sprintf("  region      = %q\n", seg.Region))
	b.WriteString(fmt.Sprintf("  environment = %q\n", seg.Environment))
	b.WriteString(fmt.Sprintf("  class       = %q\n", seg.Class))
	b.WriteString(fmt.Sprintf("  component   = %q\n", seg.Component))
	b.WriteString(fmt.Sprintf("  module      = %q\n", key))

	if len(mod.Vars) > 0 {
		b.WriteString("\n")
		keys := make([]string, 0, len(mod.Vars))
		for k := range mod.Vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hcl, err := toHCL(mod.Vars[k])
			if err != nil {
				return fmt.Errorf("module %q var %q: %w", key, k, err)
			}
			b.WriteString(fmt.Sprintf("  %s = %s\n", k, hcl))
		}
	}

	b.WriteString("}\n\n")
	return nil
}

func toHCL(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return stringToHCL(val), nil
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
		return sliceToHCL(val)
	case map[string]interface{}:
		return mapToHCL(val)
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("unsupported type %T", v)
	}
}

func stringToHCL(s string) string {
	// pure reference: entire string is exactly one ${x.y} token
	if m := crossRefRe.FindStringSubmatch(s); m != nil && m[0] == s {
		return "module." + m[1] + "." + m[2]
	}

	// mixed or plain string — substitute refs and emit as quoted HCL
	resolved := crossRefRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := crossRefRe.FindStringSubmatch(match)
		return "${module." + sub[1] + "." + sub[2] + "}"
	})
	return fmt.Sprintf("%q", resolved)
}

func sliceToHCL(items []interface{}) (string, error) {
	var parts []string
	for _, item := range items {
		h, err := toHCL(item)
		if err != nil {
			return "", err
		}
		parts = append(parts, h)
	}
	return "[\n    " + strings.Join(parts, ",\n    ") + ",\n  ]", nil
}

func mapToHCL(m map[string]interface{}) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		h, err := toHCL(m[k])
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("      %s = %s", k, h))
	}
	return "{\n" + strings.Join(lines, "\n") + "\n    }", nil
}

func validateRefs(moduleKey string, vars map[string]interface{}, declared map[string]*leaf.Module) error {
	return walkRefs(vars, func(ref, field string) error {
		if _, ok := declared[ref]; !ok {
			return fmt.Errorf("module %q: cross-ref ${%s.%s} references undeclared instance %q", moduleKey, ref, field, ref)
		}
		return nil
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
