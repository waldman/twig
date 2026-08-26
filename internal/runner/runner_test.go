package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile_basic(t *testing.T) {
	input := []byte("KEY=value\nANOTHER=123\n")
	pairs, err := parseEnvFile(input, "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 || pairs[0] != "KEY=value" || pairs[1] != "ANOTHER=123" {
		t.Errorf("unexpected pairs: %v", pairs)
	}
}

func TestParseEnvFile_commentsAndBlanks(t *testing.T) {
	input := []byte("# comment\n\nKEY=value\n# another comment\n")
	pairs, err := parseEnvFile(input, "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0] != "KEY=value" {
		t.Errorf("unexpected pairs: %v", pairs)
	}
}

func TestParseEnvFile_exportPrefix(t *testing.T) {
	input := []byte("export ARM_CLIENT_ID=abc123\n")
	pairs, err := parseEnvFile(input, "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0] != "ARM_CLIENT_ID=abc123" {
		t.Errorf("unexpected pairs: %v", pairs)
	}
}

func TestParseEnvFile_quotedValues(t *testing.T) {
	input := []byte(`KEY="double quoted"` + "\n" + `OTHER='single quoted'` + "\n")
	pairs, err := parseEnvFile(input, "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0] != "KEY=double quoted" {
		t.Errorf("expected double quotes stripped, got %q", pairs[0])
	}
	if pairs[1] != "OTHER=single quoted" {
		t.Errorf("expected single quotes stripped, got %q", pairs[1])
	}
}

func TestParseEnvFile_valueWithEquals(t *testing.T) {
	input := []byte("KEY=a=b=c\n")
	pairs, err := parseEnvFile(input, "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0] != "KEY=a=b=c" {
		t.Errorf("expected value with = preserved, got %q", pairs[0])
	}
}

func TestParseEnvFile_invalidKey(t *testing.T) {
	input := []byte("123BAD=value\n")
	_, err := parseEnvFile(input, "test.env")
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}
}

func TestParseEnvFile_missingEquals(t *testing.T) {
	input := []byte("NODIVIDER\n")
	_, err := parseEnvFile(input, "test.env")
	if err == nil {
		t.Fatal("expected error for missing =, got nil")
	}
}

func TestLoadEnvFiles_missingFile(t *testing.T) {
	_, err := LoadEnvFiles([]string{"/nonexistent/path.env"})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadEnvFiles_multipleFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.env")
	f2 := filepath.Join(dir, "b.env")
	os.WriteFile(f1, []byte("A=1\nSHARED=from_a\n"), 0644)
	os.WriteFile(f2, []byte("B=2\nSHARED=from_b\n"), 0644)

	pairs, err := LoadEnvFiles([]string{f1, f2})
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 4 {
		t.Fatalf("want 4 pairs, got %d: %v", len(pairs), pairs)
	}
}

func TestMergeEnv_fileWins(t *testing.T) {
	base := []string{"KEY=from_env", "BASE_ONLY=yes"}
	overrides := []string{"KEY=from_file", "FILE_ONLY=yes"}

	merged := mergeEnv(base, overrides)

	m := make(map[string]string)
	for _, kv := range merged {
		k, v, _ := splitFirst(kv)
		m[k] = v
	}

	if m["KEY"] != "from_file" {
		t.Errorf("expected file to win for KEY, got %q", m["KEY"])
	}
	if m["BASE_ONLY"] != "yes" {
		t.Errorf("expected BASE_ONLY preserved, got %q", m["BASE_ONLY"])
	}
	if m["FILE_ONLY"] != "yes" {
		t.Errorf("expected FILE_ONLY included, got %q", m["FILE_ONLY"])
	}
}

func TestNeedsInit_noTerraformDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# content"), 0644)
	needsInit, upgrade := NeedsInit(dir)
	if !needsInit {
		t.Fatal("expected needsInit=true when .terraform/ absent")
	}
	if upgrade {
		t.Fatal("expected upgrade=false on fresh init")
	}
}

func TestNeedsInit_hashMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".terraform"), 0755)
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# content"), 0644)
	if err := RecordInitHash(dir); err != nil {
		t.Fatal(err)
	}
	needsInit, _ := NeedsInit(dir)
	if needsInit {
		t.Fatal("expected needsInit=false when hash matches")
	}
}

func TestNeedsInit_hashMismatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".terraform"), 0755)
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# old content"), 0644)
	if err := RecordInitHash(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# new content"), 0644)
	needsInit, upgrade := NeedsInit(dir)
	if !needsInit {
		t.Fatal("expected needsInit=true when main.tf changed")
	}
	if !upgrade {
		t.Fatal("expected upgrade=true when main.tf changed with existing .terraform/")
	}
}

func splitFirst(kv string) (string, string, bool) {
	for i, c := range kv {
		if c == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}
