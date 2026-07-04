package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfine_RelativeInside(t *testing.T) {
	base := t.TempDir()
	got, err := Confine(base, "src/foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "src", "foo.go")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfine_AbsoluteInside(t *testing.T) {
	base := t.TempDir()
	absPath := filepath.Join(base, "src", "foo.go")
	got, err := Confine(base, absPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != absPath {
		t.Errorf("got %q, want %q", got, absPath)
	}
}

func TestConfine_DotDotEscape(t *testing.T) {
	base := t.TempDir()
	_, err := Confine(base, "../outside.txt")
	if err == nil {
		t.Fatal("expected error for ../ escape, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("error should mention escape, got: %v", err)
	}
}

func TestConfine_DotDotDeepEscape(t *testing.T) {
	base := t.TempDir()
	_, err := Confine(base, "src/../../outside.txt")
	if err == nil {
		t.Fatal("expected error for deep ../ escape, got nil")
	}
}

func TestConfine_PrefixMatchNotFooled(t *testing.T) {
	// Classic bug: /home/foo should NOT match as inside /home/foobar.
	parent := t.TempDir()
	base := filepath.Join(parent, "projX")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(parent, "projXsecret", "file.txt")
	_, err := Confine(base, sibling)
	if err == nil {
		t.Fatal("expected error for sibling dir with shared name prefix, got nil")
	}
}

func TestConfine_ExactBaseDir(t *testing.T) {
	base := t.TempDir()
	// Resolve to canonical form (handles Windows 8.3 short names).
	resolvedBase, _ := filepath.EvalSymlinks(base)
	if resolvedBase == "" {
		resolvedBase = base
	}
	got, err := Confine(base, ".")
	if err != nil {
		t.Fatalf("Confine(\".\"): %v", err)
	}
	if got != resolvedBase {
		t.Errorf("got %q, want %q", got, resolvedBase)
	}
}

func TestConfine_DotdotToBaseItself(t *testing.T) {
	// "src/../foo.go" is fine — it's still inside base.
	base := t.TempDir()
	resolvedBase, _ := filepath.EvalSymlinks(base)
	if resolvedBase == "" {
		resolvedBase = base
	}
	got, err := Confine(base, "src/../foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(resolvedBase, "foo.go")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfine_RootSlashes(t *testing.T) {
	// Absolute path with redundant separators cleans up.
	base := t.TempDir()
	got, err := Confine(base, "src//foo/./bar.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "src", "foo", "bar.go")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Symlink hardening tests ---

func TestConfine_SymlinkEscapeBlocked(t *testing.T) {
	// Create a symlink inside the project that points outside.
	base := t.TempDir()
	outside := t.TempDir() // a directory outside base

	// Create a target file outside the project.
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	// Symlink inside base pointing to the outside file.
	link := filepath.Join(base, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v (skipping on this platform)", err)
	}

	_, err := Confine(base, "link.txt")
	if err == nil {
		t.Fatal("expected error for symlink pointing outside project, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should mention symlink, got: %v", err)
	}
}

func TestConfine_SymlinkToInsideAllowed(t *testing.T) {
	// Symlink pointing to a file *inside* the project should be allowed.
	base := t.TempDir()

	// Create a real file inside the project.
	realFile := filepath.Join(base, "real.txt")
	if err := os.WriteFile(realFile, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	// Symlink to it (also inside the project).
	link := filepath.Join(base, "link.txt")
	if err := os.Symlink(realFile, link); err != nil {
		t.Skipf("cannot create symlink: %v (skipping)", err)
	}

	got, err := Confine(base, "link.txt")
	if err != nil {
		t.Fatalf("symlink to inside-base file should be allowed: %v", err)
	}
	// Should resolve to the real file's canonical path.
	resolvedReal, _ := filepath.EvalSymlinks(realFile)
	if resolvedReal == "" {
		resolvedReal = realFile
	}
	if got != resolvedReal {
		t.Errorf("expected resolved path %q, got %q", resolvedReal, got)
	}
}

func TestConfine_SymlinkDirEscapeBlocked(t *testing.T) {
	// A symlinked directory pointing outside the project.
	base := t.TempDir()
	outside := t.TempDir()

	// Create a file in the outside dir.
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	// Symlink a directory inside base to the outside dir.
	linkDir := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("cannot create symlink: %v (skipping)", err)
	}

	_, err := Confine(base, "escape/secret.txt")
	if err == nil {
		t.Fatal("expected error for symlinked dir escape, got nil")
	}
}

func TestConfine_NewFileInExistingDir(t *testing.T) {
	// A file that doesn't exist yet but whose parent dir is real.
	// Should pass (used by write_file).
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "src"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := Confine(base, "src/newfile.go")
	if err != nil {
		t.Fatalf("new file in existing dir should pass: %v", err)
	}
	// Should contain the resolved base dir.
	resolvedBase, _ := filepath.EvalSymlinks(base)
	if resolvedBase == "" {
		resolvedBase = base
	}
	if !strings.HasPrefix(got, resolvedBase) {
		t.Errorf("path should be within base: got %q, base %q", got, resolvedBase)
	}
}

func TestConfine_NewFileSymlinkParentBlocked(t *testing.T) {
	// Parent dir is a symlink pointing outside — new file should be blocked.
	base := t.TempDir()
	outside := t.TempDir()

	linkDir := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("cannot create symlink: %v (skipping)", err)
	}

	_, err := Confine(base, "escape/newfile.txt")
	if err == nil {
		t.Fatal("expected error for new file in symlinked dir, got nil")
	}
	if !strings.Contains(err.Error(), "parent symlink") {
		t.Errorf("error should mention parent symlink, got: %v", err)
	}
}

func TestIsSymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real.txt")
	if err := os.WriteFile(real, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(base, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v (skipping)", err)
	}

	if !IsSymlink(link) {
		t.Error("IsSymlink should return true for symlink")
	}
	if IsSymlink(real) {
		t.Error("IsSymlink should return false for regular file")
	}
	if IsSymlink(filepath.Join(base, "nonexistent")) {
		t.Error("IsSymlink should return false for nonexistent path")
	}
}
