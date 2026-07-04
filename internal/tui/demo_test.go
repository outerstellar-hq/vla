package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDemoFrame_MainView(t *testing.T) {
	blocks := []DemoBlock{
		{Type: "user", Content: "Hello VLA"},
		{Type: "assistant", Content: "Hi! How can I help?"},
		{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"main.go"}`, ToolResult: "package main", Status: "done"},
	}

	frame := RenderDemoFrame(blocks, DemoOptions{
		ModelName: "gpt-4o",
		SessionID: "test123",
		ToolCount: 10,
		Tokens:    500,
		Width:     80,
		Height:    24,
	})

	plain := StripANSI(frame)
	if !strings.Contains(plain, "Hello VLA") {
		t.Error("frame should contain user message")
	}
	if !strings.Contains(plain, "Hi! How can I help?") {
		t.Error("frame should contain assistant message")
	}
	if !strings.Contains(plain, "read_file") {
		t.Error("frame should contain tool name")
	}
	if !strings.Contains(plain, "gpt-4o") {
		t.Error("frame should contain model name in status bar")
	}
}

func TestRenderDemoFrame_Streaming(t *testing.T) {
	blocks := []DemoBlock{
		{Type: "user", Content: "Write a function"},
	}
	frame := RenderDemoFrame(blocks, DemoOptions{
		Streaming: "func hello() {",
		Spinning:  true,
		Width:     80,
		Height:    24,
	})

	plain := StripANSI(frame)
	if !strings.Contains(plain, "func hello() {") {
		t.Error("frame should contain streaming text")
	}
	if !strings.Contains(plain, "▌") {
		t.Error("frame should show streaming cursor")
	}
}

func TestRenderDemoFrame_ToolExpanded(t *testing.T) {
	blocks := []DemoBlock{
		{
			Type:       "tool",
			ToolName:   "search",
			ToolArgs:   `{"query":"TODO"}`,
			ToolResult: "main.go:42:// TODO fix this",
			Status:     "done",
			Expanded:   true,
		},
	}
	frame := RenderDemoFrame(blocks, DemoOptions{
		Width:  80,
		Height: 24,
	})

	plain := StripANSI(frame)
	if !strings.Contains(plain, "args:") {
		t.Error("expanded tool block should show 'args:' section")
	}
	if !strings.Contains(plain, "result:") {
		t.Error("expanded tool block should show 'result:' section")
	}
}

func TestRenderDemoDiff(t *testing.T) {
	blocks := []DemoBlock{
		{Type: "user", Content: "Fix this"},
		{
			Type:     "tool",
			ToolName: "update_file",
			Status:   "running",
		},
	}
	diffArgs := `{"path":"foo.go","old_string":"old code","new_string":"new code"}`

	frame := RenderDemoDiff(blocks, "update_file", diffArgs, DemoOptions{
		Width:  100,
		Height: 24,
	})

	plain := StripANSI(frame)
	if !strings.Contains(plain, "update_file") {
		t.Error("diff frame should show tool name in header")
	}
}

func TestRenderDemoPicker(t *testing.T) {
	items := []SessionItem{
		{ID: "20260704T120000Z", Project: "/proj", Model: "gpt-4o", Created: time.Now()},
		{ID: "20260704T090000Z", Project: "/proj", Model: "claude-3", Created: time.Now().Add(-4 * time.Hour)},
	}

	frame := RenderDemoPicker(items, DemoOptions{
		Width:  80,
		Height: 24,
	})

	plain := StripANSI(frame)
	if !strings.Contains(plain, "Switch Session") {
		t.Error("picker frame should contain 'Switch Session' title")
	}
	if !strings.Contains(plain, "20260704T120000Z") {
		t.Error("picker frame should show session ID")
	}
	if !strings.Contains(plain, "gpt-4o") {
		t.Error("picker frame should show model name")
	}
	if !strings.Contains(plain, "2 session(s)") {
		t.Error("picker frame should show session count")
	}
}

func TestDemoBlockMapping(t *testing.T) {
	tests := []struct {
		demo   DemoBlock
		wantTy blockType
	}{
		{DemoBlock{Type: "user"}, blockUser},
		{DemoBlock{Type: "assistant"}, blockAssistant},
		{DemoBlock{Type: "tool"}, blockTool},
		{DemoBlock{Type: "system"}, blockSystem},
		{DemoBlock{Type: "error"}, blockError},
		{DemoBlock{Type: "unknown"}, blockUser}, // defaults to blockUser (0)
	}
	for _, tt := range tests {
		got := demoBlockToBlock(tt.demo)
		if got.typ != tt.wantTy {
			t.Errorf("DemoBlock{Type:%q} → block type %d, want %d", tt.demo.Type, got.typ, tt.wantTy)
		}
	}
}

func TestDemoBlockStatusMapping(t *testing.T) {
	tests := []struct {
		status     string
		wantStatus toolStatus
	}{
		{"done", toolDone},
		{"running", toolRunning},
		{"denied", toolDenied},
		{"blocked", toolBlocked},
		{"unknown", toolRunning}, // defaults to zero-value (running)
	}
	for _, tt := range tests {
		db := DemoBlock{Type: "tool", Status: tt.status}
		got := demoBlockToBlock(db)
		if got.status != tt.wantStatus {
			t.Errorf("DemoBlock{Status:%q} → tool status %d, want %d", tt.status, got.status, tt.wantStatus)
		}
	}
}

func TestStripANSI(t *testing.T) {
	// \x1b[32mgreen\x1b[0m → "green"
	input := "\x1b[32mgreen\x1b[0m text"
	got := StripANSI(input)
	if got != "green text" {
		t.Errorf("StripANSI = %q, want %q", got, "green text")
	}
}

func TestStripANSI_NoEscapeCodes(t *testing.T) {
	got := StripANSI("plain text")
	if got != "plain text" {
		t.Errorf("StripANSI = %q, want %q", got, "plain text")
	}
}

func TestRenderDemoFrame_Defaults(t *testing.T) {
	// Should not panic with zero-value options.
	frame := RenderDemoFrame(nil, DemoOptions{})
	if frame == "" {
		t.Error("frame should not be empty even with nil blocks")
	}
}
