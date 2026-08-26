package runner

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// CacheDir returns the persistent cache directory for the given leaf path.
func CacheDir(leafAbs string) string {
	hash := sha256.Sum256([]byte(leafAbs))
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".twig-cache", fmt.Sprintf("%x", hash))
}

// WriteMain writes the generated main.tf into the cache directory.
func WriteMain(cacheDir, content string) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	return os.WriteFile(filepath.Join(cacheDir, "main.tf"), []byte(content), 0644)
}

const initHashFile = ".twig-init-hash"

// NeedsInit reports whether terraform init must be run and whether -upgrade is
// required. upgrade is true when .terraform/ exists but main.tf changed (the
// lock file may no longer satisfy the new constraints).
func NeedsInit(cacheDir string) (needsInit, upgrade bool) {
	if _, err := os.Stat(filepath.Join(cacheDir, ".terraform")); os.IsNotExist(err) {
		return true, false
	}
	cur, err := mainTFHash(cacheDir)
	if err != nil {
		return true, true
	}
	stored, err := os.ReadFile(filepath.Join(cacheDir, initHashFile))
	if err != nil {
		return true, true
	}
	if cur != strings.TrimSpace(string(stored)) {
		return true, true
	}
	return false, false
}

// RecordInitHash writes the current main.tf hash so NeedsInit can detect
// future changes.
func RecordInitHash(cacheDir string) error {
	h, err := mainTFHash(cacheDir)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cacheDir, initHashFile), []byte(h), 0644)
}

func mainTFHash(cacheDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir, "main.tf"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseEnvFile parses a shell-sourceable KEY=value file. Supports blank lines,
// # comments, export prefix, and single/double quoted values.
func parseEnvFile(data []byte, path string) ([]string, error) {
	var result []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=value, got: %q", path, lineNum, scanner.Text())
		}
		key = strings.TrimSpace(key)
		if !envKeyRe.MatchString(key) {
			return nil, fmt.Errorf("%s:%d: invalid env key %q", path, lineNum, key)
		}
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		result = append(result, key+"="+val)
	}
	return result, scanner.Err()
}

// LoadEnvFiles reads each file in paths, parses KEY=value pairs, and returns a
// combined slice. Files are processed in order; later entries for the same key
// override earlier ones. Missing files are a hard error.
func LoadEnvFiles(paths []string) ([]string, error) {
	var all []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("env_files: %w", err)
		}
		pairs, err := parseEnvFile(data, path)
		if err != nil {
			return nil, err
		}
		all = append(all, pairs...)
	}
	return all, nil
}

// mergeEnv merges base (e.g. os.Environ()) with overrides. For duplicate keys,
// overrides win. Order within each group is preserved.
func mergeEnv(base, overrides []string) []string {
	type entry struct {
		val string
		idx int
	}
	result := make(map[string]entry, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))

	for _, kv := range base {
		k, v, _ := strings.Cut(kv, "=")
		if _, seen := result[k]; !seen {
			order = append(order, k)
		}
		result[k] = entry{val: v}
	}
	for _, kv := range overrides {
		k, v, _ := strings.Cut(kv, "=")
		if _, seen := result[k]; !seen {
			order = append(order, k)
		}
		result[k] = entry{val: v}
	}

	out := make([]string, 0, len(result))
	for _, k := range order {
		out = append(out, k+"="+result[k].val)
	}
	return out
}

// Terraform runs terraform with the given subcommand and extra args in cacheDir.
// extraEnv, if non-nil, is merged into the subprocess environment (overrides win).
func Terraform(cacheDir string, subcmd string, extraArgs, extraEnv []string) error {
	args := append([]string{subcmd}, extraArgs...)
	cmd := exec.Command("terraform", args...)
	cmd.Dir = cacheDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = mergeEnv(os.Environ(), extraEnv)
	}
	return cmd.Run()
}

// Init runs terraform init in cacheDir. Pass upgrade=true when the lock file
// may conflict with changed version constraints.
func Init(cacheDir string, upgrade bool, extraEnv []string) error {
	var args []string
	if upgrade {
		args = []string{"-upgrade"}
	}
	return Terraform(cacheDir, "init", args, extraEnv)
}
