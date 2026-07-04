// Package agent implements VLA's core agent loop: the cycle of reading user
// input, streaming it to an LLM, executing tool calls the LLM requests, and
// feeding results back. The loop handles compaction (summarizing old context),
// persistence (NDJSON transcripts), slash commands, hooks, permissions, and
// human-in-the-loop approval for destructive tools.
//
// The package defines the Streamer, Compactor, ToolApprover, HookRunner,
// and other interfaces to break import cycles (the llm and compaction
// packages import agent for Message, so agent cannot import them).
//
// Key types:
//   - Loop: the main agent loop, created once per session
//   - Message: OpenAI-compatible chat message (role, content, tool calls)
//   - Event: typed notifications emitted to the TUI (tool start/result, usage)
//   - Coordinator: parallel sub-agent execution
package agent

// EventType identifies what kind of structured event the loop is emitting
// to the TUI (or any other observer). These flow over the event channel
// set via SetEventChan, parallel to the raw streaming text that goes
// through the io.Writer.
type EventType int

const (
	EventTurnStart EventType = iota
	EventTurnEnd
	EventToolStart  // a tool call is about to execute
	EventToolResult // a tool call finished (or errored)
	EventUsage      // token usage update (after each LLM call)
	EventApprovalReq
)

// Event is a single structured notification from the agent loop.
// Only the fields relevant to Type are populated; the rest are zero.
//
//   - EventTurnStart / EventTurnEnd: no fields
//   - EventToolStart:                Tool, Args
//   - EventToolResult:               Tool, Result, Error
//   - EventUsage:                    Usage (pointer; nil = no usage data)
//   - EventApprovalReq:              Tool, Args, Preview + ID (for response matching)
type Event struct {
	Type    EventType
	Tool    string
	Args    string
	Result  string
	Error   bool
	Usage   *Usage
	Preview string
	ID      string // for approval requests: an ID the responder quotes back
}

// Usage carries token counts for one LLM API call. Mirrors llm.Usage but
// defined here to avoid an import cycle (llm imports agent).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// emitEvent sends an event on the loop's event channel if one is configured.
// Non-blocking: if the channel is full or nil, the event is dropped (the TUI
// is a consumer of best-effort notifications, not a critical path).
func (l *Loop) emitEvent(e Event) {
	if l.events == nil {
		return
	}
	select {
	case l.events <- e:
	default:
		// Channel full — drop the event rather than blocking the loop.
	}
}
