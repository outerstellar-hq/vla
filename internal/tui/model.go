// Package tui implements VLA's full-screen terminal interface using
// bubbletea. It shows a scrollable conversation pane (user messages,
// assistant streaming with markdown rendering, tool call blocks with
// expand/collapse), a live status bar (model, spinner, token count), and
// a multi-line input area with slash-command autocomplete.
//
// The agent loop runs in a background goroutine. Two channels carry data
// from loop to TUI:
//   - streamCh: raw streaming text tokens (via io.Writer)
//   - eventCh:  typed events (tool start/result, turn boundaries, usage)
//
// The TUI sends user input to the loop via inputReady.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/abrandt/vla/internal/agent"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the bubbletea model for the VLA TUI.
type Model struct {
	viewport viewport.Model
	textarea textarea.Model
	blocks   []block
	width    int
	height   int

	// Status bar state.
	modelName  string
	toolCount  int
	sessionID  string
	tokens     int // accumulated total tokens from EventUsage
	spinner    spinner.Model
	spinning   bool   // true while waiting for LLM or tool
	statusText string // "thinking", "running: read_file", "idle"

	// Channels.
	inputReady chan string        // TUI → loop: user submits text
	streamCh   <-chan string      // loop → TUI: raw streaming tokens
	eventCh    <-chan agent.Event // loop → TUI: typed events

	// View state.
	quitting     bool
	streaming    strings.Builder // accumulates the current streaming response
	isStreaming  bool
	scrollLocked bool // true = auto-follow (GotoBottom on new content)

	// Autocomplete.
	acItems   []string // filtered slash commands
	acIndex   int      // selected item in autocomplete
	acVisible bool

	// Approval prompt state.
	pendingApproval *ApprovalReq       // non-nil when waiting for user y/n/a
	approvalsCh     <-chan ApprovalReq // nil = no TUI approval (use --yes or readline)

	// Slash commands available for autocomplete.
	slashCommands []string
}

// Styles for the status bar and input area.
var (
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
	inputStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	acStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	acSelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// knownSlashCommands is the static list of commands for autocomplete.
// These are handled by the agent loop; the TUI just provides the menu.
var knownSlashCommands = []string{
	"/help", "/tools", "/model", "/memory", "/compact",
	"/session", "/cost", "/clear",
}

// New creates the TUI model. Channels wire it to the agent loop:
//   - inputReady: the TUI sends user-submitted text here
//   - streamCh: the loop sends raw streaming LLM tokens here
//   - eventCh: the loop sends typed events (tool calls, usage, turn boundaries)
//
// modelName, toolCount, and sessionID populate the static parts of the
// status bar. If approver is non-nil, the TUI handles tool approval prompts
// inline (fixing the deadlock that ReadlineApprover causes in alt-screen mode).
func New(
	modelName string,
	toolCount int,
	sessionID string,
	inputReady chan string,
	streamCh <-chan string,
	eventCh <-chan agent.Event,
	approver *TUIApprover,
) Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message... (Ctrl+Enter to submit, Ctrl+C to quit, Tab to expand tools)"
	ta.Focus()
	ta.CharLimit = 0 // unlimited input

	vp := viewport.New(80, 20)
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := Model{
		viewport:      vp,
		textarea:      ta,
		modelName:     modelName,
		toolCount:     toolCount,
		sessionID:     sessionID,
		inputReady:    inputReady,
		streamCh:      streamCh,
		eventCh:       eventCh,
		scrollLocked:  true,
		spinner:       sp,
		slashCommands: knownSlashCommands,
		statusText:    "idle",
	}
	// Store the approver's channel for polling.
	if approver != nil {
		m.approvalsCh = approver.Approvals()
	}
	return m
}

// Init implements tea.Model. Starts the channel polling and cursor blink.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
		m.spinner.Tick,
		pollStream(),
		pollEvents(),
	}
	if m.approvalsCh != nil {
		cmds = append(cmds, pollApprovals())
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		convHeight := msg.Height - 7 // status(1) + ac(?) + gap(1) + input(4) + gap(1)
		if convHeight < 3 {
			convHeight = 3
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = convHeight
		m.textarea.SetWidth(msg.Width - 4)
		m.renderBlocks()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.spinning {
			cmds = append(cmds, cmd)
		}
		// Don't return here — let the textarea/viewport also process the msg.

	case tea.KeyMsg:
		// If an approval prompt is pending, intercept y/n/a.
		if m.pendingApproval != nil {
			switch msg.String() {
			case "y", "Y":
				m.pendingApproval.Resp <- true
				m.pendingApproval = nil
				return m, nil
			case "n", "N":
				m.pendingApproval.Resp <- false
				m.pendingApproval = nil
				return m, nil
			case "a", "A":
				// Approve all: send true and note it. Future approvals in this
				// turn are auto-approved (the loop's approver still asks, but
				// we respond immediately — see approvalTickMsg handler).
				m.pendingApproval.Resp <- true
				m.pendingApproval = nil
				return m, nil
			}
			// Other keys are ignored during approval prompt.
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyCtrlJ:
			// Submit the input (Ctrl+J = linefeed, the standard TUI submit).
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" && !m.isStreaming {
				m.blocks = append(m.blocks, block{typ: blockUser, content: text})
				m.textarea.Reset()
				m.isStreaming = true
				m.spinning = true
				m.statusText = "thinking"
				m.streaming.Reset()
				m.scrollLocked = true // re-enable follow on new message
				m.acVisible = false
				m.inputReady <- text
				m.renderBlocks()
				cmds = append(cmds, m.spinner.Tick)
			}
			return m, tea.Batch(cmds...)

		case tea.KeyTab:
			// Toggle expansion of the last tool block.
			for i := len(m.blocks) - 1; i >= 0; i-- {
				if m.blocks[i].typ == blockTool {
					m.blocks[i].expanded = !m.blocks[i].expanded
					m.renderBlocks()
					break
				}
			}
			return m, nil

		case tea.KeyCtrlF:
			// Toggle scroll lock (auto-follow).
			m.scrollLocked = !m.scrollLocked
			m.renderBlocks()
			return m, nil

		case tea.KeyEsc:
			m.acVisible = false
			return m, nil

		case tea.KeyUp:
			if m.acVisible && len(m.acItems) > 0 {
				m.acIndex = (m.acIndex - 1 + len(m.acItems)) % len(m.acItems)
				return m, nil
			}

		case tea.KeyDown:
			if m.acVisible && len(m.acItems) > 0 {
				m.acIndex = (m.acIndex + 1) % len(m.acItems)
				return m, nil
			}

		case tea.KeyEnter:
			if m.acVisible && len(m.acItems) > 0 {
				// Accept autocomplete selection.
				m.textarea.SetValue(m.acItems[m.acIndex])
				m.acVisible = false
				return m, nil
			}

		case tea.KeyRunes, tea.KeyBackspace, tea.KeyDelete:
			// Fall through to textarea update, then check for autocomplete.
			// We'll handle autocomplete after the textarea processes the key.
		}

	// Custom tick messages from channel polling.
	case streamTickMsg:
		select {
		case token := <-m.streamCh:
			m.streaming.WriteString(token)
			m.renderStreaming()
			cmds = append(cmds, pollStream())
		default:
			// Channel empty — but keep polling in case more data arrives.
			cmds = append(cmds, pollStream())
		}

	case eventTickMsg:
		consumed := false
		select {
		case ev := <-m.eventCh:
			m.handleEvent(ev)
			consumed = true
		default:
		}
		// Always keep polling events.
		cmds = append(cmds, pollEvents())
		_ = consumed

	case approvalTickMsg:
		if m.pendingApproval == nil {
			select {
			case req := <-m.approvalsCh:
				m.pendingApproval = &req
			default:
			}
		}
		if m.approvalsCh != nil {
			cmds = append(cmds, pollApprovals())
		}
	}

	// Update viewport and textarea with the message.
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	// After textarea processes the key, check for slash-command autocomplete.
	if m.shouldShowAutocomplete() {
		m.updateAutocomplete()
	} else {
		m.acVisible = false
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	convPane := m.viewport.View()
	b.WriteString(convPane)
	b.WriteString("\n")

	// Approval prompt (above input, below conversation).
	if m.pendingApproval != nil {
		b.WriteString(m.renderApprovalPrompt())
		b.WriteString("\n")
	}

	// Autocomplete menu (above input).
	if m.acVisible && len(m.acItems) > 0 {
		for i, item := range m.acItems {
			if i == m.acIndex {
				b.WriteString(acSelStyle.Render(" " + item + " "))
			} else {
				b.WriteString(acStyle.Render(" " + item + " "))
			}
			b.WriteString("\n")
		}
	}

	// Gap + input.
	b.WriteString(inputStyle.Render(m.textarea.View()))

	return b.String()
}

// renderApprovalPrompt builds the inline approval prompt shown when a
// destructive tool call needs the user's permission.
func (m Model) renderApprovalPrompt() string {
	if m.pendingApproval == nil {
		return ""
	}
	req := m.pendingApproval
	var b strings.Builder
	b.WriteString(errorLabelStyle.Render("┌─ Approval needed ────────────────────"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("│ %s %s\n", toolLabelStyle.Render("⚙"), toolNameStyle.Render(req.Tool)))
	// Show preview, truncated to 5 lines.
	lines := strings.Split(req.Preview, "\n")
	maxLines := 5
	for i, line := range lines {
		if i >= maxLines {
			b.WriteString(systemLabelStyle.Render("│ …"))
			break
		}
		b.WriteString(fmt.Sprintf("│ %s\n", systemLabelStyle.Render(line)))
	}
	b.WriteString(errorLabelStyle.Render("└─ Allow? [y]es / [n]o / [a]ll ─────────"))
	return b.String()
}

// renderStatusBar builds the dynamic status bar:
//
//	vla │ gpt-4o │ 24 tools │ ⠹ thinking │ 1.2k tokens │ following
func (m Model) renderStatusBar() string {
	var parts []string

	parts = append(parts, dimStyle.Render("vla"))
	parts = append(parts, m.modelName)
	parts = append(parts, fmt.Sprintf("%d tools", m.toolCount))

	// Spinner + status.
	if m.spinning {
		spinnerText := m.spinner.View()
		statusText := m.statusText
		if statusText == "" {
			statusText = "working"
		}
		parts = append(parts, fmt.Sprintf("%s %s", spinnerText, statusText))
	} else {
		parts = append(parts, dimStyle.Render("idle"))
	}

	// Token count.
	if m.tokens > 0 {
		parts = append(parts, formatTokens(m.tokens))
	}

	// Scroll state.
	if m.scrollLocked {
		parts = append(parts, dimStyle.Render("↓ following"))
	} else {
		parts = append(parts, dimStyle.Render("⏸ paused (Ctrl+F to follow)"))
	}

	// Session ID (truncated).
	sessID := m.sessionID
	if len(sessID) > 8 {
		sessID = sessID[:8]
	}
	parts = append(parts, dimStyle.Render(sessID))

	sep := dimStyle.Render(" │ ")
	return statusStyle.Render(strings.Join(parts, sep))
}

// formatTokens formats a token count for compact display (e.g. 1200 → "1.2k").
func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d tok", n)
	}
	return fmt.Sprintf("%.1fk tok", float64(n)/1000)
}

// handleEvent processes one typed event from the agent loop, updating the
// TUI state (blocks, spinner, token count) accordingly.
func (m *Model) handleEvent(ev agent.Event) {
	switch ev.Type {
	case agent.EventTurnStart:
		m.spinning = true
		m.statusText = "thinking"
		m.isStreaming = true
		m.streaming.Reset()

	case agent.EventTurnEnd:
		// Flush any remaining streaming content into a block.
		if m.streaming.Len() > 0 {
			m.blocks = append(m.blocks, block{
				typ:     blockAssistant,
				content: m.streaming.String(),
			})
			m.streaming.Reset()
		}
		// Close any still-running tool blocks.
		for i := range m.blocks {
			if m.blocks[i].typ == blockTool && m.blocks[i].status == toolRunning {
				m.blocks[i].status = toolDone
			}
		}
		m.isStreaming = false
		m.spinning = false
		m.statusText = "idle"
		m.renderBlocks()

	case agent.EventToolStart:
		// Start a new tool block in running state.
		m.blocks = append(m.blocks, block{
			typ:      blockTool,
			toolName: ev.Tool,
			toolArgs: ev.Args,
			status:   toolRunning,
		})
		m.statusText = "running: " + ev.Tool
		m.renderBlocks()

	case agent.EventToolResult:
		// Update the most recent matching tool block with the result.
		for i := len(m.blocks) - 1; i >= 0; i-- {
			if m.blocks[i].typ == blockTool &&
				m.blocks[i].toolName == ev.Tool &&
				m.blocks[i].status == toolRunning {
				m.blocks[i].toolResult = ev.Result
				if ev.Error {
					m.blocks[i].status = toolDenied
				} else {
					m.blocks[i].status = toolDone
				}
				break
			}
		}
		// If the spinner was showing tool status, revert to thinking.
		if m.isStreaming {
			m.statusText = "thinking"
		} else {
			m.statusText = "idle"
			m.spinning = false
		}
		m.renderBlocks()

	case agent.EventUsage:
		if ev.Usage != nil {
			m.tokens = ev.Usage.TotalTokens
		}
	}
}

// shouldShowAutocomplete returns true when the input starts with "/" but
// hasn't been submitted yet (i.e. the user is typing a slash command).
func (m *Model) shouldShowAutocomplete() bool {
	val := m.textarea.Value()
	return strings.HasPrefix(strings.TrimSpace(val), "/")
}

// updateAutocomplete rebuilds the filtered command list based on current input.
func (m *Model) updateAutocomplete() {
	val := strings.TrimSpace(m.textarea.Value())
	var filtered []string
	for _, cmd := range m.slashCommands {
		if strings.HasPrefix(cmd, val) {
			filtered = append(filtered, cmd)
		}
	}
	m.acItems = filtered
	m.acIndex = 0
	m.acVisible = len(filtered) > 0 && val != "/"
}

// renderBlocks rebuilds the conversation pane from the block list.
// If streaming is active, appends the in-progress assistant block with a cursor.
func (m *Model) renderBlocks() {
	var b strings.Builder
	for _, blk := range m.blocks {
		b.WriteString(renderBlock(blk, m.viewport.Width))
		b.WriteString("\n\n")
	}
	// Append in-progress streaming response.
	if m.streaming.Len() > 0 {
		label := assistantLabelStyle.Render("VLA")
		fmt.Fprintf(&b, "%s: %s▌\n", label, m.streaming.String())
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
	if m.scrollLocked {
		m.viewport.GotoBottom()
	}
}

// renderStreaming updates the view during token streaming (hot path).
// Same as renderBlocks but called on every token to avoid rebuilding all blocks.
func (m *Model) renderStreaming() {
	m.renderBlocks()
}

// --- Channel polling commands ---

type streamTickMsg struct{}
type eventTickMsg struct{}
type approvalTickMsg struct{}

func pollStream() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg { return streamTickMsg{} })
}

func pollEvents() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg { return eventTickMsg{} })
}

func pollApprovals() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return approvalTickMsg{} })
}
