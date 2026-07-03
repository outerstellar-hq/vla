package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathToURIString(t *testing.T) {
	got := pathToURIString("/home/user/project")
	if !strings.HasPrefix(got, "file://") {
		t.Errorf("expected file:// prefix, got %q", got)
	}
}

func TestURIToRelPath(t *testing.T) {
	base := "/home/user/project"
	uri := "file:///home/user/project/src/main.go"
	got := uriToRelPath(uri, base)
	if got != "src/main.go" {
		t.Errorf("got %q, want src/main.go", got)
	}
}

func TestURIToRelPath_OutsideBase(t *testing.T) {
	// A URI outside the base dir should still return something (best-effort).
	base := "/home/user/project"
	uri := "file:///other/path/file.go"
	got := uriToRelPath(uri, base)
	if got == "" {
		t.Error("expected non-empty path")
	}
}

func TestFormatLSPDefinition_SingleLocation(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///home/proj/src/main.go","range":{"start":{"line":5,"character":6}}}`)
	got := formatLSPDefinition(raw, "/home/proj")
	if !strings.Contains(got, "src/main.go") {
		t.Errorf("expected path in output, got %q", got)
	}
	if !strings.Contains(got, ":6") {
		t.Errorf("expected line 6 (1-based), got %q", got)
	}
}

func TestFormatLSPDefinition_MultipleLocations(t *testing.T) {
	raw := json.RawMessage(`[
		{"uri":"file:///home/proj/a.go","range":{"start":{"line":1,"character":0}}},
		{"uri":"file:///home/proj/b.go","range":{"start":{"line":2,"character":0}}}
	]`)
	got := formatLSPDefinition(raw, "/home/proj")
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 definitions, got %d: %q", len(lines), got)
	}
}

func TestFormatLSPDefinition_Empty(t *testing.T) {
	raw := json.RawMessage(`[]`)
	got := formatLSPDefinition(raw, "/home/proj")
	if got != "" {
		t.Errorf("expected empty for no locations, got %q", got)
	}
}

func TestFormatLSPReferences(t *testing.T) {
	raw := json.RawMessage(`[
		{"uri":"file:///home/proj/a.go","range":{"start":{"line":3,"character":0}}},
		{"uri":"file:///home/proj/b.go","range":{"start":{"line":7,"character":0}}}
	]`)
	got := formatLSPReferences(raw, "/home/proj")
	if !strings.Contains(got, "2 references") {
		t.Errorf("expected reference count, got %q", got)
	}
	if !strings.Contains(got, "a.go") || !strings.Contains(got, "b.go") {
		t.Errorf("expected both files, got %q", got)
	}
}

func TestFormatLSPReferences_Empty(t *testing.T) {
	raw := json.RawMessage(`[]`)
	got := formatLSPReferences(raw, "/home/proj")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHover_NoManager(t *testing.T) {
	h := Hover{BaseDir: t.TempDir()}
	out, _ := h.Execute(json.RawMessage(`{"file":"f.go","line":1}`))
	if !strings.Contains(out, "not available") {
		t.Errorf("got %q", out)
	}
}

func TestHover_MissingFile(t *testing.T) {
	h := Hover{BaseDir: t.TempDir()}
	out, _ := h.Execute(json.RawMessage(`{"line":1}`))
	if !strings.Contains(out, "file is required") {
		t.Errorf("got %q", out)
	}
}

func TestHover_MissingLine(t *testing.T) {
	h := Hover{BaseDir: t.TempDir()}
	out, _ := h.Execute(json.RawMessage(`{"file":"f.go"}`))
	if !strings.Contains(out, "line is required") {
		t.Errorf("got %q", out)
	}
}

func TestDiagnostics_NoManager(t *testing.T) {
	d := Diagnostics{BaseDir: t.TempDir()}
	out, _ := d.Execute(json.RawMessage(`{"file":"f.go"}`))
	if !strings.Contains(out, "not available") {
		t.Errorf("got %q", out)
	}
}

func TestDiagnostics_MissingFile(t *testing.T) {
	d := Diagnostics{BaseDir: t.TempDir()}
	out, _ := d.Execute(json.RawMessage(`{}`))
	if !strings.Contains(out, "file is required") {
		t.Errorf("got %q", out)
	}
}

func TestSearchNative_FallbackPath(t *testing.T) {
	// searchNative is the pure-Go fallback when rg isn't available.
	// Test it directly to cover that code path.
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "func target() {}\n")
	writeFile(t, dir, "b.txt", "not searchable\n")
	writeFile(t, dir, "blob.png", "target\x00\x01") // binary

	results, err := searchNative(dir, dir, "target", false, 10)
	if err != nil {
		t.Fatalf("searchNative: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r, "a.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a.go in results: %v", results)
	}
}

func TestSearchNative_NoResults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "nothing here\n")
	results, _ := searchNative(dir, dir, "target_xyz", false, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestIsBinaryExt(t *testing.T) {
	binary := []string{".png", ".jpg", ".exe", ".zip", ".pdf", ".mp4"}
	for _, ext := range binary {
		if !isBinaryExt(ext) {
			t.Errorf("expected %q to be binary", ext)
		}
	}
	text := []string{".go", ".py", ".js", ".txt", ".md", ".html"}
	for _, ext := range text {
		if isBinaryExt(ext) {
			t.Errorf("expected %q to be text", ext)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	if truncateStr("hello", 10) != "hello" {
		t.Error("short string should be unchanged")
	}
	got := truncateStr("hello world", 5)
	if got != "hello…" {
		t.Errorf("got %q", got)
	}
}

// silence unused imports
var _ = os.Stat
var _ = filepath.Join
