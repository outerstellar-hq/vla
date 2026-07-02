// Package agent implements the VLA core agent loop and the message types
// shared between the transcript, the LLM client, and the tools.
package agent

// Role identifies who produced a message in the conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a single function call requested by the assistant.
// The OpenAI API streams this in fragments; the LLM client assembles
// the complete call before appending it to the transcript.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function name + arguments pair inside a ToolCall.
// Arguments is a raw JSON string (per the OpenAI API contract, not a parsed object).
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string, parsed by the tool
}

// Message is one turn in the conversation. It maps to the OpenAI chat
// completions message shape, with a few VLA-specific fields (Timestamp,
// Kind) used only by the transcript layer.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // set when Role == RoleTool
	Timestamp  string     `json:"timestamp,omitempty"`    // ISO-8601, set by the transcript layer
}
