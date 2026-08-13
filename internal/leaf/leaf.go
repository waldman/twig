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

// Inherited holds the merged result of every vars.yaml file in the leaf's
// hierarchy, plus per-key origin tracking for provenance.
type Inherited struct {
	Vars        map[string]interface{}
	VarsOrigins map[string]string // var name → source vars.yaml path

	RemoteState        map[string]string // alias → leaf path
	RemoteStateOrigins map[string]string // alias → source vars.yaml path

	ModuleDefaults        map[string]map[string]interface{} // source → var → value
	ModuleDefaultsOrigins map[string]map[string]string      // source → var → source vars.yaml path
}

// Leaf is the parsed leaf file. Declaration order is preserved for both
// remote_state aliases and module instance keys.
type Leaf struct {
	Inherited *Inherited // populated by the caller after Load, via LoadInherited

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
var varsYamlAllowedTopLevel = map[string]bool{
	"vars":            true,
	"remote_state":    true,
	"module_defaults": true,
}

type varsYamlFile struct {
	Vars           map[string]interface{}            `yaml:"vars"`
	RemoteState    map[string]string                 `yaml:"remote_state"`
	ModuleDefaults map[string]map[string]interface{} `yaml:"module_defaults"`
	Extra          map[string]yaml.Node              `yaml:",inline"`
}

// LoadInherited walks from infra/ down through each path segment, merging
// every vars.yaml file's `vars:`, `remote_state:`, and `module_defaults:`
// sections. Lower levels (closer to the leaf) override higher ones per key.
// Missing files are silently skipped. The returned Inherited also carries
// per-key origin tracking (which vars.yaml supplied each value) for the
// generator's provenance comments.
func LoadInherited(root string, seg *pathparse.Segments) (*Inherited, error) {
	dirs := []string{
		filepath.Join(root, "infra"),
		filepath.Join(root, "infra", seg.Cloud),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region, seg.Environment),
		filepath.Join(root, "infra", seg.Cloud, seg.Profile, seg.Region, seg.Environment, seg.Class),
	}

	inh := &Inherited{
		Vars:                  make(map[string]interface{}),
		VarsOrigins:           make(map[string]string),
		RemoteState:           make(map[string]string),
		RemoteStateOrigins:    make(map[string]string),
		ModuleDefaults:        make(map[string]map[string]interface{}),
		ModuleDefaultsOrigins: make(map[string]map[string]string),
	}

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
				return nil, fmt.Errorf("%s: unknown top-level key %q (accepted: vars, remote_state, module_defaults)", path, k)
			}
		}

		// vars
		for k, v := range file.Vars {
			if reservedVars[k] {
				return nil, fmt.Errorf("%s: %q is a reserved path variable and cannot be used inside vars:", path, k)
			}
			inh.Vars[k] = v
			inh.VarsOrigins[k] = path
		}

		// remote_state
		for alias, leafPath := range file.RemoteState {
			if reservedKeys[alias] {
				return nil, fmt.Errorf("%s: remote_state alias %q is a reserved ref namespace", path, alias)
			}
			inh.RemoteState[alias] = leafPath
			inh.RemoteStateOrigins[alias] = path
		}

		// module_defaults
		for source, vars := range file.ModuleDefaults {
			if inh.ModuleDefaults[source] == nil {
				inh.ModuleDefaults[source] = make(map[string]interface{})
				inh.ModuleDefaultsOrigins[source] = make(map[string]string)
			}
			for k, v := range vars {
				if reservedVars[k] {
					return nil, fmt.Errorf("%s: %q is a reserved path variable and cannot appear in module_defaults[%q]", path, k, source)
				}
				inh.ModuleDefaults[source][k] = v
				inh.ModuleDefaultsOrigins[source][k] = path
			}
		}
	}
	return inh, nil
}
