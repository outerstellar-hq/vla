package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSpecs(t *testing.T) {
	specs := DefaultSpecs()
	if specs[LangPython].Command == "" {
		t.Error("missing Python server spec")
	}
	if specs[LangGo].Command == "" {
		t.Error("missing Go server spec")
	}
	// Verify all new languages have specs.
	for _, lang := range []Language{
		LangCSS, LangHTML, LangRust, LangRuby, LangC,
		LangDart, LangLua, LangElixir, LangScala, LangSwift,
	} {
		if specs[lang].Command == "" {
			t.Errorf("missing spec for language %q", lang)
		}
	}
}

func TestDefaultSpecs_AllHaveCommands(t *testing.T) {
	specs := DefaultSpecs()
	if len(specs) < 17 {
		t.Errorf("expected at least 17 language specs, got %d", len(specs))
	}
	for lang, spec := range specs {
		if spec.Command == "" {
			t.Errorf("spec for %q has empty command", lang)
		}
		if spec.Language != lang {
			t.Errorf("spec for %q has wrong language field: %q", lang, spec.Language)
		}
	}
}

func TestInferLanguage_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644)
	if got := InferLanguage(dir); got != LangRust {
		t.Errorf("expected Rust, got %q", got)
	}
}

func TestInferLanguage_Ruby(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'"), 0644)
	if got := InferLanguage(dir); got != LangRuby {
		t.Errorf("expected Ruby, got %q", got)
	}
}

func TestInferLanguage_Dart(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pubspec.yaml"), []byte("name: myapp"), 0644)
	if got := InferLanguage(dir); got != LangDart {
		t.Errorf("expected Dart, got %q", got)
	}
}

func TestInferLanguage_Elixir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mix.exs"), []byte("defmodule M do end"), 0644)
	if got := InferLanguage(dir); got != LangElixir {
		t.Errorf("expected Elixir, got %q", got)
	}
}

func TestInferLanguage_Scala(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "build.sbt"), []byte("name := \"test\""), 0644)
	if got := InferLanguage(dir); got != LangScala {
		t.Errorf("expected Scala, got %q", got)
	}
}

func TestInferLanguage_Swift(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Package.swift"), []byte("// swift"), 0644)
	if got := InferLanguage(dir); got != LangSwift {
		t.Errorf("expected Swift, got %q", got)
	}
}

func TestInferLanguage_C(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.0)"), 0644)
	if got := InferLanguage(dir); got != LangC {
		t.Errorf("expected C/C++, got %q", got)
	}
}

func TestInferLanguage_Lua(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".luarc.json"), []byte("{}"), 0644)
	if got := InferLanguage(dir); got != LangLua {
		t.Errorf("expected Lua, got %q", got)
	}
}

func TestInferLanguage_CSS(t *testing.T) {
	// CSS/HTML don't have project marker files — they're detected by
	// file extension at the tool level, not InferLanguage. Verify
	// InferLanguage returns empty for a CSS-only directory.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "style.css"), []byte("body {}"), 0644)
	if got := InferLanguage(dir); got != "" {
		t.Errorf("CSS-only dir should return empty (no marker), got %q", got)
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager(DefaultSpecs())
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if len(m.specs) == 0 {
		t.Error("manager has no specs")
	}
}

func TestManager_GetMissingServer(t *testing.T) {
	m := NewManager(map[Language]ServerSpec{
		LangPython: {Language: LangPython, Command: "nonexistent-lsp-server-xyz", Args: []string{"--stdio"}},
	})
	_, err := m.Get(LangPython, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing server, got nil")
	}
}

func TestManager_GetUnknownLanguage(t *testing.T) {
	m := NewManager(DefaultSpecs())
	_, err := m.Get(Language("ruby"), t.TempDir())
	if err == nil {
		t.Fatal("expected error for unknown language")
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager(DefaultSpecs())
	m.Close()
}

func TestInferLanguage_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0644)
	if lang := InferLanguage(dir); lang != LangPython {
		t.Errorf("expected LangPython, got %q", lang)
	}
}

func TestInferLanguage_Pyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0644)
	if lang := InferLanguage(dir); lang != LangPython {
		t.Errorf("expected LangPython, got %q", lang)
	}
}

func TestInferLanguage_Unknown(t *testing.T) {
	lang := InferLanguage(t.TempDir())
	if lang != "" {
		t.Errorf("expected empty, got %q", lang)
	}
}

// TestClient_Notify verifies the Notify method writes a correctly framed
// message. Uses bytes.Buffer as the writer (non-blocking, inspectable).
func TestClient_Notify(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(errReader{}, &buf)
	defer c.Close()

	if err := c.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "file:///test.go"},
	}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Parse the framed message from the buffer.
	msg, err := readMessage(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	var notif map[string]any
	if err := json.Unmarshal(msg, &notif); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if notif["method"] != "textDocument/didOpen" {
		t.Errorf("method = %v", notif["method"])
	}
}

// TestClient_OnNotificationUnregister verifies registration/unregistration
// doesn't panic or leak goroutines.
func TestClient_OnNotificationUnregister(t *testing.T) {
	c := NewClient(errReader{}, &bytes.Buffer{})
	defer c.Close()

	unreg := c.OnNotification("test/method", func(json.RawMessage) {})
	if unreg == nil {
		t.Fatal("OnNotification returned nil")
	}
	unreg()
}

// errReader is an io.Reader that immediately returns EOF.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("eof") }

// Keep netPipe for potential future use.
var _ = net.Pipe
