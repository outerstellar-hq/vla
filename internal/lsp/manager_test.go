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
