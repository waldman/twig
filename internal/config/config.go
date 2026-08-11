package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Backend map[string]string

type Config struct {
	ModulesPath string  `yaml:"modules_path"`
	ModulesRef  string  `yaml:"modules_ref"`
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

// ModulesRoot returns the absolute local path to the modules directory.
// Only meaningful for local (non-git) module sources.
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

// IsGitSource reports whether modules_path (or TWIG_MODULES_PATH) is a git URL.
func (c *Config) IsGitSource() bool {
	mp := c.ModulesPath
	if env := os.Getenv("TWIG_MODULES_PATH"); env != "" {
		mp = env
	}
	return isGitPath(mp)
}

// ModuleSource returns the Terraform source string for the given module subpath.
// For local sources this is an absolute filesystem path; for git sources it is a
// git:: URL that Terraform resolves during init.
func (c *Config) ModuleSource(subpath string) string {
	mp := c.ModulesPath
	if env := os.Getenv("TWIG_MODULES_PATH"); env != "" {
		mp = env
	}
	if isGitPath(mp) {
		return buildGitSource(mp, subpath, c.ModulesRef)
	}
	return c.ModulesRoot() + "/" + subpath
}

func isGitPath(s string) bool {
	for _, prefix := range []string{
		"https://", "http://", "git@", "git::",
		"github.com/", "gitlab.com/", "bitbucket.org/",
	} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// buildGitSource constructs a git:: Terraform source URL.
//
// Supported input forms for mp:
//   - github.com/org/repo//subdir        (bare hostname)
//   - https://github.com/org/repo.git//subdir
//   - git@github.com:org/repo.git//subdir
//   - git::https://...                   (already prefixed)
func buildGitSource(mp, subpath, ref string) string {
	// Strip explicit git:: prefix — we'll add it back.
	mp = strings.TrimPrefix(mp, "git::")

	// Isolate the scheme (e.g. "https://") so the path-separator "//" search
	// doesn't accidentally split on the "//" inside "://".
	scheme := ""
	rest := mp
	if idx := strings.Index(mp, "://"); idx >= 0 {
		scheme = mp[:idx+3]
		rest = mp[idx+3:]
	}

	// Split on // to separate the repo URL from the in-repo subdirectory.
	repoPath, subdir, hasSubdir := strings.Cut(rest, "//")
	repoURL := scheme + repoPath

	// Bare hostname (no scheme, not SSH) → https + .git
	if scheme == "" && !strings.HasPrefix(repoURL, "git@") {
		repoURL = "https://" + repoURL
		if !strings.HasSuffix(repoURL, ".git") {
			repoURL += ".git"
		}
	}

	var fullPath string
	if hasSubdir && subdir != "" {
		fullPath = subdir + "/" + subpath
	} else {
		fullPath = subpath
	}

	src := "git::" + repoURL + "//" + fullPath
	if ref != "" {
		src += "?ref=" + ref
	}
	return src
}
