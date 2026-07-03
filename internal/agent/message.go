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

// ContentPart is one part of a multimodal message (text or image).
// When a message has ContentParts, the LLM client sends content as an array
// instead of a plain string (OpenAI multimodal format).
type ContentPart struct {
	Type     string `json:"type"` // "text" or "image_url"
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"` // data:image/png;base64,... or https://...
	} `json:"image_url,omitempty"`
}

// Message is one turn in the conversation. It maps to the OpenAI chat
// completions message shape, with a few VLA-specific fields (Timestamp,
// ContentParts) used for multimodal and the transcript layer.
type Message struct {
	Role         Role          `json:"role"`
	Content      string        `json:"content,omitempty"`
	ContentParts []ContentPart `json:"-"` // multimodal: sent as content array when non-empty
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"` // set when Role == RoleTool
	Timestamp    string        `json:"timestamp,omitempty"`    // ISO-8601, set by the transcript layer
}

// HasImage returns true if the message contains image content parts.
func (m Message) HasImage() bool {
	for _, p := range m.ContentParts {
		if p.Type == "image_url" {
			return true
		}
	}
	return false
}
