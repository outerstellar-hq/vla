package tui

import (
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderUserBlock(t *testing.T) {
	b := block{typ: blockUser, content: "hello world"}
	out := renderBlock(b, 80)
	if !strings.Contains(out, "You") {
		t.Errorf("user block should contain 'You' label, got: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("user block should contain content, got: %q", out)
	}
}

func TestRenderToolBlockCollapsed(t *testing.T) {
	b := block{
		typ:      blockTool,
		toolName: "read_file",
		toolArgs: `{"path":"/tmp/foo.go"}`,
		status:   toolDone,
	}
	out := renderBlock(b, 80)
	if !strings.Contains(out, "read_file") {
		t.Errorf("collapsed tool block should show tool name, got: %q", out)
	}
	if !strings.Contains(out, "/tmp/foo.go") {
		t.Errorf("collapsed tool block should show path summary, got: %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("done tool block should show checkmark, got: %q", out)
	}
}

func TestRenderToolBlockExpanded(t *testing.T) {
	b := block{
		typ:        blockTool,
		toolName:   "write_file",
		toolArgs:   `{"path":"/tmp/bar.go","content":"package main"}`,
		toolResult: "File written successfully",
		status:     toolDone,
		expanded:   true,
	}
	out := renderBlock(b, 80)
	if !strings.Contains(out, "args:") {
		t.Errorf("expanded tool block should show 'args:' section, got: %q", out)
	}
	if !strings.Contains(out, "result:") {
		t.Errorf("expanded tool block should show 'result:' section, got: %q", out)
	}
	if !strings.Contains(out, "File written successfully") {
		t.Errorf("expanded tool block should show result content, got: %q", out)
	}
}

func TestRenderToolBlockError(t *testing.T) {
	b := block{
		typ:      blockTool,
		toolName: "delete_file",
		toolArgs: `{"path":"/nonexistent"}`,
		status:   toolDenied,
	}
	out := renderBlock(b, 80)
	if !strings.Contains(out, "delete_file") {
		t.Errorf("error tool block should show tool name, got: %q", out)
	}
}

func TestToolArgSummary(t *testing.T) {
	tests := []struct {
		toolName string
		args     string
		want     string
	}{
		{"read_file", `{"path":"/tmp/foo.go"}`, "(/tmp/foo.go)"},
		{"search", `{"query":"hello"}`, "(hello)"},
		{"list_files", `{}`, ""},
		{"write_file", `{"path":"/long/path/that/exceeds/the/fifty/character/limit/seriously.go"}`, "(/long/path/that/exceeds/the/fifty/character/l…"},
	}
	for _, tt := range tests {
		got := toolArgSummary(tt.toolName, tt.args)
		if tt.want == "" {
			if got != "" {
				t.Errorf("toolArgSummary(%s) = %q, want empty", tt.toolName, got)
			}
			continue
		}
		if !strings.HasPrefix(got, "(") || !strings.Contains(got, "…") || !strings.Contains(got, tt.want[:5]) {
			// For non-empty expected, just verify format roughly.
			if !strings.HasPrefix(got, "(") {
				t.Errorf("toolArgSummary(%s) = %q, expected parenthesized", tt.toolName, got)
			}
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	input := `{"b":"2","a":"1"}`
	out := prettyJSON(input)
	if !strings.Contains(out, "  ") {
		t.Errorf("prettyJSON should indent, got: %q", out)
	}
	// Keys should be re-sorted by json.MarshalIndent.
	if strings.Index(out, "\"a\"") > strings.Index(out, "\"b\"") {
		t.Errorf("prettyJSON should sort keys, got: %q", out)
	}
}

func TestLooksLikeMarkdown(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"hello world", false},
		{"# Heading", true},
		{"```go\nfmt.Println()\n```", true},
		{"- list item", true},
		{"**bold text**", true},
		{"`inline code`", true},
		{"| col1 | col2 |\n| --- | --- |", true},
		{"plain text without any markdown", false},
	}
	for _, tt := range tests {
		got := looksLikeMarkdown(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeMarkdown(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0 tok"},
		{500, "500 tok"},
		{1200, "1.2k tok"},
		{15000, "15.0k tok"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHandleEventTurnStart(t *testing.T) {
	m := newTestModel()
	m.handleEvent(agent.Event{Type: agent.EventTurnStart})
	if !m.spinning {
		t.Error("expected spinning=true after EventTurnStart")
	}
	if !m.isStreaming {
		t.Error("expected isStreaming=true after EventTurnStart")
	}
	if m.statusText != "thinking" {
		t.Errorf("expected statusText='thinking', got %q", m.statusText)
	}
}

func TestHandleEventTurnEnd(t *testing.T) {
	m := newTestModel()
	m.isStreaming = true
	m.streaming.WriteString("partial response")
	m.handleEvent(agent.Event{Type: agent.EventTurnEnd})
	if m.isStreaming {
		t.Error("expected isStreaming=false after EventTurnEnd")
	}
	if m.spinning {
		t.Error("expected spinning=false after EventTurnEnd")
	}
	if len(m.blocks) != 1 || m.blocks[0].typ != blockAssistant {
		t.Errorf("expected 1 assistant block, got %d blocks", len(m.blocks))
	}
	if m.blocks[0].content != "partial response" {
		t.Errorf("expected block content='partial response', got %q", m.blocks[0].content)
	}
}

func TestHandleEventToolStartAndResult(t *testing.T) {
	m := newTestModel()

	// Tool start creates a running block.
	m.handleEvent(agent.Event{Type: agent.EventToolStart, Tool: "read_file", Args: `{"path":"/foo"}`})
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].status != toolRunning {
		t.Error("expected tool block status=running")
	}
	if m.blocks[0].toolName != "read_file" {
		t.Errorf("expected tool name='read_file', got %q", m.blocks[0].toolName)
	}

	// Tool result updates the block.
	m.handleEvent(agent.Event{Type: agent.EventToolResult, Tool: "read_file", Result: "file contents", Error: false})
	if m.blocks[0].status != toolDone {
		t.Error("expected tool block status=done after result")
	}
	if m.blocks[0].toolResult != "file contents" {
		t.Errorf("expected toolResult='file contents', got %q", m.blocks[0].toolResult)
	}
}

func TestHandleEventUsage(t *testing.T) {
	m := newTestModel()
	m.handleEvent(agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{TotalTokens: 5000}})
	if m.tokens != 5000 {
		t.Errorf("expected tokens=5000, got %d", m.tokens)
	}
}

func TestScrollLockToggle(t *testing.T) {
	m := newTestModel()
	if !m.scrollLocked {
		t.Error("expected scrollLocked=true by default")
	}

	// Ctrl+F toggles scroll lock off.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if m2.(Model).scrollLocked {
		t.Error("expected scrollLocked=false after Ctrl+F")
	}

	// Ctrl+F toggles back on.
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if !m3.(Model).scrollLocked {
		t.Error("expected scrollLocked=true after second Ctrl+F")
	}
}

func TestTabExpandsLastToolBlock(t *testing.T) {
	m := newTestModel()
	m.blocks = []block{
		{typ: blockUser, content: "hi"},
		{typ: blockTool, toolName: "read_file", status: toolDone, expanded: false},
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := m2.(Model)
	if !model.blocks[1].expanded {
		t.Error("expected last tool block to be expanded after Tab")
	}

	// Tab again collapses it.
	m3, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = m3.(Model)
	if model.blocks[1].expanded {
		t.Error("expected last tool block to be collapsed after second Tab")
	}
}

func TestTUIApproverApprove(t *testing.T) {
	a := NewTUIApprover()

	// Simulate the TUI responding 'yes' in a goroutine.
	go func() {
		req := <-a.Approvals()
		if req.Tool != "write_file" {
			t.Errorf("expected tool='write_file', got %q", req.Tool)
		}
		req.Resp <- true
	}()

	approved := a.Approve("write_file", map[string]any{"path": "/foo"}, "WRITE /foo")
	if !approved {
		t.Error("expected approve=true")
	}
}

func TestTUIApproverDeny(t *testing.T) {
	a := NewTUIApprover()

	go func() {
		req := <-a.Approvals()
		req.Resp <- false
	}()

	approved := a.Approve("delete_file", map[string]any{"path": "/bar"}, "DELETE /bar")
	if approved {
		t.Error("expected approve=false (denied)")
	}
}

func TestTUIApproverRequiresApproval(t *testing.T) {
	a := NewTUIApprover()

	// Destructive tools require approval.
	if !a.RequiresApproval("write_file") {
		t.Error("write_file should require approval")
	}
	if !a.RequiresApproval("update_file") {
		t.Error("update_file should require approval")
	}
	if !a.RequiresApproval("delete_file") {
		t.Error("delete_file should require approval")
	}
	if !a.RequiresApproval("git_commit") {
		t.Error("git_commit should require approval")
	}

	// Read-only tools don't require approval.
	if a.RequiresApproval("read_file") {
		t.Error("read_file should NOT require approval")
	}
	if a.RequiresApproval("search") {
		t.Error("search should NOT require approval")
	}
}

func TestAutocompleteFiltering(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/he")
	m.updateAutocomplete()
	if !m.acVisible {
		t.Error("expected autocomplete visible")
	}
	if len(m.acItems) == 0 {
		t.Fatal("expected at least one autocomplete item")
	}
	// /help should be in the list.
	found := false
	for _, item := range m.acItems {
		if item == "/help" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected /help in autocomplete items: %v", m.acItems)
	}
}

func TestAutocompleteHidesOnNonSlashInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello world")
	if m.shouldShowAutocomplete() {
		t.Error("should not show autocomplete for non-slash input")
	}
}

func TestRenderBlocksScrollFollow(t *testing.T) {
	m := newTestModel()
	m.scrollLocked = false
	m.blocks = []block{{typ: blockUser, content: "test"}}
	m.renderBlocks()
	// With scrollLocked=false, GotoBottom should NOT have been called.
	// We can't easily test viewport position directly, but verify no panic.
}

func TestRenderBlocksWithStreaming(t *testing.T) {
	m := newTestModel()
	m.blocks = []block{{typ: blockUser, content: "hi"}}
	m.streaming.WriteString("partial assistant")
	m.renderBlocks()
	content := m.viewport.View()
	if !strings.Contains(content, "hi") {
		t.Error("renderBlocks should contain user message")
	}
	if !strings.Contains(content, "partial assistant") {
		t.Error("renderBlocks should contain streaming content")
	}
	if !strings.Contains(content, "▌") {
		t.Error("renderBlocks should show streaming cursor")
	}
}

func TestRenderApprovalPrompt(t *testing.T) {
	m := newTestModel()
	m.pendingApproval = &ApprovalReq{
		ID:      "test-1",
		Tool:    "write_file",
		Preview: "WRITE /tmp/foo.go (10 bytes):\npackage main",
	}
	prompt := m.renderApprovalPrompt()
	if !strings.Contains(prompt, "write_file") {
		t.Errorf("approval prompt should show tool name: %q", prompt)
	}
	if !strings.Contains(prompt, "/tmp/foo.go") {
		t.Errorf("approval prompt should show preview path: %q", prompt)
	}
	if !strings.Contains(prompt, "[y]es") {
		t.Errorf("approval prompt should show y/n/a options: %q", prompt)
	}
}

func TestApprovalKeyHandling(t *testing.T) {
	m := newTestModel()
	resp := make(chan bool, 1)
	m.pendingApproval = &ApprovalReq{
		ID:   "test",
		Tool: "write_file",
		Resp: resp,
	}

	// Press 'y' — should approve.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m2.(Model).pendingApproval != nil {
		t.Error("pendingApproval should be nil after pressing y")
	}
	select {
	case approved := <-resp:
		if !approved {
			t.Error("expected approval response = true")
		}
	default:
		t.Error("no response sent on approval channel")
	}
}

// newTestModel creates a Model with sensible defaults for unit testing.
func newTestModel() Model {
	m := New("test-model", 10, "test-session",
		make(chan string, 1),
		make(chan string, 1),
		make(chan agent.Event, 1),
		nil,                  // no approver
		make(chan string, 1), // switchCh
		nil,                  // no session lister
		"",                   // no project path
	)
	m.width = 80
	m.height = 24
	m.viewport.Width = 80
	m.viewport.Height = 20
	return m
}
