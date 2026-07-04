package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/abrandt/vla/internal/tools"
)

// ToolApprover decides whether a tool call needs human approval before
// executing. Defined here to avoid importing the approval package (which
// would create a dependency). The production approver lives in the approval
// package; tests use AlwaysApprover.
type ToolApprover interface {
	// RequiresApproval returns true if the tool name needs a human checkpoint.
	RequiresApproval(toolName string) bool
	// Approve asks the user to approve a tool call. Returns true if approved,
	// false if denied. The preview is a human-readable description of the change.
	Approve(toolName string, args map[string]any, preview string) bool
}

// HookRunner executes user-defined hooks before/after tool calls.
// Defined here to avoid importing the hooks package.
type HookRunner interface {
	RunBeforeTool(toolName string) error
	RunAfterTool(toolName, result string)
}

// CommandHandler executes slash commands (/help, /tools, etc.). If set,
// the loop intercepts user input starting with "/" and calls this instead
// of sending the message to the LLM.
type CommandHandler func(input string) (output string, handled bool)

// ToolPermissionChecker checks if a tool is blocked by permission rules.
// Returns true if the tool is allowed to proceed (not denied).
type ToolPermissionChecker interface {
	IsBlocked(toolName string) bool
}

// TranscriptWriter persists a message to the on-disk transcript. It is
// satisfied by a closure wrapping session.Session.Append; defined here to
// avoid an import cycle (session doesn't import agent, but we keep it clean).
// If nil, messages are not persisted (in-memory only).
type TranscriptWriter func(turn map[string]any) error

// InputReader reads one line of user input with a prompt. It's satisfied by
// *readline.Instance in production and by a plainReader in tests. Returns
// the text (without trailing newline) and io.EOF when input is exhausted.
type InputReader interface {
	Readline() (string, error)
	Close() error
}

// Streamer is the minimal LLM-client surface the loop depends on. It is
// satisfied by *llm.Client; defining it here breaks what would otherwise be
// an import cycle (llm imports agent for Message, so agent cannot import llm).
type Streamer interface {
	StreamTo(messages []Message, toolDefs []map[string]any, out io.Writer) (Message, error)
}

// UsageProvider returns accumulated token usage. If the Streamer also
// satisfies this interface (as *llm.Client does), the loop emits EventUsage
// after each LLM call so the TUI can show a live token count.
type UsageProvider interface {
	// TotalUsage returns a snapshot of the accumulated usage. The concrete
	// type must be the llm package's Usage (or layout-compatible); we use a
	// pointer-to-Usage return so the loop can copy fields without importing llm.
	UsageSnapshot() Usage
}

// Summarizer summarizes a slice of messages into a terse string. Defined
// here (mirroring compaction.Summarizer) to avoid the agent↔compaction
// import cycle; the loop receives the real compaction logic via Compactor.
type Summarizer func(msgs []Message) (string, error)

// Compactor reduces the message list for the LLM when it grows too large.
// It is satisfied by compaction.Compact; injected here to avoid an import
// cycle (compaction imports agent for Message).
type Compactor func(msgs []Message, sum Summarizer, threshold int) ([]Message, error)

// Loop is the VLA agent loop. It is created once per session and run with
// an InputReader (readline or plain stdin) and an output writer (terminal).
type Loop struct {
	client     Streamer
	registry   *tools.Registry
	summarizer Summarizer
	compactor  Compactor
	injector   ContextInjector       // optional; nil = no context injection
	writer     TranscriptWriter      // optional; nil = no persistence
	input      InputReader           // set via SetInput; nil = legacy bufio mode
	approver   ToolApprover          // optional; nil = no approval checks
	permCheck  ToolPermissionChecker // optional; nil = no permission checks
	cmdHandler CommandHandler        // optional; nil = no slash commands
	hooks      HookRunner            // optional; nil = no hooks
	events     chan<- Event          // optional; nil = no structured events
	threshold  int
	messages   []Message
}

// NewLoop returns a Loop wired to the given client, tool registry, compactor,
// and summarizer. threshold is the compaction character threshold. client must
// satisfy Streamer (e.g. *llm.Client); compactor is typically compaction.Compact.
func NewLoop(client Streamer, registry *tools.Registry, compactor Compactor, sum Summarizer, threshold int) *Loop {
	return &Loop{
		client:     client,
		registry:   registry,
		compactor:  compactor,
		summarizer: sum,
		threshold:  threshold,
	}
}

// SetContextInjector installs a context injector (e.g. memory auto-injection).
// Must be called before Run.
func (l *Loop) SetContextInjector(inj ContextInjector) {
	l.injector = inj
}

// SetTranscriptWriter installs a persistence hook. Every message (user,
// assistant, tool result) is written to the transcript as it happens.
// Must be called before Run.
func (l *Loop) SetTranscriptWriter(w TranscriptWriter) {
	l.writer = w
}

// SetInput installs an InputReader (e.g. readline instance) for interactive
// line editing, history, and multi-line support. If not called, Run falls
// back to plain bufio.Reader with blank-line-to-submit.
func (l *Loop) SetInput(r InputReader) {
	l.input = r
}

// SetApprover installs a tool approver for human-in-the-loop checkpoints
// before destructive tool calls. If not called, all tools run without asking.
func (l *Loop) SetApprover(a ToolApprover) {
	l.approver = a
}

// SetPermissionChecker installs a permission checker that can block tools
// entirely (before they reach the approver). If not called, no tools are
// blocked.
func (l *Loop) SetPermissionChecker(p ToolPermissionChecker) {
	l.permCheck = p
}

// SetHookRunner installs a hook runner for before/after tool call scripts.
func (l *Loop) SetHookRunner(h HookRunner) {
	l.hooks = h
}

// SetCommandHandler installs a slash command handler. Messages starting with
// "/" are intercepted and handled locally instead of sent to the LLM.
func (l *Loop) SetCommandHandler(h CommandHandler) {
	l.cmdHandler = h
}

// SetEventChan installs a channel for structured events (tool start/result,
// turn boundaries, usage). The TUI uses these to render tool-call blocks,
// spinners, and a live status bar. Events are non-blocking (dropped if the
// channel is full). Must be called before Run.
func (l *Loop) SetEventChan(ch chan<- Event) {
	l.events = ch
}

// emitUsage checks if the Streamer provides usage data and, if so, emits an
// EventUsage. Called after each LLM API call.
func (l *Loop) emitUsage() {
	if l.events == nil {
		return
	}
	if up, ok := l.client.(UsageProvider); ok {
		u := up.UsageSnapshot()
		l.emitEvent(Event{Type: EventUsage, Usage: &u})
	}
}

// LoadMessages restores the in-memory message list from a prior session.
// Call this before Run to resume a conversation (--resume).
func (l *Loop) LoadMessages(msgs []Message) {
	l.messages = msgs
}

// persist writes one message to the transcript if a writer is configured.
func (l *Loop) persist(role Role, content string, toolCalls []ToolCall, toolCallID string) {
	if l.writer == nil {
		return
	}
	turn := map[string]any{
		"type":      "turn",
		"role":      string(role),
		"content":   content,
		"timestamp": nowISO(),
	}
	if len(toolCalls) > 0 {
		turn["tool_calls"] = toolCalls
	}
	if toolCallID != "" {
		turn["tool_call_id"] = toolCallID
	}
	_ = l.writer(turn) // best-effort; don't crash the loop on write failure
}

// Run reads user messages and writes assistant responses + tool results to
// out. If an InputReader was set via SetInput, it uses readline (line editing,
// history). Otherwise it reads from in with blank-line-to-submit (for tests).
func (l *Loop) Run(in io.Reader, out io.Writer) error {
	if l.input != nil {
		return l.runReadline(out)
	}
	return l.runPlain(in, out)
}

// runReadline uses the InputReader (readline in production) for input.
func (l *Loop) runReadline(out io.Writer) error {
	for {
		text, err := l.input.Readline()
		if err == io.EOF {
			fmt.Fprintln(out)
			return nil
		}
		if err != nil {
			return fmt.Errorf("agent: read input: %w", err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		// Intercept slash commands.
		if l.cmdHandler != nil {
			if output, handled := l.cmdHandler(text); handled {
				fmt.Fprintln(out, output)
				continue
			}
		}

		l.messages = append(l.messages, Message{Role: RoleUser, Content: text})
		l.persist(RoleUser, text, nil, "")
		if err := l.turn(out); err != nil {
			return err
		}
	}
}

// runPlain uses a bufio reader with blank-line-to-submit. Used by tests and
// when readline is not available (e.g. piped input).
func (l *Loop) runPlain(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "> ")
		text, err := readMessage(reader)
		if err == io.EOF {
			fmt.Fprintln(out)
			return nil
		}
		if err != nil {
			return fmt.Errorf("agent: read input: %w", err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		// Intercept slash commands.
		if l.cmdHandler != nil {
			if output, handled := l.cmdHandler(text); handled {
				fmt.Fprintln(out, output)
				continue
			}
		}

		l.messages = append(l.messages, Message{Role: RoleUser, Content: text})
		l.persist(RoleUser, text, nil, "")
		if err := l.turn(out); err != nil {
			return err
		}
	}
}

// MaxTurns is the maximum number of LLM calls within a single turn (i.e.
// consecutive tool-call → re-call cycles). Prevents runaway loops where
// the model keeps requesting tool calls forever. 50 is generous — real
// tasks rarely need more than 10-15.
const MaxTurns = 50

// turn executes one full agent turn: call LLM → stream → execute tool calls
// → loop until the LLM responds without tool calls.
func (l *Loop) turn(out io.Writer) error {
	l.emitEvent(Event{Type: EventTurnStart})
	defer l.emitEvent(Event{Type: EventTurnEnd})

	for iter := 0; iter < MaxTurns; iter++ {
		view, err := l.compactor(l.messages, l.summarizer, l.threshold)
		if err != nil {
			return err
		}
		if l.injector != nil {
			view = l.injector(view, lastUserContent(l.messages))
		}

		msg, err := l.client.StreamTo(view, l.registry.Schemas(), out)
		if err != nil {
			return err
		}
		l.emitUsage()
		l.messages = append(l.messages, msg)
		l.persist(RoleAssistant, msg.Content, msg.ToolCalls, "")

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, tc := range msg.ToolCalls {
			l.emitEvent(Event{Type: EventToolStart, Tool: tc.Function.Name, Args: tc.Function.Arguments})
			result := l.executeToolCall(tc)
			isError := strings.HasPrefix(result, "Error:")
			l.emitEvent(Event{Type: EventToolResult, Tool: tc.Function.Name, Result: result, Error: isError})
			l.messages = append(l.messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
			l.persist(RoleTool, result, nil, tc.ID)
			fmt.Fprintf(out, "[tool %s → %s]\n", tc.Function.Name, truncate(result, 200))
		}
	}
	// Reached MaxTurns — the model is stuck in a tool-call loop. Abort
	// gracefully and return control to the user.
	fmt.Fprintf(out, "[reached max %d tool-call iterations — returning control to user]\n", MaxTurns)
	return nil
}

// executeToolCall looks up the tool by name and runs it. Per the design,
// tool errors are returned as result strings — they never break the loop.
// Permission checks run first (deny = blocked entirely). If the tool requires
// approval and an approver is set, the user is asked before execution.
func (l *Loop) executeToolCall(tc ToolCall) string {
	// Permission check — blocked tools never execute.
	if l.permCheck != nil && l.permCheck.IsBlocked(tc.Function.Name) {
		return fmt.Sprintf("Error: tool %q is blocked by permission rules", tc.Function.Name)
	}

	// Approval check — destructive tools ask the user first.
	if l.approver != nil && l.approver.RequiresApproval(tc.Function.Name) {
		args := parseArgs(tc.Function.Arguments)
		preview := buildPreview(tc.Function.Name, args)
		if !l.approver.Approve(tc.Function.Name, args, preview) {
			return fmt.Sprintf("Tool %q was denied by the user.", tc.Function.Name)
		}
	}

	// Hook: before_tool (can block).
	if l.hooks != nil {
		if err := l.hooks.RunBeforeTool(tc.Function.Name); err != nil {
			return fmt.Sprintf("Error: blocked by before_tool hook: %v", err)
		}
	}

	tool, ok := l.registry.Get(tc.Function.Name)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
	}
	result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
	if err != nil {
		result = fmt.Sprintf("Error: %s: %v", tc.Function.Name, err)
	}
	// Hook: after_tool (non-blocking, runs after tool completes).
	if l.hooks != nil {
		l.hooks.RunAfterTool(tc.Function.Name, result)
	}
	return result
}

// readMessage reads one user message: lines until a blank line is entered.
// A blank line submits the accumulated text. EOF returns io.EOF.
func readMessage(r *bufio.Reader) (string, error) {
	var b strings.Builder
	sawAny := false
	for {
		line, err := r.ReadString('\n')
		stripped := strings.TrimRight(line, "\r\n")
		if stripped == "" {
			if err == io.EOF {
				if sawAny {
					return b.String(), nil
				}
				return "", io.EOF
			}
			if b.Len() > 0 {
				return b.String(), nil
			}
			continue
		}
		sawAny = true
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(stripped)
		if err == io.EOF {
			return b.String(), nil
		}
	}
}

// truncate shortens s to at most n chars, appending "…" if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// lastUserContent returns the content of the most recent user message, or
// empty string if there isn't one. Used by the context injector to know what
// the current query is about.
func lastUserContent(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// nowISO returns the current time in RFC 3339 format for transcript timestamps.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// parseArgs unmarshals the tool call arguments JSON into a map. Returns an
// empty map on parse failure (non-fatal — the approval preview just shows
// less detail).
func parseArgs(argsJSON string) map[string]any {
	var args map[string]any
	if argsJSON != "" && argsJSON != "null" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args
}

// buildPreview creates a human-readable preview of what the tool will do,
// shown in the approval prompt. For write/update it shows the file path and
// content snippet; for delete it shows the path; for git_commit the message.
func buildPreview(toolName string, args map[string]any) string {
	path, _ := args["path"].(string)
	switch toolName {
	case "write_file":
		content, _ := args["content"].(string)
		return fmt.Sprintf("WRITE %s (%d bytes):\n%s", path, len(content), truncateStr(content, 500))
	case "update_file":
		old, _ := args["old_string"].(string)
		newStr, _ := args["new_string"].(string)
		return fmt.Sprintf("UPDATE %s:\n- %s\n+ %s", path, truncateStr(old, 200), truncateStr(newStr, 200))
	case "delete_file":
		return fmt.Sprintf("DELETE %s", path)
	case "git_commit":
		msg, _ := args["message"].(string)
		return fmt.Sprintf("GIT COMMIT: %s", msg)
	default:
		return fmt.Sprintf("%s %v", toolName, args)
	}
}

// truncateStr shortens s to at most n chars, appending "…" if truncated.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
