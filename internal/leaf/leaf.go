package leaf

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/waldman/twig/internal/pathparse"
)

// reservedVars — none may appear as var names.
var reservedVars = map[string]bool{
	"cloud":       true,
	"profile":     true,
	"region":      true,
	"environment": true,
	"class":       true,
	"component":   true,
	"module":      true,
}

// reservedKeys — none may be used as module instance keys or remote_state aliases
// (they are ref namespaces in the ${ns.key.field} syntax).
var reservedKeys = map[string]bool{
	"module": true,
	"remote": true,
	"vars":   true,
}

type Module struct {
	Source string                 `yaml:"source"`
	Vars   map[string]interface{} `yaml:"vars"`
}

// Leaf is the parsed leaf file. Declaration order is preserved for both
// remote_state aliases and module instance keys.
type Leaf struct {
	InheritedVars map[string]interface{} // merged from vars.yaml files up the tree

	RemoteStateKeys []string
	RemoteState     map[string]string // alias → leaf path relative to project root

	ModuleKeys []string
	Modules    map[string]*Module
}

type rawLeaf struct {
	RemoteState yaml.Node `yaml:"remote_state"`
	Modules     yaml.Node `yaml:"modules"`
}

func Load(leafFile string) (*Leaf, error) {
	data, err := os.ReadFile(leafFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", leafFile, err)
	}

	var raw rawLeaf
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", leafFile, err)
	}

	l := &Leaf{
		RemoteState: make(map[string]string),
		Modules:     make(map[string]*Module),
	}

	// parse remote_state first — needed for alias conflict check below
	nodes := raw.RemoteState.Content
	for i := 0; i+1 < len(nodes); i += 2 {
		alias := nodes[i].Value
		if reservedKeys[alias] {
			return nil, fmt.Errorf("remote_state alias %q is a reserved ref namespace and cannot be used as an alias", alias)
		}
		path := nodes[i+1].Value
		l.RemoteStateKeys = append(l.RemoteStateKeys, alias)
		l.RemoteState[alias] = path
	}

	// parse modules
	nodes = raw.Modules.Content
	for i := 0; i+1 < len(nodes); i += 2 {
		key := nodes[i].Value
		if reservedKeys[key] {
			return nil, fmt.Errorf("module key %q is a reserved ref namespace and cannot be used as an instance key", key)
		}
		if _, conflict := l.RemoteState[key]; conflict {
			return nil, fmt.Errorf("module key %q conflicts with remote_state alias of the same name", key)
		}
		var mod Module
		if err := nodes[i+1].Decode(&mod); err != nil {
			return nil, fmt.Errorf("module %q: %w", key, err)
		}
		if mod.Source == "" {
			return nil, fmt.Errorf("module %q: source is required", key)
		}
		for varName := range mod.Vars {
			if reservedVars[varName] {
				return nil, fmt.Errorf("module %q: %q is a reserved path variable and cannot appear in vars", key, varName)
			}
		}
		l.ModuleKeys = append(l.ModuleKeys, key)
		l.Modules[key] = &mod
	}

	return l, nil
}

// varsYamlAllowedTopLevel — accepted top-level keys in a vars.yaml file.
// PR 2 will add "module_defaults" and "remote_state".
var varsYamlAllowedTopLevel = map[string]bool{
	"vars": true,
}

type varsYamlFile struct {
	Vars  map[string]interface{}            `yaml:"vars"`
	Extra map[string]yaml.Node              `yaml:",inline"`
}

// LoadInheritedVars walks from infra/ down through each path segment, merging
// the `vars:` section of any vars.yaml files found. Lower levels (closer to
// the leaf) override higher ones. Missing files are silently skipped.
// Reserved path variable names are rejected as keys inside `vars:`, and any
// unknown top-level key in a vars.yaml is rejected.
func LoadInheritedVars(root string, seg *pathparse.Segments) (map[string]interface{}, error) {
	dirs := []string{
		filepath.Join(root, "infra"),
		filepath.Join(root, "infra", seg.Cloud),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region, seg.Environment),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region, seg.Environment, seg.Class),
	}

	merged := make(map[string]interface{})
	for _, dir := range dirs {
		path := filepath.Join(dir, "vars.yaml")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var file varsYamlFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for k := range file.Extra {
			if !varsYamlAllowedTopLevel[k] {
				return nil, fmt.Errorf("%s: unknown top-level key %q (only \"vars\" is accepted)", path, k)
			}
		}
		for k, v := range file.Vars {
			if reservedVars[k] {
				return nil, fmt.Errorf("%s: %q is a reserved path variable and cannot be used inside vars:", path, k)
			}
			merged[k] = v
		}
	}
	return merged, nil
}
