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

// TestAppendRead_RoundTrip verifies that every shape of turn we write can be
// read back losslessly. This is the contract the agent loop relies on:
// anything not preserved across an Append→Read cycle is lost to the LLM.
func TestAppendRead_RoundTrip_AllTurnTypes(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(WithDir(dir), WithModel("gpt-4o"))

	turns := []map[string]any{
		// Plain user text.
		{"type": "turn", "role": "user", "content": "hello world", "timestamp": "2026-07-02T15:03:01Z"},
		// Assistant text + tool call (with nested arguments JSON string).
		{"type": "turn", "role": "assistant", "content": "calling echo", "tool_calls": []any{
			map[string]any{
				"id":   "call_1",
				"type": "function",
				"function": map[string]any{
					"name":      "echo",
					"arguments": `{"text":"hi"}`,
				},
			},
		}, "timestamp": "2026-07-02T15:03:02Z"},
		// Tool result.
		{"type": "turn", "role": "tool", "tool_call_id": "call_1", "content": "hi", "timestamp": "2026-07-02T15:03:02Z"},
		// Empty content edge case.
		{"type": "turn", "role": "assistant", "content": "", "timestamp": "2026-07-02T15:03:03Z"},
		// Unicode content.
		{"type": "turn", "role": "user", "content": "héllo 世界 🚀", "timestamp": "2026-07-02T15:03:04Z"},
	}
	for _, turn := range turns {
		if err := s.Append(turn); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	got, _, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(turns) {
		t.Fatalf("round-trip lost turns: wrote %d, read %d", len(turns), len(got))
	}

	// Verify the critical fields survived the JSON encode/decode cycle.
	if got[0]["content"] != "hello world" {
		t.Errorf("user content = %v", got[0]["content"])
	}
	assistant := got[1]
	tcs, ok := assistant["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Fatalf("tool_calls round-trip failed: %v", assistant["tool_calls"])
	}
	tc := tcs[0].(map[string]any)
	fn := tc["function"].(map[string]any)
	if fn["name"] != "echo" {
		t.Errorf("tool call name = %v", fn["name"])
	}
	if fn["arguments"] != `{"text":"hi"}` {
		t.Errorf("tool call arguments = %v (must be exact JSON string)", fn["arguments"])
	}
	if got[2]["tool_call_id"] != "call_1" {
		t.Errorf("tool_call_id = %v", got[2]["tool_call_id"])
	}
	if got[4]["content"] != "héllo 世界 🚀" {
		t.Errorf("unicode content = %v", got[4]["content"])
	}
}

// TestResume_PreservesIDAndCWD verifies that Open recovers the session ID and
// CWD recorded in the metadata line, which the CLI needs to chdir back.
func TestResume_PreservesIDAndCWD(t *testing.T) {
	dir := t.TempDir()
	original, _ := New(WithDir(dir), WithModel("gpt-4o"))
	original.Append(map[string]any{
		"type": "turn", "role": "user", "content": "first", "timestamp": "2026-07-02T15:03:01Z",
	})

	// Reopen the same file.
	resumed, err := Open(original.Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if resumed.ID() != original.ID() {
		t.Errorf("ID = %q, want %q", resumed.ID(), original.ID())
	}
	if resumed.CWD() != original.CWD() {
		t.Errorf("CWD = %q, want %q", resumed.CWD(), original.CWD())
	}

	turns, _, err := resumed.Read()
	if err != nil {
		t.Fatalf("Read resumed: %v", err)
	}
	if len(turns) != 1 || turns[0]["content"] != "first" {
		t.Errorf("resumed turns = %v", turns)
	}

	// Appending after resume must write to the same file.
	resumed.Append(map[string]any{
		"type": "turn", "role": "user", "content": "second", "timestamp": "2026-07-02T15:03:02Z",
	})
	turns2, _, _ := resumed.Read()
	if len(turns2) != 2 {
		t.Errorf("after resume-append, expected 2 turns, got %d", len(turns2))
	}
}

// TestRead_EmptyTranscript verifies the error when a transcript has a metadata
// line but no turns — must return an empty slice, not error.
func TestRead_MetadataOnly_NoTurns(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(WithDir(dir), WithModel("gpt-4o"))
	turns, _, err := s.Read()
	if err != nil {
		t.Fatalf("Read on metadata-only transcript: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(turns))
	}
}
