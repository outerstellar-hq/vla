package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// blockType identifies what kind of content a block holds.
type blockType int

const (
	blockUser blockType = iota
	blockAssistant
	blockTool
	blockSystem
	blockError
)

// toolStatus tracks the lifecycle of a tool-call block.
type toolStatus int

const (
	toolRunning toolStatus = iota
	toolDone
	toolDenied
	toolBlocked
)

// block is one rendered unit in the conversation pane. Each user message,
// assistant response, tool call, or system notice is one block.
type block struct {
	typ        blockType
	content    string // rendered text (assistant messages are markdown-rendered at display time)
	toolName   string
	toolArgs   string
	toolResult string
	status     toolStatus
	expanded   bool // for tool blocks: show full args + result
}

// blockStyles holds lipgloss styles for block rendering.
var (
	userLabelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	assistantLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	toolLabelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	systemLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorLabelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	toolNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	toolPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	toolOkStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	toolErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	toolRunStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	codeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)
)

// renderBlock renders a single block to a string for the conversation pane.
// width is the available viewport width (used for markdown rendering).
func renderBlock(b block, width int) string {
	switch b.typ {
	case blockUser:
		return renderUserBlock(b)
	case blockAssistant:
		return renderAssistantBlock(b, width)
	case blockTool:
		return renderToolBlock(b, width)
	case blockSystem:
		return renderSystemBlock(b)
	case blockError:
		return renderErrorBlock(b)
	default:
		return b.content
	}
}

func renderUserBlock(b block) string {
	label := userLabelStyle.Render("You")
	return fmt.Sprintf("%s: %s", label, b.content)
}

func renderAssistantBlock(b block, width int) string {
	label := assistantLabelStyle.Render("VLA")
	rendered := renderMarkdown(b.content, width)
	return fmt.Sprintf("%s: %s", label, rendered)
}

func renderSystemBlock(b block) string {
	label := systemLabelStyle.Render("ℹ")
	return fmt.Sprintf("%s %s", label, systemLabelStyle.Render(b.content))
}

func renderErrorBlock(b block) string {
	label := errorLabelStyle.Render("✗")
	return fmt.Sprintf("%s %s", label, errorLabelStyle.Render(b.content))
}

// renderToolBlock renders a tool-call block. Collapsed by default: one line
// showing the tool name, a short arg summary, and status. Expanded: shows
// the full args (pretty JSON) and result (truncated to keep the view usable).
func renderToolBlock(b block, width int) string {
	var statusIcon string
	switch b.status {
	case toolRunning:
		statusIcon = toolRunStyle.Render("⟳")
	case toolDone:
		statusIcon = toolOkStyle.Render("✓")
	case toolDenied:
		statusIcon = toolErrStyle.Render("⊘")
	case toolBlocked:
		statusIcon = toolErrStyle.Render("⊘")
	default:
		statusIcon = toolOkStyle.Render("✓")
	}

	name := toolNameStyle.Render(b.toolName)
	summary := toolArgSummary(b.toolName, b.toolArgs)

	// Collapsed view: ⚙ read_file (/path/to/file.go) ✓
	label := toolLabelStyle.Render("⚙")
	header := fmt.Sprintf("%s %s %s %s", label, name, toolPathStyle.Render(summary), statusIcon)
	header = strings.TrimSpace(header)

	if !b.expanded {
		return header
	}

	// Expanded view: header + args + result
	var details strings.Builder
	details.WriteString(header)
	details.WriteString("\n")

	// Pretty-printed args
	if b.toolArgs != "" && b.toolArgs != "{}" {
		pretty := prettyJSON(b.toolArgs)
		details.WriteString("  ")
		details.WriteString(systemLabelStyle.Render("args:"))
		details.WriteString("\n")
		for _, line := range strings.Split(pretty, "\n") {
			details.WriteString("    ")
			details.WriteString(toolPathStyle.Render(line))
			details.WriteString("\n")
		}
	}

	// Result (truncated for display)
	if b.toolResult != "" {
		result := b.toolResult
		if len(result) > 500 {
			result = result[:500] + "…"
		}
		details.WriteString("  ")
		details.WriteString(systemLabelStyle.Render("result:"))
		details.WriteString("\n")
		for _, line := range strings.Split(result, "\n") {
			details.WriteString("    ")
			details.WriteString(line)
			details.WriteString("\n")
		}
	}

	return strings.TrimRight(details.String(), "\n")
}

// toolArgSummary extracts the most relevant argument (usually "path") for
// the collapsed tool display. Falls back to a truncated raw arg string.
func toolArgSummary(toolName, argsJSON string) string {
	if argsJSON == "" || argsJSON == "{}" {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return truncateStr(argsJSON, 40)
	}

	// Priority: path > file > name > query > pattern > first string value.
	for _, key := range []string{"path", "file", "name", "query", "pattern", "command", "url"} {
		if v, ok := args[key].(string); ok && v != "" {
			return fmt.Sprintf("(%s)", truncateStr(v, 50))
		}
	}

	// Show arg count if nothing more specific.
	if len(args) > 0 {
		return fmt.Sprintf("(%d args)", len(args))
	}
	return ""
}

// prettyJSON reformats a JSON string with 2-space indentation.
func prettyJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	return string(out)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
