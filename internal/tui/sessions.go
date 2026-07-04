package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// SessionItem is a single session entry shown in the picker. It's a
// UI-facing copy of session.IndexEntry — defined here so the TUI package
// doesn't need to import the session package.
type SessionItem struct {
	ID      string
	Project string
	Model   string
	Created time.Time
}

// SessionLister returns sessions for a given project path. If project is
// empty, returns all sessions. Satisfied by a closure in tui_runner.go.
type SessionLister func(project string) []SessionItem

// sessionPicker is the full-screen session selection overlay. When visible,
// it replaces the conversation pane with a scrollable list of sessions.
type sessionPicker struct {
	items   []SessionItem
	index   int // selected item
	visible bool
}

// pickerStyles for the session list rendering.
var (
	pickerTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true).
				Padding(0, 1)
	pickerItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	pickerSelStyle  = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("12")).
			Bold(true)
	pickerIDStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	pickerTimeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pickerModelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	pickerHintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// open loads sessions from the lister and shows the picker.
func (p *sessionPicker) open(lister SessionLister, project string) {
	if lister != nil {
		p.items = lister(project)
	}
	p.index = 0
	p.visible = true
}

// close hides the picker.
func (p *sessionPicker) close() {
	p.visible = false
}

// up moves the selection up (wraps around).
func (p *sessionPicker) up() {
	if len(p.items) == 0 {
		return
	}
	p.index = (p.index - 1 + len(p.items)) % len(p.items)
}

// down moves the selection down (wraps around).
func (p *sessionPicker) down() {
	if len(p.items) == 0 {
		return
	}
	p.index = (p.index + 1) % len(p.items)
}

// selected returns the currently highlighted session, or nil if empty.
func (p *sessionPicker) selected() *SessionItem {
	if len(p.items) == 0 {
		return nil
	}
	return &p.items[p.index]
}

// render produces the full picker overlay for the View() method.
// width and height are the conversation pane dimensions.
func (p *sessionPicker) render(width, height int) string {
	var b strings.Builder

	// Title bar.
	b.WriteString(pickerTitleStyle.Render(" Switch Session "))
	b.WriteString("\n")
	b.WriteString(pickerHintStyle.Render(" ↑↓ navigate · Enter select · Esc cancel "))
	b.WriteString("\n\n")

	if len(p.items) == 0 {
		b.WriteString(pickerTimeStyle.Render("No sessions found."))
		b.WriteString("\n")
		return b.String()
	}

	// Calculate how many items fit.
	maxItems := height - 4 // title(2) + blank(1) + footer(1)
	if maxItems < 1 {
		maxItems = 1
	}

	// Scroll window: show a window of items around the cursor.
	start := 0
	if p.index >= maxItems {
		start = p.index - maxItems + 1
	}
	end := start + maxItems
	if end > len(p.items) {
		end = len(p.items)
	}

	idWidth := 20
	for i := start; i < end; i++ {
		item := p.items[i]
		idStr := item.ID
		if len(idStr) > idWidth {
			idStr = idStr[:idWidth]
		}

		timeStr := relativeTime(item.Created)
		modelStr := item.Model
		if len(modelStr) > 20 {
			modelStr = modelStr[:20]
		}

		line := fmt.Sprintf(" %-*s  %-8s  %s", idWidth, idStr, timeStr, modelStr)

		if i == p.index {
			b.WriteString(pickerSelStyle.Render(line))
		} else {
			b.WriteString(pickerItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	// Footer with count.
	b.WriteString("\n")
	b.WriteString(pickerHintStyle.Render(fmt.Sprintf(" %d session(s)", len(p.items))))

	return b.String()
}

// relativeTime formats a time as a human-readable relative duration
// (e.g. "2h ago", "3d ago", "just now").
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
