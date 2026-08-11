package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Backend map[string]string

type Config struct {
	ModulesPath string  `yaml:"modules_path"`
	Backend     Backend `yaml:"backend"`
	// absolute path to the directory containing twig.yaml
	Root string `yaml:"-"`
}

// Load walks up from startDir until it finds twig.yaml, then parses it.
func Load(startDir string) (*Config, error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "twig.yaml")
		data, err := os.ReadFile(candidate)
		if err == nil {
			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("parse %s: %w", candidate, err)
			}
			cfg.Root = dir
			if cfg.ModulesPath == "" {
				return nil, fmt.Errorf("%s: modules_path is required", candidate)
			}
			if len(cfg.Backend) == 0 {
				return nil, fmt.Errorf("%s: backend is required", candidate)
			}
			if _, ok := cfg.Backend["key"]; ok {
				return nil, fmt.Errorf("%s: backend.key must not be set — twig derives it from the leaf path", candidate)
			}
			return &cfg, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("twig.yaml not found (walked up from %s)", startDir)
		}
		dir = parent
	}
}

// ModulesRoot returns the absolute path to the modules directory.
func (c *Config) ModulesRoot() string {
	if mp := os.Getenv("TWIG_MODULES_PATH"); mp != "" {
		if filepath.IsAbs(mp) {
			return mp
		}
		return filepath.Join(c.Root, mp)
	}
	if filepath.IsAbs(c.ModulesPath) {
		return c.ModulesPath
	}
	return filepath.Clean(filepath.Join(c.Root, c.ModulesPath))
}
