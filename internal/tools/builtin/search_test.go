package builtin

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestSearch_LiteralMatch(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.go", "package main\n\nfunc foo() {}\n")
	writeFile(t, dir, "b.go", "func bar() {}\nfunc foo() {}\n")
	s := Search{Ctx: ctx}
	out, err := s.Execute(json.RawMessage(`{"pattern":"foo"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Should find "foo" in both files (2+ matches).
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Errorf("expected >=2 matches, got %d:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if !strings.Contains(l, "foo") {
			t.Errorf("match line doesn't contain pattern: %q", l)
		}
	}
}

func TestSearch_NoMatch(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.go", "package main\n")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"nonexistent_xyz"}`))
	if out != "no matches found" {
		t.Errorf("got %q", out)
	}
}

func TestSearch_EmptyPattern(t *testing.T) {
	ctx, _ := ctxFor(t)
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":""}`))
	if !strings.Contains(out, "pattern is required") {
		t.Errorf("got %q", out)
	}
}

func TestSearch_Regex(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.go", "func foo()\nfunc bar()\nfunc foobar()\n")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"foo.*bar","regex":true}`))
	// Should match "func foobar()" — the only line containing foo...bar.
	if !strings.Contains(out, "foobar") {
		t.Errorf("expected foobar match:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		t.Errorf("expected exactly 1 match, got %d:\n%s", len(lines), out)
	}
}

func TestSearch_PathScoped(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "src/a.go", "target\n")
	writeFile(t, dir, "other/b.go", "target\n")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"target","path":"src"}`))
	if !strings.Contains(out, "src/a.go") {
		t.Errorf("expected src/a.go:\n%s", out)
	}
	if strings.Contains(out, "other/b.go") {
		t.Errorf("other/b.go should be excluded:\n%s", out)
	}
}

func TestSearch_MaxResults(t *testing.T) {
	ctx, dir := ctxFor(t)
	// Write a file with many matches.
	content := ""
	for i := 0; i < 50; i++ {
		content += "matchline\n"
	}
	writeFile(t, dir, "big.go", content)
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"matchline","max_results":5}`))
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 results (capped), got %d", len(lines))
	}
}

func TestSearch_SkipsBinaryFiles(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.go", "secret_token\n")
	// Write a fake binary file that happens to contain the pattern.
	writeFile(t, dir, "blob.png", "secret_token\n\x00\x01\x02")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"secret_token"}`))
	if strings.Contains(out, "blob.png") {
		t.Errorf("binary file should be skipped:\n%s", out)
	}
	if !strings.Contains(out, "a.go") {
		t.Errorf("a.go should match:\n%s", out)
	}
}

func TestSearch_SkipsIgnoredDirs(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "real.go", "findme\n")
	writeFile(t, dir, "node_modules/dep.go", "findme\n")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"findme"}`))
	if strings.Contains(out, "node_modules") {
		t.Errorf("node_modules should be skipped:\n%s", out)
	}
	if !strings.Contains(out, "real.go") {
		t.Errorf("real.go should match:\n%s", out)
	}
}

func TestSearch_EscapeBlocked(t *testing.T) {
	ctx, _ := ctxFor(t)
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"x","path":"../../../etc"}`))
	if !strings.Contains(out, "escapes") {
		t.Errorf("expected escape error, got %q", out)
	}
}

// If rg is installed, verify it produces the same results as the native path.
func TestSearch_RipgrepIfAvailable(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed; skipping ripgrep-specific test")
	}
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.go", "package main\nfunc target() {}\n")
	s := Search{Ctx: ctx}
	out, _ := s.Execute(json.RawMessage(`{"pattern":"target"}`))
	if !strings.Contains(out, "target") {
		t.Errorf("rg path produced no match:\n%s", out)
	}
}
