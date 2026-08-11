package runner

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// NeedsInit reports whether terraform init has not been run in the cache dir.
func NeedsInit(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, ".terraform"))
	return os.IsNotExist(err)
}

// Terraform runs terraform with the given subcommand and extra args in cacheDir.
func Terraform(cacheDir string, subcmd string, extraArgs []string) error {
	args := append([]string{subcmd}, extraArgs...)
	cmd := exec.Command("terraform", args...)
	cmd.Dir = cacheDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Init runs terraform init in cacheDir.
func Init(cacheDir string) error {
	return Terraform(cacheDir, "init", nil)
}
