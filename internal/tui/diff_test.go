package tui

import (
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

func TestComputeDiff_AllAdded(t *testing.T) {
	result := computeDiff("", "line1\nline2\nline3")
	if len(result) != 3 {
		t.Fatalf("expected 3 diff lines, got %d", len(result))
	}
	for i, dl := range result {
		if dl.status != diffAdded {
			t.Errorf("line %d: expected diffAdded, got %d", i, dl.status)
		}
	}
	if result[0].text != "line1" {
		t.Errorf("line 0 text: got %q, want %q", result[0].text, "line1")
	}
}

func TestComputeDiff_AllRemoved(t *testing.T) {
	result := computeDiff("line1\nline2\nline3", "")
	if len(result) != 3 {
		t.Fatalf("expected 3 diff lines, got %d", len(result))
	}
	for i, dl := range result {
		if dl.status != diffRemoved {
			t.Errorf("line %d: expected diffRemoved, got %d", i, dl.status)
		}
	}
}

func TestComputeDiff_NoChange(t *testing.T) {
	result := computeDiff("a\nb\nc", "a\nb\nc")
	if len(result) != 3 {
		t.Fatalf("expected 3 diff lines, got %d", len(result))
	}
	for i, dl := range result {
		if dl.status != diffUnchanged {
			t.Errorf("line %d: expected diffUnchanged, got %d", i, dl.status)
		}
	}
}

func TestComputeDiff_PartialChange(t *testing.T) {
	old := "line1\nold2\nline3"
	new := "line1\nnew2\nline3"
	result := computeDiff(old, new)

	// Expect: unchanged(line1), removed(old2), added(new2), unchanged(line3)
	if len(result) != 4 {
		t.Fatalf("expected 4 diff lines, got %d: %+v", len(result), result)
	}
	if result[0].status != diffUnchanged || result[0].text != "line1" {
		t.Errorf("line 0: expected unchanged 'line1', got status=%d text=%q", result[0].status, result[0].text)
	}
	if result[1].status != diffRemoved || result[1].text != "old2" {
		t.Errorf("line 1: expected removed 'old2', got status=%d text=%q", result[1].status, result[1].text)
	}
	if result[2].status != diffAdded || result[2].text != "new2" {
		t.Errorf("line 2: expected added 'new2', got status=%d text=%q", result[2].status, result[2].text)
	}
	if result[3].status != diffUnchanged || result[3].text != "line3" {
		t.Errorf("line 3: expected unchanged 'line3', got status=%d text=%q", result[3].status, result[3].text)
	}
}

func TestComputeDiff_Insertion(t *testing.T) {
	old := "a\nc"
	new := "a\nb\nc"
	result := computeDiff(old, new)

	// Expect: unchanged(a), added(b), unchanged(c)
	if len(result) != 3 {
		t.Fatalf("expected 3 diff lines, got %d: %+v", len(result), result)
	}
	if result[1].status != diffAdded || result[1].text != "b" {
		t.Errorf("line 1: expected added 'b', got status=%d text=%q", result[1].status, result[1].text)
	}
}

func TestComputeDiff_Deletion(t *testing.T) {
	old := "a\nb\nc"
	new := "a\nc"
	result := computeDiff(old, new)

	// Expect: unchanged(a), removed(b), unchanged(c)
	if len(result) != 3 {
		t.Fatalf("expected 3 diff lines, got %d: %+v", len(result), result)
	}
	if result[1].status != diffRemoved || result[1].text != "b" {
		t.Errorf("line 1: expected removed 'b', got status=%d text=%q", result[1].status, result[1].text)
	}
}

func TestComputeDiff_EmptyBoth(t *testing.T) {
	result := computeDiff("", "")
	if len(result) != 0 {
		t.Errorf("expected 0 diff lines for empty input, got %d", len(result))
	}
}

func TestComputeDiff_TruncatesLargeInput(t *testing.T) {
	// Generate > maxDiffLines lines.
	old := strings.Repeat("old\n", 300)
	new := strings.Repeat("new\n", 300)
	result := computeDiff(old, new)
	if len(result) > maxDiffLines {
		t.Errorf("expected diff to be capped at %d lines, got %d", maxDiffLines, len(result))
	}
}

func TestRenderDiff_ColorCoding(t *testing.T) {
	lines := []diffLine{
		{text: "unchanged", status: diffUnchanged},
		{text: "added", status: diffAdded},
		{text: "removed", status: diffRemoved},
	}
	out := renderDiff(lines, 60)

	// The rendered output contains ANSI codes; verify the text content.
	if !strings.Contains(out, "unchanged") {
		t.Error("expected 'unchanged' in output")
	}
	if !strings.Contains(out, "+ added") {
		t.Error("expected '+ added' in output")
	}
	if !strings.Contains(out, "- removed") {
		t.Error("expected '- removed' in output")
	}
}

func TestRenderDiff_EmptyInput(t *testing.T) {
	out := renderDiff(nil, 60)
	if !strings.Contains(out, "(no changes)") {
		t.Errorf("expected '(no changes)' for empty diff, got: %q", out)
	}
}

func TestRenderDiff_LineTruncation(t *testing.T) {
	longLine := strings.Repeat("x", 100)
	lines := []diffLine{
		{text: longLine, status: diffAdded},
	}
	out := renderDiff(lines, 20) // narrow width
	// The output should contain the truncation indicator.
	if !strings.Contains(out, "…") {
		t.Error("expected truncation indicator '…' for long line in narrow pane")
	}
}

func TestRenderDiffPane_HasTitle(t *testing.T) {
	content := "+ new line\n- old line"
	out := renderDiffPane("update_file — /foo.go", content, 60, 10)
	if !strings.Contains(out, "update_file") {
		t.Error("expected diff pane to contain title")
	}
	if !strings.Contains(out, "/foo.go") {
		t.Error("expected diff pane to contain file path")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\nb\nc\n", []string{"a", "b", "c"}},   // trailing newline trimmed
		{"a\r\nb\r\nc", []string{"a", "b", "c"}}, // CRLF normalized
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitLines(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// --- Integration tests: diff pane + TUI model ---

func TestDiffPaneToggleCtrlD(t *testing.T) {
	m := newTestModel()
	if m.diffVisible {
		t.Error("diff pane should start hidden")
	}

	// Ctrl+D shows the diff pane.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !m2.(Model).diffVisible {
		t.Error("expected diffVisible=true after Ctrl+D")
	}

	// Ctrl+D again hides it.
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m3.(Model).diffVisible {
		t.Error("expected diffVisible=false after second Ctrl+D")
	}
}

func TestDiffPaneEscHides(t *testing.T) {
	m := newTestModel()
	m.diffVisible = true

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m2.(Model).diffVisible {
		t.Error("expected diffVisible=false after Esc")
	}
}

func TestHandleEventWriteFileShowsDiff(t *testing.T) {
	m := newTestModel()
	m.diffPane.Width = 40
	m.handleEvent(agent.Event{
		Type: agent.EventToolStart,
		Tool: "write_file",
		Args: `{"path":"/tmp/foo.go","content":"package main\nfunc foo() {}"}`,
	})

	if !m.diffVisible {
		t.Error("expected diffVisible=true after write_file EventToolStart")
	}
	if !strings.Contains(m.diffTitle, "write_file") {
		t.Errorf("expected diffTitle to contain 'write_file', got %q", m.diffTitle)
	}
	if !strings.Contains(m.diffTitle, "/tmp/foo.go") {
		t.Errorf("expected diffTitle to contain path, got %q", m.diffTitle)
	}
	if !strings.Contains(m.diffContent, "package main") {
		t.Errorf("expected diffContent to contain 'package main', got %q", m.diffContent)
	}
}

func TestHandleEventUpdateFileShowsDiff(t *testing.T) {
	m := newTestModel()
	m.diffPane.Width = 40
	m.handleEvent(agent.Event{
		Type: agent.EventToolStart,
		Tool: "update_file",
		Args: `{"path":"/bar.go","old_string":"old line","new_string":"new line"}`,
	})

	if !m.diffVisible {
		t.Error("expected diffVisible=true after update_file EventToolStart")
	}
	// Diff content should show the change.
	if !strings.Contains(m.diffContent, "new line") {
		t.Errorf("expected diffContent to contain 'new line', got %q", m.diffContent)
	}
}

func TestHandleEventReadFileDoesNotShowDiff(t *testing.T) {
	m := newTestModel()
	m.handleEvent(agent.Event{
		Type: agent.EventToolStart,
		Tool: "read_file",
		Args: `{"path":"/tmp/foo.go"}`,
	})

	if m.diffVisible {
		t.Error("read_file should NOT trigger diff pane")
	}
}

func TestShowDiffForToolBadJSON(t *testing.T) {
	m := newTestModel()
	m.showDiffForTool("write_file", "not valid json")
	// Should not crash, should not show diff.
	if m.diffVisible {
		t.Error("expected diff to remain hidden on bad JSON")
	}
}

func TestResizePanes(t *testing.T) {
	m := newTestModel()
	m.width = 100

	// When diff is hidden, conversation pane is full width.
	m.resizePanes()
	if m.viewport.Width != 100 {
		t.Errorf("expected viewport width=100, got %d", m.viewport.Width)
	}

	// When diff is visible, both panes are half width.
	m.diffVisible = true
	m.resizePanes()
	if m.viewport.Width != 50 {
		t.Errorf("expected viewport width=50, got %d", m.viewport.Width)
	}
	if m.diffPane.Width != 50 {
		t.Errorf("expected diffPane width=50, got %d", m.diffPane.Width)
	}
}
