package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/memory"
	"github.com/abrandt/vla/internal/session"
)

func TestSystemPrompt(t *testing.T) {
	p := SystemPrompt()
	if !strings.Contains(p, "VLA") {
		t.Error("prompt should mention VLA")
	}
	if !strings.Contains(p, "read_file") {
		t.Error("prompt should mention tools")
	}
	if !strings.Contains(p, "memory_save") {
		t.Error("prompt should mention memory tools")
	}
	if len(p) < 100 {
		t.Error("prompt suspiciously short")
	}
}

func TestLoadTranscriptMessages_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	sess, err := session.New(session.WithDir(dir), session.WithModel("test"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	// Write some turns manually.
	_ = sess.Append(map[string]any{
		"type": "turn", "role": "user", "content": "hello",
	})
	_ = sess.Append(map[string]any{
		"type": "turn", "role": "assistant", "content": "hi there",
		"tool_calls": []any{
			map[string]any{
				"id":   "c1",
				"type": "function",
				"function": map[string]any{
					"name":      "echo",
					"arguments": `{"text":"hi"}`,
				},
			},
		},
	})
	_ = sess.Append(map[string]any{
		"type": "turn", "role": "tool", "tool_call_id": "c1", "content": "hi",
	})

	msgs, err := LoadTranscriptMessages(sess)
	if err != nil {
		t.Fatalf("LoadTranscriptMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != agent.RoleUser || msgs[0].Content != "hello" {
		t.Errorf("msg 0: %+v", msgs[0])
	}
	if msgs[1].Role != agent.RoleAssistant {
		t.Errorf("msg 1 role: %v", msgs[1].Role)
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msgs[1].ToolCalls))
	}
	tc := msgs[1].ToolCalls[0]
	if tc.ID != "c1" || tc.Function.Name != "echo" || tc.Function.Arguments != `{"text":"hi"}` {
		t.Errorf("tool call round-trip failed: %+v", tc)
	}
	if msgs[2].Role != agent.RoleTool || msgs[2].ToolCallID != "c1" || msgs[2].Content != "hi" {
		t.Errorf("msg 2: %+v", msgs[2])
	}
}

func TestLoadTranscriptMessages_Empty(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.New(session.WithDir(dir), session.WithModel("test"))
	msgs, err := LoadTranscriptMessages(sess)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestNewMemoryInjector_WithResults(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	_ = store.Save(&memory.Memory{Project: "p", Content: "The database uses PostgreSQL"})

	injector := NewMemoryInjector(store, nil, func() string { return "p" })

	view := []agent.Message{{Role: agent.RoleUser, Content: "database"}}
	result := injector(view, "database")

	// Should prepend a system message with the memory.
	if len(result) < 2 {
		t.Fatalf("expected injected + original, got %d messages", len(result))
	}
	if result[0].Role != agent.RoleSystem {
		t.Errorf("expected system role, got %v", result[0].Role)
	}
	if !strings.Contains(result[0].Content, "PostgreSQL") {
		t.Errorf("injected memory missing content: %q", result[0].Content)
	}
}

func TestNewMemoryInjector_NoResults(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	injector := NewMemoryInjector(store, nil, func() string { return "p" })

	view := []agent.Message{{Role: agent.RoleUser, Content: "test"}}
	result := injector(view, "test")

	// No memories → return view unchanged.
	if len(result) != len(view) {
		t.Errorf("expected %d messages (no injection), got %d", len(view), len(result))
	}
}

func TestNewMemoryInjector_EmptyQuery(t *testing.T) {
	store := memory.NewStore(t.TempDir())
	_ = store.Save(&memory.Memory{Project: "p", Content: "something"})
	injector := NewMemoryInjector(store, nil, func() string { return "p" })

	view := []agent.Message{{Role: agent.RoleUser, Content: "x"}}
	result := injector(view, "")

	// Empty query → return view unchanged.
	if len(result) != len(view) {
		t.Errorf("expected no injection for empty query, got %d", len(result))
	}
}

func TestArchitectPrompt(t *testing.T) {
	p := ArchitectPrompt()
	if !strings.Contains(p, "senior") {
		t.Error("architect prompt should mention 'senior'")
	}
	if !strings.Contains(p, "technical debt") {
		t.Error("architect prompt should mention 'technical debt'")
	}
	if !strings.Contains(p, "anti-pattern") {
		t.Error("architect prompt should mention 'anti-pattern'")
	}
	if !strings.Contains(p, "read_file") {
		t.Error("architect prompt should still list tools")
	}
}

func TestPromptForPersona(t *testing.T) {
	// Default (empty) → standard prompt.
	p := PromptForPersona("")
	if !strings.Contains(p, "agentic coding harness") {
		t.Error("default persona should contain standard prompt text")
	}

	// Architect → architect prompt.
	p = PromptForPersona("architect")
	if !strings.Contains(p, "senior") {
		t.Error("architect persona should contain architect prompt text")
	}

	// Unknown → falls back to default.
	p = PromptForPersona("unknown")
	if !strings.Contains(p, "agentic coding harness") {
		t.Error("unknown persona should fall back to default")
	}
}

func TestLoadPersonaFile(t *testing.T) {
	// Empty path → empty string.
	if LoadPersonaFile("") != "" {
		t.Error("empty path should return empty string")
	}

	// Non-existent file → empty string.
	if LoadPersonaFile("/nonexistent/file.md") != "" {
		t.Error("non-existent file should return empty string")
	}

	// Real file → contents.
	dir := t.TempDir()
	path := filepath.Join(dir, "persona.md")
	content := "You are a custom persona with specific instructions."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := LoadPersonaFile(path)
	if got != content {
		t.Errorf("LoadPersonaFile = %q, want %q", got, content)
	}
}
