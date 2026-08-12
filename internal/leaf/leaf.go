package leaf

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/waldman/twig/internal/pathparse"
)

// All seven path variables — none may appear in vars.
var reservedVars = map[string]bool{
	"cloud":       true,
	"profile":     true,
	"region":      true,
	"environment": true,
	"class":       true,
	"component":   true,
	"module":      true,
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
		path := nodes[i+1].Value
		l.RemoteStateKeys = append(l.RemoteStateKeys, alias)
		l.RemoteState[alias] = path
	}

	// parse modules
	nodes = raw.Modules.Content
	for i := 0; i+1 < len(nodes); i += 2 {
		key := nodes[i].Value
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

// LoadInheritedVars walks from infra/ down through each path segment, merging
// any vars.yaml files found. Lower levels (closer to the leaf) override higher
// ones. Missing files are silently skipped. Reserved path variable names are
// rejected.
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
		var vars map[string]interface{}
		if err := yaml.Unmarshal(data, &vars); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for k, v := range vars {
			if reservedVars[k] {
				return nil, fmt.Errorf("%s: %q is a reserved path variable and cannot be used in vars.yaml", path, k)
			}
			merged[k] = v
		}
	}
	return merged, nil
}
