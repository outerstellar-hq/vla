// Package commands implements VLA's slash commands — messages starting with
// "/" that are handled locally (not sent to the LLM). They let the user
// inspect state, manage memory, and control the session directly.
//
// The agent loop checks IsSlashCommand before sending a message to the LLM.
// If it is one, Execute runs the command and returns the output string
// (displayed to the user) instead of making an LLM call.
package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abrandt/vla/internal/tools"
)

// Context provides everything slash commands need to execute.
type Context struct {
	Registry       *tools.Registry
	Model          string
	SessionID      string
	ToolCount      int
	MemSearch      func(query string) (string, error)     // memory_search shortcut
	MemSave        func(content string) (string, error)   // memory_save shortcut
	TriggerCompact func()                                 // manually trigger compaction
	GetUsage       func() (prompt, completion, total int) // token usage
	GetCost        func() float64                         // accumulated cost in USD
	UndoFunc       func() (string, error)                 // undo last file change
	UndoCount      func() int                             // count of undoable changes
	DiffFunc       func() (string, error)                 // session-wide git diff
	AttachImage    func(path string) (string, error)      // attach image to next message
	SpawnAgent     func(task string) (string, error)      // dispatch sub-agent for parallel task
}

// Result is the output of a slash command.
type Result struct {
	Output  string // displayed to the user
	Handled bool   // true if this was a valid command
}

// IsSlashCommand returns true if the input starts with "/".
func IsSlashCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// Execute runs a slash command and returns the result.
// If the command is unknown, returns Handled=false so the caller can treat
// it as a normal message (or show an error).
func Execute(input string, ctx Context) Result {
	input = strings.TrimSpace(input)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return Result{Handled: false}
	}
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/help":
		return Result{Output: helpText(), Handled: true}

	case "/tools":
		return Result{Output: listTools(ctx), Handled: true}

	case "/model":
		if len(args) == 0 {
			return Result{Output: fmt.Sprintf("Current model: %s", ctx.Model), Handled: true}
		}
		return Result{Output: "Model switching requires restart. Use: vla --model " + args[0], Handled: true}

	case "/memory":
		return executeMemory(args, ctx)

	case "/compact":
		if ctx.TriggerCompact != nil {
			ctx.TriggerCompact()
			return Result{Output: "Compaction triggered.", Handled: true}
		}
		return Result{Output: "Compaction not available.", Handled: true}

	case "/session":
		return Result{Output: fmt.Sprintf("Session: %s\nModel: %s\nTools: %d", ctx.SessionID, ctx.Model, ctx.ToolCount), Handled: true}

	case "/cost":
		return executeCost(ctx)

	case "/undo":
		return executeUndo(ctx)

	case "/diff":
		return executeDiff(ctx)

	case "/image":
		return executeImage(ctx, parts)

	case "/spawn":
		return executeSpawn(ctx, parts)

	case "/clear":
		return Result{Output: "Use Ctrl+C to exit and start a new session.", Handled: true}

	default:
		return Result{
			Output:  fmt.Sprintf("Unknown command: %s. Type /help for available commands.", cmd),
			Handled: true,
		}
	}
}

func helpText() string {
	return `VLA Slash Commands:
  /help              Show this help
  /tools             List all registered tools
  /model [name]      Show or change the model (requires restart)
  /memory <cmd>      Memory operations (see below)
  /compact           Manually trigger context compaction
  /cost              Show token usage and estimated cost
  /clear             Exit and start fresh (Ctrl+C)

Memory commands:
  /memory search <query>   Search stored memories
  /memory save <text>      Save a memory
  /memory list             List all memories`
}

func listTools(ctx Context) string {
	if ctx.Registry == nil {
		return "No tools registered."
	}
	schemas := ctx.Registry.Schemas()
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if fn, ok := s["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return fmt.Sprintf("%d tools:\n  %s", len(names), strings.Join(names, "\n  "))
}

func executeCost(ctx Context) Result {
	var lines []string
	if ctx.GetUsage != nil {
		prompt, completion, total := ctx.GetUsage()
		lines = append(lines, fmt.Sprintf("Tokens: %d prompt + %d completion = %d total",
			prompt, completion, total))
	}
	if ctx.GetCost != nil {
		cost := ctx.GetCost()
		lines = append(lines, fmt.Sprintf("Estimated cost: $%.4f", cost))
	}
	if len(lines) == 0 {
		return Result{Output: "Cost tracking not available.", Handled: true}
	}
	return Result{Output: strings.Join(lines, "\n"), Handled: true}
}

func executeUndo(ctx Context) Result {
	if ctx.UndoFunc == nil {
		return Result{Output: "Undo not available.", Handled: true}
	}
	// Show how many changes are available.
	if ctx.UndoCount != nil && ctx.UndoCount() == 0 {
		return Result{Output: "Nothing to undo.", Handled: true}
	}
	path, err := ctx.UndoFunc()
	if err != nil {
		return Result{Output: fmt.Sprintf("Undo failed: %v", err), Handled: true}
	}
	if path == "" {
		return Result{Output: "Nothing to undo.", Handled: true}
	}
	remaining := 0
	if ctx.UndoCount != nil {
		remaining = ctx.UndoCount()
	}
	return Result{
		Output:  fmt.Sprintf("Undid change to %s (%d changes remaining)", path, remaining),
		Handled: true,
	}
}

func executeDiff(ctx Context) Result {
	if ctx.DiffFunc == nil {
		return Result{Output: "Diff not available.", Handled: true}
	}
	diff, err := ctx.DiffFunc()
	if err != nil {
		return Result{Output: fmt.Sprintf("Error: %v", err), Handled: true}
	}
	if diff == "" {
		return Result{Output: "No changes detected.", Handled: true}
	}
	return Result{Output: diff, Handled: true}
}

func executeImage(ctx Context, args []string) Result {
	if ctx.AttachImage == nil {
		return Result{Output: "Image attachment not available.", Handled: true}
	}
	if len(args) == 0 {
		return Result{Output: "Usage: /image <path-to-image>", Handled: true}
	}
	path := strings.Join(args, " ")
	result, err := ctx.AttachImage(path)
	if err != nil {
		return Result{Output: fmt.Sprintf("Error: %v", err), Handled: true}
	}
	return Result{Output: result, Handled: true}
}

func executeSpawn(ctx Context, args []string) Result {
	if ctx.SpawnAgent == nil {
		return Result{Output: "Sub-agent dispatch not available.", Handled: true}
	}
	if len(args) == 0 {
		return Result{Output: "Usage: /spawn <task description>\n\nDispatches a sub-agent to work on a task in parallel.", Handled: true}
	}
	task := strings.Join(args, " ")
	result, err := ctx.SpawnAgent(task)
	if err != nil {
		return Result{Output: fmt.Sprintf("Sub-agent error: %v", err), Handled: true}
	}
	return Result{Output: "Sub-agent result:\n" + result, Handled: true}
}

func executeMemory(args []string, ctx Context) Result {
	if len(args) == 0 {
		return Result{Output: "Usage: /memory <search|save|list> [args]", Handled: true}
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "search":
		if len(args) < 2 {
			return Result{Output: "Usage: /memory search <query>", Handled: true}
		}
		query := strings.Join(args[1:], " ")
		if ctx.MemSearch != nil {
			out, err := ctx.MemSearch(query)
			if err != nil {
				return Result{Output: fmt.Sprintf("Error: %v", err), Handled: true}
			}
			return Result{Output: out, Handled: true}
		}
		return Result{Output: "Memory search not available.", Handled: true}

	case "save":
		if len(args) < 2 {
			return Result{Output: "Usage: /memory save <text>", Handled: true}
		}
		text := strings.Join(args[1:], " ")
		if ctx.MemSave != nil {
			out, err := ctx.MemSave(text)
			if err != nil {
				return Result{Output: fmt.Sprintf("Error: %v", err), Handled: true}
			}
			return Result{Output: out, Handled: true}
		}
		return Result{Output: "Memory save not available.", Handled: true}

	case "list":
		// Reuse search with empty query to list all.
		if ctx.MemSearch != nil {
			out, err := ctx.MemSearch("")
			if err != nil {
				return Result{Output: fmt.Sprintf("Error: %v", err), Handled: true}
			}
			return Result{Output: out, Handled: true}
		}
		return Result{Output: "Memory not available.", Handled: true}

	default:
		return Result{Output: "Unknown memory command: " + sub + ". Use search, save, or list.", Handled: true}
	}
}
