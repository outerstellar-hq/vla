package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHooks(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".vla")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(content), 0644)
}

func TestLoad_NoFile(t *testing.T) {
	m := Load(t.TempDir())
	if m.HasHooks() {
		t.Error("expected no hooks without config file")
	}
}

func TestLoad_WithHooks(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "after_tool", "tool": "write_file", "command": "echo wrote"},
			{"event": "on_session_start", "command": "echo started"}
		]
	}`)
	m := Load(dir)
	if !m.HasHooks() {
		t.Fatal("expected hooks")
	}
	if len(m.hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(m.hooks))
	}
}

func TestRun_MatchingEvent(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "after_tool", "tool": "write_file", "command": "true"}
		]
	}`)
	m := Load(dir)
	// "true" always exits 0, should not error.
	if err := m.Run(EventAfterTool, "write_file", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_NonMatchingTool(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "after_tool", "tool": "write_file", "command": "false"}
		]
	}`)
	m := Load(dir)
	// Different tool — hook should NOT run.
	if err := m.Run(EventAfterTool, "read_file", nil); err != nil {
		t.Errorf("hook for different tool should not run: %v", err)
	}
}

func TestRun_BeforeToolBlocksOnError(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "before_tool", "command": "false"}
		]
	}`)
	m := Load(dir)
	// "false" exits 1, before_tool should block.
	err := m.Run(EventBeforeTool, "write_file", nil)
	if err == nil {
		t.Error("expected before_tool to block when hook exits non-zero")
	}
}

func TestRun_AfterToolDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "after_tool", "command": "false"}
		]
	}`)
	m := Load(dir)
	// after_tool hook fails but should NOT block (non-blocking event).
	if err := m.Run(EventAfterTool, "write_file", nil); err != nil {
		t.Errorf("after_tool should not block: %v", err)
	}
}

func TestRun_NoMatchingEvent(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "on_session_start", "command": "false"}
		]
	}`)
	m := Load(dir)
	// Different event — should be a no-op.
	if err := m.Run(EventAfterTool, "write_file", nil); err != nil {
		t.Errorf("should not error for non-matching event: %v", err)
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeHooks(t, dir, `{bad json`)
	m := Load(dir)
	if m.HasHooks() {
		t.Error("malformed JSON should result in no hooks")
	}
}

func TestRun_EnvVarsPassed(t *testing.T) {
	// Verify that hooks run without error when env vars are provided.
	// (Full env-var content verification requires Unix-specific file redirects.)
	dir := t.TempDir()
	writeHooks(t, dir, `{
		"hooks": [
			{"event": "after_tool", "tool": "write_file", "command": "true"}
		]
	}`)
	m := Load(dir)
	if err := m.Run(EventAfterTool, "write_file", map[string]string{"VLA_FILE": "test.go"}); err != nil {
		t.Errorf("should not error with env: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
