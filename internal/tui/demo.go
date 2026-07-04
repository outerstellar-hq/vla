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

// DemoFrame is a single frame in an animated demo sequence.
type DemoFrame struct {
	Blocks  []DemoBlock
	Options DemoOptions
}

// RenderSingle renders this frame as a complete ANSI-styled TUI view.
func (f DemoFrame) RenderSingle() string {
	return RenderDemoFrame(f.Blocks, f.Options)
}

// RenderDemoSequence takes a list of frames and renders each one as a
// complete ANSI-styled TUI frame. The result is a slice of ANSI strings
// suitable for converting to individual PNG frames and stitching into
// an animated GIF.
//
// Each frame should be a progression of the previous (e.g. frame 1 shows
// the user's message, frame 2 adds the spinner + partial response, frame 3
// adds a tool call, etc.). The caller is responsible for designing the
// narrative flow.
func RenderDemoSequence(frames []DemoFrame) []string {
	result := make([]string, len(frames))
	for i, f := range frames {
		result[i] = RenderDemoFrame(f.Blocks, f.Options)
	}
	return result
}

// DefaultDemoSequence returns a frame sequence that demonstrates a realistic
// VLA interaction with smooth typing and streaming effects. The sequence
// uses many frames with incremental text reveal for a natural-looking demo.
//
// The narrative: user types a question → agent thinks → streams a response
// → calls read_file → analyzes → calls update_file → confirms the fix.
//
// Frame counts per phase are tuned for ~500ms GIF frame delay:
//   - Typing: 3 chars/frame (fast typing)
//   - Streaming: 4 chars/frame (LLM token speed)
//   - Pauses: 2 frames at phase transitions (tool running, thinking)
func DefaultDemoSequence() []DemoFrame {
	baseOpts := DemoOptions{
		ModelName: "gpt-4o",
		SessionID: "20260704T120000Z",
		ToolCount: 24,
		Tokens:    0,
		Width:     100,
		Height:    28,
	}

	userText := "Fix the login bug in auth.py — users can't log in with valid credentials"
	response1 := "I'll investigate the auth module. Let me read the file first."
	response2 := "Found it! The `verify` function uses MD5 and compares the wrong value. Let me fix it."
	response3 := "Done! Switched `verify()` from MD5 to bcrypt. Login flow should work now. ✅"

	readResult := "import hashlib\n\ndef login(username, password):\n    user = get_user(username)\n    if user and verify(password, user.hash):\n        return create_session(user)\n    return None\n\ndef verify(password, stored_hash):\n    return hashlib.md5(password.encode()).hexdigest() == stored_hash"

	var frames []DemoFrame
	tokens := 0
	opts := func() DemoOptions { o := baseOpts; o.Tokens = tokens; return o }

	// === PHASE 1: User types their message (typing effect, 3 chars/frame) ===
	for i := 1; i <= len(userText); i += 3 {
		frames = append(frames, DemoFrame{
			Blocks:  []DemoBlock{{Type: "user", Content: userText[:demoMin(i, len(userText))]}},
			Options: opts(),
		})
	}
	// Full user message + pause (duplicate frame for a beat).
	fullUser := DemoFrame{Blocks: []DemoBlock{{Type: "user", Content: userText}}, Options: opts()}
	frames = append(frames, fullUser, fullUser)

	// === PHASE 2: Agent thinking (spinner, brief pause) ===
	tokens += 80
	thinkingOpts := func() DemoOptions {
		o := baseOpts
		o.Spinning = true
		o.StatusText = "thinking"
		o.Tokens = tokens
		return o
	}
	for i := 0; i < 3; i++ {
		frames = append(frames, DemoFrame{Blocks: []DemoBlock{{Type: "user", Content: userText}}, Options: thinkingOpts()})
	}

	// === PHASE 3: Agent streams response1 (token streaming, 4 chars/frame) ===
	streamOpts := func(text string) DemoOptions { o := thinkingOpts(); o.Streaming = text; return o }
	for i := 1; i <= len(response1); i += 4 {
		tokens += 8
		frames = append(frames, DemoFrame{
			Blocks:  []DemoBlock{{Type: "user", Content: userText}},
			Options: streamOpts(response1[:demoMin(i, len(response1))]),
		})
	}

	// === PHASE 4: read_file (running pause → done) ===
	tokens += 120
	readDone := []DemoBlock{
		{Type: "user", Content: userText},
		{Type: "assistant", Content: response1},
		{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: readResult},
	}
	// Running frame (pause).
	toolRunOpts := func() DemoOptions {
		o := baseOpts
		o.Spinning = true
		o.StatusText = "running: read_file"
		o.Tokens = tokens
		return o
	}
	readRunning := DemoFrame{
		Blocks: []DemoBlock{
			{Type: "user", Content: userText},
			{Type: "assistant", Content: response1},
			{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "running"},
		},
		Options: toolRunOpts(),
	}
	frames = append(frames, readRunning, readRunning)
	// Done frame.
	frames = append(frames, DemoFrame{Blocks: readDone, Options: toolRunOpts()})

	// === PHASE 5: Agent streams response2 (analysis) ===
	readDoneShort := []DemoBlock{
		{Type: "user", Content: userText},
		{Type: "assistant", Content: response1},
		{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "(14 lines)"},
	}
	thinkAfterRead := func(text string) DemoOptions {
		o := baseOpts
		o.Spinning = true
		o.StatusText = "thinking"
		o.Tokens = tokens
		o.Streaming = text
		return o
	}
	for i := 1; i <= len(response2); i += 4 {
		tokens += 8
		frames = append(frames, DemoFrame{Blocks: readDoneShort, Options: thinkAfterRead(response2[:demoMin(i, len(response2))])})
	}

	// === PHASE 6: update_file (running pause → done) ===
	tokens += 150
	updateDone := []DemoBlock{
		{Type: "user", Content: userText},
		{Type: "assistant", Content: response1},
		{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "(14 lines)"},
		{Type: "assistant", Content: response2},
		{Type: "tool", ToolName: "update_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "File updated"},
	}
	updateRunOpts := func() DemoOptions {
		o := baseOpts
		o.Spinning = true
		o.StatusText = "running: update_file"
		o.Tokens = tokens
		return o
	}
	updateRunning := DemoFrame{
		Blocks: []DemoBlock{
			{Type: "user", Content: userText},
			{Type: "assistant", Content: response1},
			{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "(14 lines)"},
			{Type: "assistant", Content: response2},
			{Type: "tool", ToolName: "update_file", ToolArgs: `{"path":"auth.py"}`, Status: "running"},
		},
		Options: updateRunOpts(),
	}
	frames = append(frames, updateRunning, updateRunning)
	frames = append(frames, DemoFrame{Blocks: updateDone, Options: updateRunOpts()})

	// === PHASE 7: Agent streams response3 (confirmation) ===
	confirmOpts := func(text string) DemoOptions {
		o := baseOpts
		o.Spinning = true
		o.StatusText = "thinking"
		o.Tokens = tokens
		o.Streaming = text
		return o
	}
	for i := 1; i <= len(response3); i += 4 {
		tokens += 8
		frames = append(frames, DemoFrame{Blocks: updateDone, Options: confirmOpts(response3[:demoMin(i, len(response3))])})
	}

	// === PHASE 8: Final idle state (hold for a beat) ===
	tokens += 100
	finalBlocks := []DemoBlock{
		{Type: "user", Content: userText},
		{Type: "assistant", Content: response1},
		{Type: "tool", ToolName: "read_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "(14 lines)"},
		{Type: "assistant", Content: response2},
		{Type: "tool", ToolName: "update_file", ToolArgs: `{"path":"auth.py"}`, Status: "done", ToolResult: "File updated"},
		{Type: "assistant", Content: response3},
	}
	idleOpts := DemoOptions{ModelName: "gpt-4o", SessionID: "20260704T120000Z", ToolCount: 24, Tokens: tokens, Width: 100, Height: 28, StatusText: "idle"}
	finalFrame := DemoFrame{Blocks: finalBlocks, Options: idleOpts}
	for i := 0; i < 5; i++ { // hold the final frame longer
		frames = append(frames, finalFrame)
	}

	return frames
}

func demoMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
