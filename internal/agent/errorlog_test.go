package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogToolError_WritesNDJSON(t *testing.T) {
	// Use a temp HOME so we don't pollute the real log.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	LogToolError("sess-123", "write_file", `{"path":"/foo"}`, "Error: permission denied")

	path := filepath.Join(dir, ".vla", "logs", "tool-errors.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}

	var entry errorLogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("expected valid JSON: %v\nraw: %s", err, string(data))
	}

	if entry.Session != "sess-123" {
		t.Errorf("session: got %q, want sess-123", entry.Session)
	}
	if entry.Tool != "write_file" {
		t.Errorf("tool: got %q, want write_file", entry.Tool)
	}
	if entry.Error != "Error: permission denied" {
		t.Errorf("error: got %q", entry.Error)
	}
	if entry.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
}

func TestLogToolError_AppendsMultiple(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	LogToolError("s1", "read_file", "{}", "Error: not found")
	LogToolError("s1", "write_file", "{}", "Error: disk full")
	LogToolError("s2", "delete_file", "{}", "Error: locked")

	path := filepath.Join(dir, ".vla", "logs", "tool-errors.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d", len(lines))
	}

	// Each line should be valid JSON.
	for i, line := range lines {
		var entry errorLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestLogToolError_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	// Directory doesn't exist yet.
	logPath := filepath.Join(dir, ".vla", "logs")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("log dir should not exist yet")
	}

	LogToolError("s1", "echo", "{}", "Error: test")

	// Directory and file should now exist.
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log dir should be created: %v", err)
	}
}

func TestMaxConsecutiveErrors(t *testing.T) {
	if MaxConsecutiveErrors != 3 {
		t.Errorf("MaxConsecutiveErrors = %d, want 3", MaxConsecutiveErrors)
	}
}
