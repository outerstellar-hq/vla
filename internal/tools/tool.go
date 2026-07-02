// Package tools defines the VLA tool framework: the Tool interface every
// tool implements, and the Registry that collects tools and exposes their
// schemas to the LLM. Each concrete tool lives in its own file under
// builtin/ and is fully self-contained.
package tools

import "encoding/json"

// Tool is the interface every VLA tool implements.
// To alter a tool's schema or behavior, edit its file — no central
// registry changes required beyond the one-line registration call.
type Tool interface {
	// Name returns the tool's unique identifier (e.g. "echo").
	Name() string

	// Schema returns the OpenAI function-calling JSON schema for this
	// tool's parameters — what the LLM sees. The caller wraps this with
	// the tool's Name() to form the full tool definition.
	Schema() map[string]any

	// Execute runs the tool with the given raw JSON arguments and
	// returns the result string. Tools parse args themselves via
	// json.Unmarshal into a tool-specific struct for type safety.
	// Per the design: tool errors are returned as result strings
	// ("Error: ..."), NOT as a Go error, so the loop can feed them
	// back to the LLM. The error return is reserved for failures the
	// loop cannot recover from (which, by convention, tools never raise).
	Execute(args json.RawMessage) (string, error)
}
