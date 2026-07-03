// Package approval implements VLA's diff approval system — the human-in-the-
// loop checkpoint before destructive tool calls (write_file, update_file,
// delete_file, git_commit).
//
// The Approver interface is injected into the agent loop. Each approver
// decides whether a tool call should proceed:
//   - AlwaysApprover: auto-approves everything (--yes flag or piped input)
//   - ReadlineApprover: shows a preview and waits for y/n at the terminal
//   - PermissionApprover: combines permission rules + approval (see permissions package)
package approval

// Action describes what the approver decided.
type Action int

const (
	ActionAllow Action = iota // proceed with the tool call
	ActionDeny                // block the tool call, return "denied by user" to LLM
)

// Decision is the result of an approval check.
type Decision struct {
	Action  Action
	Message string // human-readable reason (shown to the LLM if denied)
}

// Request describes what the approver is evaluating.
type Request struct {
	ToolName string         // e.g. "write_file", "delete_file"
	Args     map[string]any // the tool's arguments
	Preview  string         // human-readable preview of the change (diff, file path, etc.)
}

// Approver decides whether a tool call should proceed. The agent loop calls
// Approve before executing any tool that requires approval. Read-only tools
// (read_file, search, list_files, etc.) bypass the approver entirely.
type Approver interface {
	Approve(req Request) Decision
}

// AlwaysApprover auto-approves everything. Used with --yes flag or when
// input is piped (no interactive terminal to confirm with).
type AlwaysApprover struct{}

func (AlwaysApprover) Approve(_ Request) Decision {
	return Decision{Action: ActionAllow}
}

// RequireApproval lists tool names that need human approval before executing.
// Tools not in this list (read_file, search, list_files, go_to_definition,
// find_references, hover, diagnostics, web_search, web_read, memory_*)
// run without asking.
var RequireApproval = map[string]bool{
	"write_file":  true,
	"update_file": true,
	"delete_file": true,
	"git_commit":  true,
}

// RequiresApproval returns true if the tool name needs human approval.
func RequiresApproval(toolName string) bool {
	return RequireApproval[toolName]
}

// AllowAll returns an AlwaysApprover (for --yes or piped input).
func AllowAll() Approver {
	return AlwaysApprover{}
}
