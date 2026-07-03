// Package tui implements VLA's full-screen terminal interface using
// bubbletea. It shows a scrollable conversation pane (user messages,
// assistant streaming, tool call results), a status bar (model, tool count,
// session ID), and a multi-line input area at the bottom.
//
// The agent loop runs in a background goroutine. Messages flow via channels:
// the TUI sends user input to the loop, the loop sends streaming tokens and
// tool results back. This keeps the UI responsive during long LLM calls.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// chatMsg is one line/block in the conversation pane.
type chatMsg struct {
	role    string // "user", "assistant", "tool", "system"
	content string
}

// Model is the bubbletea model for the VLA TUI.
type Model struct {
	viewport    viewport.Model
	textarea    textarea.Model
	messages    []chatMsg
	width       int
	height      int
	statusInfo  string        // model name, tool count, session ID
	inputReady  chan string   // user submits text here → agent loop
	streamCh    <-chan string // streaming tokens arrive here ← agent loop
	toolCh      <-chan string // tool results arrive here ← agent loop
	doneCh      <-chan bool   // agent loop signals turn complete
	quitting    bool
	streaming   strings.Builder // accumulates the current streaming response
	isStreaming bool
}

// Styles.
var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true) // blue
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // yellow
	systemStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // gray
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
	inputStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

// New creates the TUI model. Channels wire it to the agent loop:
//   - inputReady: the TUI sends user-submitted text here
//   - streamCh: the loop sends streaming LLM tokens here
//   - toolCh: the loop sends tool result summaries here
//   - doneCh: the loop signals when a turn is complete
func New(statusInfo string, inputReady chan string, streamCh <-chan string, toolCh <-chan string, doneCh <-chan bool) Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message... (Ctrl+Enter to submit, Ctrl+C to quit)"
	ta.Focus()
	ta.CharLimit = 0 // unlimited input

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return Model{
		viewport:   vp,
		textarea:   ta,
		statusInfo: statusInfo,
		inputReady: inputReady,
		streamCh:   streamCh,
		toolCh:     toolCh,
		doneCh:     doneCh,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve: status bar (1 line) + input area (4 lines) + gap (1 line).
		convHeight := msg.Height - 6
		if convHeight < 3 {
			convHeight = 3
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = convHeight
		m.textarea.SetWidth(msg.Width - 4)
		m.refreshView()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyCtrlJ:
			// Submit the input (Ctrl+J = linefeed, the standard TUI submit).
			text := strings.TrimSpace(m.textarea.Value())
			if text != "" && !m.isStreaming {
				m.messages = append(m.messages, chatMsg{role: "user", content: text})
				m.textarea.Reset()
				m.isStreaming = true
				m.streaming.Reset()
				m.inputReady <- text
				m.refreshView()
			}
			return m, nil
		}

	// Custom messages from channels (via tea.Tick or polling).
	case streamTickMsg:
		select {
		case token := <-m.streamCh:
			m.streaming.WriteString(token)
			m.updateStreamingView()
			cmds = append(cmds, pollStream(m.streamCh))
		default:
		}
	case toolTickMsg:
		select {
		case toolResult := <-m.toolCh:
			m.messages = append(m.messages, chatMsg{role: "tool", content: toolResult})
			m.refreshView()
			cmds = append(cmds, pollTool(m.toolCh))
		default:
		}
	case doneTickMsg:
		select {
		case <-m.doneCh:
			// Turn complete — flush any remaining streaming content.
			if m.streaming.Len() > 0 {
				m.messages = append(m.messages, chatMsg{role: "assistant", content: m.streaming.String()})
				m.streaming.Reset()
			}
			m.isStreaming = false
			m.refreshView()
			cmds = append(cmds, pollDone(m.doneCh))
		default:
		}
	}

	// Update viewport and textarea.
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	var taCmd tea.Cmd
	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	statusBar := statusStyle.Render(m.statusInfo)
	convPane := m.viewport.View()
	inputPane := inputStyle.Render(m.textarea.View())
	gap := lipgloss.NewStyle().Height(1).Render("")
	return lipgloss.JoinVertical(lipgloss.Left,
		statusBar,
		convPane,
		gap,
		inputPane,
	)
}

// refreshView rebuilds the conversation pane content from messages.
func (m *Model) refreshView() {
	var b strings.Builder
	for _, msg := range m.messages {
		var prefix, content string
		switch msg.role {
		case "user":
			prefix = userStyle.Render("You")
			content = msg.content
		case "assistant":
			prefix = assistantStyle.Render("VLA")
			content = msg.content
		case "tool":
			prefix = toolStyle.Render("⚙")
			content = toolStyle.Render(msg.content)
		case "system":
			prefix = systemStyle.Render("ℹ")
			content = systemStyle.Render(msg.content)
		default:
			prefix = msg.role
			content = msg.content
		}
		fmt.Fprintf(&b, "%s: %s\n\n", prefix, content)
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
	m.viewport.GotoBottom()
}

// updateStreamingView shows the in-progress streaming response.
func (m *Model) updateStreamingView() {
	var b strings.Builder
	for _, msg := range m.messages {
		var prefix, content string
		switch msg.role {
		case "user":
			prefix = userStyle.Render("You")
			content = msg.content
		case "assistant":
			prefix = assistantStyle.Render("VLA")
			content = msg.content
		case "tool":
			prefix = toolStyle.Render("⚙")
			content = toolStyle.Render(msg.content)
		case "system":
			prefix = systemStyle.Render("ℹ")
			content = systemStyle.Render(msg.content)
		default:
			prefix = msg.role
			content = msg.content
		}
		fmt.Fprintf(&b, "%s: %s\n\n", prefix, content)
	}
	// Append the streaming content.
	if m.streaming.Len() > 0 {
		fmt.Fprintf(&b, "%s: %s▌\n", assistantStyle.Render("VLA"), m.streaming.String())
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
	m.viewport.GotoBottom()
}

// --- Channel polling commands ---

type streamTickMsg struct{}
type toolTickMsg struct{}
type doneTickMsg struct{}

func pollStream(_ <-chan string) tea.Cmd {
	return tea.Tick(0, func(time.Time) tea.Msg { return streamTickMsg{} })
}

func pollTool(_ <-chan string) tea.Cmd {
	return tea.Tick(0, func(time.Time) tea.Msg { return toolTickMsg{} })
}

func pollDone(_ <-chan bool) tea.Cmd {
	return tea.Tick(0, func(time.Time) tea.Msg { return doneTickMsg{} })
}
