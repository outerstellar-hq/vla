package gitignore

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGitignore(t *testing.T, root, content string) {
	t.Helper()
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(content), 0644)
}

func TestMatcher_Empty(t *testing.T) {
	m := Load(t.TempDir())
	if m.IsIgnored("src/main.go", false) {
		t.Error("empty gitignore should not ignore anything")
	}
}

func TestMatcher_ExactName(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "secret.txt\n")
	m := Load(dir)
	if !m.IsIgnored("secret.txt", false) {
		t.Error("secret.txt should be ignored")
	}
	if m.IsIgnored("main.go", false) {
		t.Error("main.go should not be ignored")
	}
}

func TestMatcher_DirPattern(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "node_modules/\n")
	m := Load(dir)
	if !m.IsIgnored("node_modules", true) {
		t.Error("node_modules dir should be ignored")
	}
	if m.IsIgnored("node_modules", false) {
		t.Error("node_modules as a file should NOT be ignored (dirOnly pattern)")
	}
}

func TestMatcher_Wildcard(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "*.log\n*.tmp\n")
	m := Load(dir)
	if !m.IsIgnored("debug.log", false) {
		t.Error("*.log should match debug.log")
	}
	if !m.IsIgnored("cache.tmp", false) {
		t.Error("*.tmp should match cache.tmp")
	}
	if m.IsIgnored("main.go", false) {
		t.Error("main.go should not be ignored")
	}
}

func TestMatcher_Negation(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "*.log\n!important.log\n")
	m := Load(dir)
	if !m.IsIgnored("debug.log", false) {
		t.Error("debug.log should be ignored")
	}
	if m.IsIgnored("important.log", false) {
		t.Error("important.log should NOT be ignored (negation)")
	}
}

func TestMatcher_NestedPath(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "build/\n")
	m := Load(dir)
	if !m.IsIgnored("build/output.o", false) {
		t.Error("files under build/ should be ignored")
	}
}

func TestMatcher_Comments(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "# this is a comment\nsecret.txt\n  # indented comment\n")
	m := Load(dir)
	if !m.IsIgnored("secret.txt", false) {
		t.Error("secret.txt should be ignored despite comments")
	}
}

func TestIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	writeGitignore(t, dir, "node_modules/\nvendor/\n*.log\nbuild/\n")
	m := Load(dir)
	dirs := m.IgnoredDirs()
	found := map[string]bool{}
	for _, d := range dirs {
		found[d] = true
	}
	if !found["node_modules"] {
		t.Error("expected node_modules in ignored dirs")
	}
	if !found["vendor"] {
		t.Error("expected vendor in ignored dirs")
	}
	if !found["build"] {
		t.Error("expected build in ignored dirs")
	}
	// *.log is a wildcard, should not be in dirs.
	if found["*.log"] {
		t.Error("wildcard patterns should not be in IgnoredDirs")
	}
}

func TestWildcardMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.log", "debug.log", true},
		{"*.log", "debug.txt", false},
		{"cache.*", "cache.tmp", true},
		{"cache.*", "other.tmp", false},
		{"*test*", "my_test_file.go", true},
		{"*.go", "main.go", true},
		{"exact", "exact", true},
		{"exact", "other", false},
	}
	for _, c := range cases {
		got := wildcardMatch(c.pattern, c.name)
		if got != c.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}
