package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func testSessionItems() []SessionItem {
	return []SessionItem{
		{ID: "2026-07-03T120000Z", Project: "/proj", Model: "gpt-4o", Created: time.Now().Add(-2 * time.Hour)},
		{ID: "2026-07-03T100000Z", Project: "/proj", Model: "claude-3", Created: time.Now().Add(-5 * time.Hour)},
		{ID: "2026-07-02T150000Z", Project: "/proj", Model: "gpt-4o", Created: time.Now().Add(-24 * time.Hour)},
	}
}

func TestSessionPickerNavigation(t *testing.T) {
	p := sessionPicker{}
	p.items = testSessionItems()
	p.visible = true

	// Start at index 0.
	if p.index != 0 {
		t.Fatalf("expected initial index=0, got %d", p.index)
	}

	// Down moves to 1.
	p.down()
	if p.index != 1 {
		t.Errorf("after down: expected index=1, got %d", p.index)
	}

	// Down again to 2.
	p.down()
	if p.index != 2 {
		t.Errorf("after 2x down: expected index=2, got %d", p.index)
	}

	// Down wraps to 0.
	p.down()
	if p.index != 0 {
		t.Errorf("after 3x down (wrap): expected index=0, got %d", p.index)
	}

	// Up wraps to last (2).
	p.up()
	if p.index != 2 {
		t.Errorf("after up from 0 (wrap): expected index=2, got %d", p.index)
	}
}

func TestSessionPickerSelected(t *testing.T) {
	p := sessionPicker{}
	p.items = testSessionItems()
	p.visible = true
	p.index = 1

	sel := p.selected()
	if sel == nil {
		t.Fatal("expected non-nil selected")
	}
	if sel.ID != "2026-07-03T100000Z" {
		t.Errorf("selected ID: got %q, want %q", sel.ID, "2026-07-03T100000Z")
	}
}

func TestSessionPickerSelectedEmpty(t *testing.T) {
	p := sessionPicker{}
	if p.selected() != nil {
		t.Error("expected nil for empty picker")
	}
}

func TestSessionPickerRender(t *testing.T) {
	p := sessionPicker{}
	p.items = testSessionItems()
	p.visible = true

	out := p.render(80, 24)
	if !strings.Contains(out, "Switch Session") {
		t.Error("expected picker to contain title 'Switch Session'")
	}
	if !strings.Contains(out, "2026-07-03T120000Z") {
		t.Error("expected picker to contain first session ID")
	}
	if !strings.Contains(out, "gpt-4o") {
		t.Error("expected picker to contain model name")
	}
	if !strings.Contains(out, "3 session(s)") {
		t.Error("expected picker to show session count")
	}
}

func TestSessionPickerRenderEmpty(t *testing.T) {
	p := sessionPicker{}
	p.visible = true

	out := p.render(80, 24)
	if !strings.Contains(out, "No sessions found") {
		t.Error("expected empty picker to show 'No sessions found'")
	}
}

func TestSessionPickerOpenWithLister(t *testing.T) {
	items := testSessionItems()
	lister := func(project string) []SessionItem {
		return items
	}

	p := sessionPicker{}
	p.open(lister, "/proj")

	if !p.visible {
		t.Error("expected picker visible after open")
	}
	if len(p.items) != 3 {
		t.Errorf("expected 3 items, got %d", len(p.items))
	}
	if p.index != 0 {
		t.Errorf("expected index=0 after open, got %d", p.index)
	}
}

func TestSessionPickerClose(t *testing.T) {
	p := sessionPicker{visible: true}
	p.close()
	if p.visible {
		t.Error("expected picker hidden after close")
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-2 * time.Hour), "2h"},
		{now.Add(-48 * time.Hour), "2d"},
		{time.Time{}, "?"}, // zero time
	}
	for _, tt := range tests {
		got := relativeTime(tt.t)
		if got != tt.want {
			t.Errorf("relativeTime() = %q, want %q", got, tt.want)
		}
	}
}

// --- Integration tests: picker + TUI Model ---

func TestCtrlSOpensPicker(t *testing.T) {
	m := newTestModel()
	m.sessionLister = func(string) []SessionItem { return testSessionItems() }

	if m.picker.visible {
		t.Error("picker should start hidden")
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m2.(Model).picker.visible {
		t.Error("expected picker visible after Ctrl+S")
	}
}

func TestCtrlSTogglesPicker(t *testing.T) {
	m := newTestModel()
	m.picker.visible = true
	m.picker.items = testSessionItems()

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m2.(Model).picker.visible {
		t.Error("expected picker hidden after second Ctrl+S")
	}
}

func TestPickerEscDismisses(t *testing.T) {
	m := newTestModel()
	m.picker.visible = true
	m.picker.items = testSessionItems()

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m2.(Model).picker.visible {
		t.Error("expected picker hidden after Esc")
	}
}

func TestPickerUpDownKeys(t *testing.T) {
	m := newTestModel()
	m.picker.visible = true
	m.picker.items = testSessionItems()

	// Down.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m2.(Model).picker.index != 1 {
		t.Errorf("expected index=1 after Down, got %d", m2.(Model).picker.index)
	}

	// Up.
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m3.(Model).picker.index != 0 {
		t.Errorf("expected index=0 after Up, got %d", m3.(Model).picker.index)
	}
}

func TestPickerEnterSendsSwitchID(t *testing.T) {
	switchCh := make(chan string, 1)
	m := newTestModel()
	m.switchCh = switchCh
	m.picker.visible = true
	m.picker.items = testSessionItems()
	m.picker.index = 1 // select the second session

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := m2.(Model)

	if model.picker.visible {
		t.Error("picker should close after Enter")
	}

	select {
	case id := <-switchCh:
		if id != "2026-07-03T100000Z" {
			t.Errorf("switchCh: got %q, want %q", id, "2026-07-03T100000Z")
		}
	default:
		t.Error("no ID sent on switchCh")
	}
}

func TestPickerEnterNoSwitchCh(t *testing.T) {
	// When switchCh is nil, Enter should not panic.
	m := newTestModel()
	m.switchCh = nil
	m.picker.visible = true
	m.picker.items = testSessionItems()

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Should close picker without panicking.
	if m2.(Model).picker.visible {
		t.Error("picker should still close")
	}
}

func TestPickerKeysIgnoredWhenHidden(t *testing.T) {
	m := newTestModel()
	// Picker is hidden — Up/Down/Enter should go to normal handlers,
	// not crash.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Should not have opened the picker.
	if m2.(Model).picker.visible {
		t.Error("picker should not open from Down when hidden")
	}
}

func TestPickerBlocksOtherKeys(t *testing.T) {
	m := newTestModel()
	m.picker.visible = true
	m.picker.items = testSessionItems()

	// When picker is visible, typing should NOT fill the textarea
	// (keys are intercepted).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m2.(Model).textarea.Value() != "" {
		t.Error("textarea should be empty — picker blocks key input")
	}
}
