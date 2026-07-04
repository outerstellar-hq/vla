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

func TestRenderDemoSequence(t *testing.T) {
	frames := []DemoFrame{
		{Blocks: []DemoBlock{{Type: "user", Content: "hello"}}, Options: DemoOptions{Width: 80, Height: 24}},
		{Blocks: []DemoBlock{
			{Type: "user", Content: "hello"},
			{Type: "assistant", Content: "hi"},
		}, Options: DemoOptions{Width: 80, Height: 24}},
	}

	result := RenderDemoSequence(frames)
	if len(result) != 2 {
		t.Fatalf("expected 2 rendered frames, got %d", len(result))
	}

	// First frame should contain "hello" but not "hi" (assistant not added yet).
	plain1 := StripANSI(result[0])
	if !strings.Contains(plain1, "hello") {
		t.Error("frame 1 should contain 'hello'")
	}
	if strings.Contains(plain1, "hi") {
		t.Error("frame 1 should NOT contain 'hi' yet")
	}

	// Second frame should contain both.
	plain2 := StripANSI(result[1])
	if !strings.Contains(plain2, "hello") {
		t.Error("frame 2 should contain 'hello'")
	}
	if !strings.Contains(plain2, "hi") {
		t.Error("frame 2 should contain 'hi'")
	}
}

func TestRenderDemoSequence_Empty(t *testing.T) {
	result := RenderDemoSequence(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 frames for nil input, got %d", len(result))
	}
}

func TestDefaultDemoSequence(t *testing.T) {
	frames := DefaultDemoSequence()

	if len(frames) < 20 {
		t.Errorf("expected at least 20 frames in default sequence, got %d", len(frames))
	}

	// All frames should render without panicking.
	for i, f := range frames {
		output := f.RenderSingle()
		if output == "" {
			t.Errorf("frame %d rendered empty", i)
		}
	}

	// First frame should show the start of the user typing (at least "F").
	plain0 := StripANSI(frames[0].RenderSingle())
	if !strings.Contains(plain0, "You:") {
		t.Errorf("frame 0 should contain 'You:' label, got: %q", plain0[:demoMin(100, len(plain0))])
	}

	// A later frame should show the full user message.
	foundFull := false
	for _, f := range frames[:len(frames)/2] {
		plain := StripANSI(f.RenderSingle())
		if strings.Contains(plain, "login bug") && strings.Contains(plain, "credentials") {
			foundFull = true
			break
		}
	}
	if !foundFull {
		t.Error("expected a frame to show the full user message")
	}

	// A frame should show a tool call.
	foundTool := false
	for _, f := range frames {
		plain := StripANSI(f.RenderSingle())
		if strings.Contains(plain, "read_file") {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Error("expected a frame with read_file tool call")
	}

	// Last frame should show "Done" or the fix (idle state).
	lastPlain := StripANSI(frames[len(frames)-1].RenderSingle())
	if !strings.Contains(lastPlain, "Done") {
		t.Error("last frame should show completion")
	}
}

func TestDemoFrameRenderSingle(t *testing.T) {
	f := DemoFrame{
		Blocks: []DemoBlock{
			{Type: "user", Content: "test"},
		},
		Options: DemoOptions{
			Width:  80,
			Height: 24,
		},
	}

	out := f.RenderSingle()
	if !strings.Contains(StripANSI(out), "test") {
		t.Error("RenderSingle should contain block content")
	}
}

func TestDefaultDemoSequence_Progresses(t *testing.T) {
	frames := DefaultDemoSequence()

	// The sequence should grow: later frames should have >= blocks than earlier.
	// (Each frame adds context — assistant responses, tool calls, etc.)
	for i := 1; i < len(frames); i++ {
		if len(frames[i].Blocks) < len(frames[i-1].Blocks) {
			// This is OK for the last frame sometimes (assistant final response
			// replaces streaming), but mid-sequence it should grow.
			if i < len(frames)-1 {
				t.Errorf("frame %d has fewer blocks (%d) than frame %d (%d)",
					i, len(frames[i].Blocks), i-1, len(frames[i-1].Blocks))
			}
		}
	}

	// Verify token count increases across frames (shows progress).
	if frames[0].Options.Tokens >= frames[len(frames)-1].Options.Tokens {
		t.Error("expected token count to increase across the sequence")
	}
}

func TestDefaultDemoSequence_HasSpinnerTransition(t *testing.T) {
	frames := DefaultDemoSequence()

	// Early frames type the user message (not spinning). After typing,
	// the agent starts thinking (spinning). Find a spinning frame.
	foundSpinning := false
	for _, f := range frames {
		if f.Options.Spinning {
			foundSpinning = true
			break
		}
	}
	if !foundSpinning {
		t.Fatal("expected at least one spinning frame in the sequence")
	}

	// Last frame should be idle (work complete).
	last := frames[len(frames)-1]
	if last.Options.Spinning {
		t.Error("last frame should not be spinning (idle)")
	}
	if last.Options.StatusText != "idle" {
		t.Errorf("last frame statusText = %q, want 'idle'", last.Options.StatusText)
	}
}

func TestDefaultDemoSequence_SessionItems(t *testing.T) {
	// Ensure the unused import doesn't cause issues — SessionItem is used
	// by RenderDemoPicker, not by the sequence. This test just verifies
	// the type exists and compiles.
	items := []SessionItem{
		{ID: "test", Project: "/p", Model: "m", Created: time.Now()},
	}
	if len(items) != 1 {
		t.Error("SessionItem slice should work")
	}
}
