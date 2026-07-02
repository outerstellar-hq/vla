package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_CreatesTranscriptFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithDir(dir), WithModel("gpt-4o"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(dir, s.ID()+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("transcript file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("transcript file is empty; expected metadata line")
	}

	f, _ := os.Open(path)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("transcript has no lines")
	}
	var meta struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Model   string `json:"model"`
		Cwd     string `json:"cwd"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		t.Fatalf("first line not valid JSON: %v", err)
	}
	if meta.Type != "session" {
		t.Errorf("metadata type = %q, want %q", meta.Type, "session")
	}
	if meta.ID == "" {
		t.Error("metadata id is empty")
	}
	if meta.Model != "gpt-4o" {
		t.Errorf("metadata model = %q, want gpt-4o", meta.Model)
	}
}

func TestAppend_Turn(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(WithDir(dir), WithModel("gpt-4o"))

	err := s.Append(map[string]any{
		"type":      "turn",
		"role":      "user",
		"content":   "hello",
		"timestamp": "2026-07-02T15:03:01Z",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	turns, meta, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if meta["model"] != "gpt-4o" {
		t.Errorf("meta model = %v", meta["model"])
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0]["content"] != "hello" {
		t.Errorf("turn content = %v", turns[0]["content"])
	}
}

func TestRead_ExistingSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-01-01T000000Z.json")
	content := "{\"type\":\"session\",\"id\":\"2026-01-01T000000Z\",\"cwd\":\"/tmp\",\"model\":\"gpt-4o\",\"created\":\"2026-01-01T00:00:00Z\"}\n" +
		"{\"type\":\"turn\",\"role\":\"user\",\"content\":\"hi\",\"timestamp\":\"2026-01-01T00:00:01Z\"}\n" +
		"{\"type\":\"turn\",\"role\":\"assistant\",\"content\":\"hello\",\"timestamp\":\"2026-01-01T00:00:02Z\"}\n"
	_ = os.WriteFile(path, []byte(content), 0644)

	s, err := Open(filepath.Join(dir, "2026-01-01T000000Z.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	turns, meta, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if meta["cwd"] != "/tmp" {
		t.Errorf("meta cwd = %v", meta["cwd"])
	}
}

func TestSessionsDir(t *testing.T) {
	dir := SessionsDir()
	if dir == "" {
		t.Fatal("SessionsDir returned empty string")
	}
	if !strings.HasSuffix(filepath.ToSlash(dir), ".vla/sessions") {
		t.Errorf("SessionsDir = %q, want path ending in .vla/sessions", dir)
	}
}
