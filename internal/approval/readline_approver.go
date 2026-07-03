package approval

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadlineApprover prompts the user at the terminal before destructive tool
// calls. Shows a preview and waits for y/n. Used in interactive (readline)
// mode; the TUI mode handles approval through its own input channel.
type ReadlineApprover struct {
	approveAll bool // once the user says "a" (approve all), skip future prompts
}

// NewReadlineApprover creates an interactive approver.
func NewReadlineApprover() *ReadlineApprover {
	return &ReadlineApprover{}
}

// RequiresApproval returns true for tools in the RequireApproval list.
func (a *ReadlineApprover) RequiresApproval(toolName string) bool {
	return RequiresApproval(toolName)
}

// Approve shows the preview and waits for user input.
// Returns true if approved, false if denied.
func (a *ReadlineApprover) Approve(toolName string, args map[string]any, preview string) bool {
	if a.approveAll {
		return true
	}
	fmt.Fprintf(os.Stderr, "\n┌─ Approval needed: %s ─────────────────────\n", toolName)
	fmt.Fprintf(os.Stderr, "%s\n", preview)
	fmt.Fprintf(os.Stderr, "Allow? [y]es / [n]o / [a]ll (approve rest of session): ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))

	switch answer {
	case "y", "yes":
		return true
	case "a", "all":
		a.approveAll = true
		return true
	default:
		fmt.Fprintf(os.Stderr, "→ Denied.\n\n")
		return false
	}
}
