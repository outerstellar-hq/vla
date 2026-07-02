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
	got, err := Confine(base, ".")
	if err != nil {
		t.Fatalf("Confine(\".\"): %v", err)
	}
	if got != base {
		t.Errorf("got %q, want %q", got, base)
	}
}

func TestConfine_DotdotToBaseItself(t *testing.T) {
	// "src/../foo.go" is fine — it's still inside base.
	base := t.TempDir()
	got, err := Confine(base, "src/../foo.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "foo.go")
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
