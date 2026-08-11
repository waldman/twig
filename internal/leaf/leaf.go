package leaf

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// All seven path variables — none may appear in vars.
var reservedVars = map[string]bool{
	"provider":    true,
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

// Leaf is the parsed leaf file. Order of module declarations is preserved.
type Leaf struct {
	ModuleKeys []string
	Modules    map[string]*Module
}

type rawLeaf struct {
	Modules yaml.Node `yaml:"modules"`
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

	leaf := &Leaf{Modules: make(map[string]*Module)}

	nodes := raw.Modules.Content
	for i := 0; i+1 < len(nodes); i += 2 {
		key := nodes[i].Value
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
		leaf.ModuleKeys = append(leaf.ModuleKeys, key)
		leaf.Modules[key] = &mod
	}

	return leaf, nil
}
