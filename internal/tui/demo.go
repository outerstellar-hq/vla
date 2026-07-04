package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

// DemoBlock is a UI-agnostic description of a conversation block for
// screenshot and demo rendering. It maps 1:1 to the internal block type.
// All fields use simple string/int/bool types so it can be constructed
// from outside the tui package.
type DemoBlock struct {
	Type       string // "user", "assistant", "tool", "system", "error"
	Content    string // text content (assistant messages are markdown-rendered)
	ToolName   string // for tool blocks: the tool name (e.g. "read_file")
	ToolArgs   string // for tool blocks: raw JSON args string
	ToolResult string // for tool blocks: the result text
	Status     string // for tool blocks: "done", "running", "denied", "blocked"
	Expanded   bool   // for tool blocks: show full args + result
}

// DemoOptions configures the demo frame rendering.
type DemoOptions struct {
	ModelName  string // e.g. "gpt-4o"
	SessionID  string // e.g. "20260703T150000Z"
	ToolCount  int    // e.g. 24
	Tokens     int    // accumulated token count for status bar
	Width      int    // terminal width (default 100)
	Height     int    // terminal height (default 30)
	Spinning   bool   // show spinner in status bar
	StatusText string // e.g. "thinking", "running: read_file"
	Streaming  string // in-progress assistant text (shows ▌ cursor)
}

// demoBlockToBlock converts a DemoBlock to the internal block type.
func demoBlockToBlock(db DemoBlock) block {
	b := block{
		content:    db.Content,
		toolName:   db.ToolName,
		toolArgs:   db.ToolArgs,
		toolResult: db.ToolResult,
		expanded:   db.Expanded,
	}

	switch db.Type {
	case "user":
		b.typ = blockUser
	case "assistant":
		b.typ = blockAssistant
	case "tool":
		b.typ = blockTool
	case "system":
		b.typ = blockSystem
	case "error":
		b.typ = blockError
	}

	switch db.Status {
	case "done":
		b.status = toolDone
	case "running":
		b.status = toolRunning
	case "denied":
		b.status = toolDenied
	case "blocked":
		b.status = toolBlocked
	}

	return b
}

// RenderDemoFrame renders a complete TUI frame with the given blocks and
// options. Returns ANSI-styled text suitable for piping to a terminal or
// converting to an image. No channels, no goroutines, no bubbletea program —
// fully deterministic and testable.
//
// The output includes the status bar, conversation pane with all blocks,
// and the input box. If opts.Streaming is non-empty, it appears as an
// in-progress assistant response with a ▌ cursor.
func RenderDemoFrame(blocks []DemoBlock, opts DemoOptions) string {
	width := opts.Width
	if width < 40 {
		width = 100
	}
	height := opts.Height
	if height < 10 {
		height = 30
	}

	// Build a minimal Model with just the fields View() needs.
	m := Model{
		modelName:    opts.ModelName,
		toolCount:    opts.ToolCount,
		sessionID:    opts.SessionID,
		tokens:       opts.Tokens,
		spinner:      spinnerModel(),
		spinning:     opts.Spinning,
		statusText:   opts.StatusText,
		scrollLocked: true,
		width:        width,
		height:       height,
		textarea:     demoTextarea(),
	}

	// Convert demo blocks to internal blocks.
	m.blocks = make([]block, len(blocks))
	for i, db := range blocks {
		m.blocks[i] = demoBlockToBlock(db)
	}

	// Set streaming content if provided.
	if opts.Streaming != "" {
		m.streaming.WriteString(opts.Streaming)
	}

	// Size the viewport.
	convHeight := height - 7
	if convHeight < 3 {
		convHeight = 3
	}
	m.viewport = demoViewport(width, convHeight)
	m.diffPane = demoViewport(width/2, convHeight)

	// Render blocks into viewport, then get the full View().
	m.renderBlocks()
	return m.View()
}

// RenderDemoDiff renders the TUI with the split-pane diff visible.
// The last block in blocks should be a write_file or update_file tool call;
// the diff pane shows the content/old_string → new_string diff.
func RenderDemoDiff(blocks []DemoBlock, diffTool, diffArgs string, opts DemoOptions) string {
	width := opts.Width
	if width < 60 {
		width = 100
	}
	height := opts.Height
	if height < 10 {
		height = 30
	}

	m := Model{
		modelName:    opts.ModelName,
		toolCount:    opts.ToolCount,
		sessionID:    opts.SessionID,
		tokens:       opts.Tokens,
		spinner:      spinnerModel(),
		spinning:     opts.Spinning,
		statusText:   opts.StatusText,
		scrollLocked: true,
		width:        width,
		height:       height,
		textarea:     demoTextarea(),
	}

	m.blocks = make([]block, len(blocks))
	for i, db := range blocks {
		m.blocks[i] = demoBlockToBlock(db)
	}

	// Set up diff pane.
	convHeight := height - 7
	if convHeight < 3 {
		convHeight = 3
	}
	halfW := width / 2
	m.viewport = demoViewport(halfW, convHeight)
	m.diffPane = demoViewport(halfW, convHeight)

	// Compute diff content.
	m.showDiffForTool(diffTool, diffArgs)
	m.diffVisible = true

	m.renderBlocks()
	m.refreshDiffPane()
	return m.View()
}

// RenderDemoPicker renders the session picker overlay with the given items.
func RenderDemoPicker(items []SessionItem, opts DemoOptions) string {
	width := opts.Width
	if width < 40 {
		width = 100
	}
	height := opts.Height
	if height < 10 {
		height = 30
	}

	m := Model{
		modelName:    opts.ModelName,
		toolCount:    opts.ToolCount,
		sessionID:    opts.SessionID,
		tokens:       opts.Tokens,
		spinner:      spinnerModel(),
		statusText:   opts.StatusText,
		scrollLocked: true,
		width:        width,
		height:       height,
		textarea:     demoTextarea(),
	}

	convHeight := height - 7
	if convHeight < 3 {
		convHeight = 3
	}
	m.viewport = demoViewport(width, convHeight)

	// Populate picker.
	m.picker.items = items
	m.picker.visible = true

	return m.View()
}

// demoTextarea creates a textarea with demo placeholder text.
func demoTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message... (Ctrl+Enter=submit, Tab=expand, Ctrl+D=diff, Ctrl+S=sessions)"
	ta.SetWidth(96)
	return ta
}

// demoViewport creates a viewport with the given dimensions.
func demoViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	vp.SetContent("")
	return vp
}

// spinnerModel returns a default spinner for demo rendering.
func spinnerModel() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Pulse
	return sp
}

// StripANSI removes ANSI escape codes from a string. Useful for tests
// that need to check content without styling noise.
func StripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
