package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ctxFor(t *testing.T) (Ctx, string) {
	t.Helper()
	dir := t.TempDir()
	return Ctx{BaseDir: dir}, dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadFile_Success(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "src/main.go", "package main\n")
	r := ReadFile{Ctx: ctx}
	out, err := r.Execute(json.RawMessage(`{"path":"src/main.go"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "package main\n" {
		t.Errorf("got %q", out)
	}
}

func TestReadFile_MissingFile(t *testing.T) {
	ctx, _ := ctxFor(t)
	r := ReadFile{Ctx: ctx}
	out, _ := r.Execute(json.RawMessage(`{"path":"nope.go"}`))
	if !strings.HasPrefix(out, "Error:") {
		t.Errorf("expected error string, got %q", out)
	}
}

func TestReadFile_EscapeBlocked(t *testing.T) {
	ctx, _ := ctxFor(t)
	r := ReadFile{Ctx: ctx}
	out, _ := r.Execute(json.RawMessage(`{"path":"../../../etc/passwd"}`))
	if !strings.Contains(out, "escapes") {
		t.Errorf("expected escape error, got %q", out)
	}
}

func TestReadFile_Truncation(t *testing.T) {
	ctx, dir := ctxFor(t)
	// Write a file larger than the limit.
	big := strings.Repeat("x", 1024)
	writeFile(t, dir, "big.txt", big)
	r := ReadFile{Ctx: ctx}
	out, _ := r.Execute(json.RawMessage(`{"path":"big.txt","limit":100}`))
	// 100 bytes of content + truncation suffix. Must not be the full 1024.
	if len(out) > 250 {
		t.Errorf("expected ~100 chars + truncation note, got %d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation notice, got %q", out)
	}
}

func TestReadFile_Offset(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.txt", "ABCDEFGH")
	r := ReadFile{Ctx: ctx}
	out, _ := r.Execute(json.RawMessage(`{"path":"f.txt","offset":3}`))
	if out != "DEFGH" {
		t.Errorf("got %q, want DEFGH", out)
	}
}

func TestWriteFile_CreatesDirsAndWrites(t *testing.T) {
	ctx, dir := ctxFor(t)
	w := WriteFile{Ctx: ctx}
	out, err := w.Execute(json.RawMessage(`{"path":"deep/nested/file.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("got %q", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "deep", "nested", "file.txt"))
	if string(got) != "hello" {
		t.Errorf("file content = %q", got)
	}
}

func TestWriteFile_OverwritesExisting(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.txt", "old")
	w := WriteFile{Ctx: ctx}
	_, _ = w.Execute(json.RawMessage(`{"path":"f.txt","content":"new"}`))
	got, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(got) != "new" {
		t.Errorf("got %q, want new", got)
	}
}

func TestUpdateFile_SingleReplace(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.go", "func foo() {\n\treturn 1\n}\n")
	u := UpdateFile{Ctx: ctx}
	out, _ := u.Execute(json.RawMessage(`{"path":"f.go","old_string":"return 1","new_string":"return 2"}`))
	if !strings.Contains(out, "1 replacement") {
		t.Errorf("got %q", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f.go"))
	if !strings.Contains(string(got), "return 2") {
		t.Errorf("file not updated: %q", got)
	}
	if strings.Contains(string(got), "return 1\n") {
		t.Errorf("old string still present: %q", got)
	}
}

func TestUpdateFile_AmbiguousRejected(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.go", "foo\nfoo\n")
	u := UpdateFile{Ctx: ctx}
	out, _ := u.Execute(json.RawMessage(`{"path":"f.go","old_string":"foo","new_string":"bar"}`))
	if !strings.Contains(out, "matches 2 times") {
		t.Errorf("expected ambiguity error, got %q", out)
	}
}

func TestUpdateFile_ReplaceAll(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.go", "foo\nfoo\n")
	u := UpdateFile{Ctx: ctx}
	out, _ := u.Execute(json.RawMessage(`{"path":"f.go","old_string":"foo","new_string":"bar","replace_all":true}`))
	if !strings.Contains(out, "2 replacements") {
		t.Errorf("got %q", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f.go"))
	if string(got) != "bar\nbar\n" {
		t.Errorf("got %q", got)
	}
}

func TestUpdateFile_NotFound(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.go", "content")
	u := UpdateFile{Ctx: ctx}
	out, _ := u.Execute(json.RawMessage(`{"path":"f.go","old_string":"missing","new_string":"x"}`))
	if !strings.Contains(out, "not found") {
		t.Errorf("got %q", out)
	}
}

func TestDeleteFile_Success(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "f.txt", "x")
	d := DeleteFile{Ctx: ctx}
	out, _ := d.Execute(json.RawMessage(`{"path":"f.txt"}`))
	if !strings.Contains(out, "deleted") {
		t.Errorf("got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt")); !os.IsNotExist(err) {
		t.Errorf("file still exists")
	}
}

func TestDeleteFile_DirectoryRejected(t *testing.T) {
	ctx, dir := ctxFor(t)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	d := DeleteFile{Ctx: ctx}
	out, _ := d.Execute(json.RawMessage(`{"path":"subdir"}`))
	if !strings.Contains(out, "directory") {
		t.Errorf("expected directory rejection, got %q", out)
	}
}

func TestListFiles_WalksTree(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "a.txt", "x")
	writeFile(t, dir, "src/b.go", "y")
	writeFile(t, dir, "src/nested/c.go", "z")
	l := ListFiles{Ctx: ctx}
	out, err := l.Execute(json.RawMessage(`{"path":"."}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"a.txt", "src/b.go", "src/nested/c.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in listing:\n%s", want, out)
		}
	}
}

func TestListFiles_SkipsIgnoredDirs(t *testing.T) {
	ctx, dir := ctxFor(t)
	writeFile(t, dir, "real.txt", "x")
	writeFile(t, dir, "node_modules/dep.js", "y")
	writeFile(t, dir, ".git/HEAD", "z")
	l := ListFiles{Ctx: ctx}
	out, _ := l.Execute(json.RawMessage(`{}`))
	if strings.Contains(out, "node_modules") {
		t.Errorf("node_modules should be skipped:\n%s", out)
	}
	if strings.Contains(out, ".git/") {
		t.Errorf(".git should be skipped:\n%s", out)
	}
	if !strings.Contains(out, "real.txt") {
		t.Errorf("real.txt missing:\n%s", out)
	}
}
