// Package builtin holds VLA's built-in tools. Each tool is a self-contained
// struct in its own file implementing tools.Tool.
package builtin

import (
	"encoding/json"
	"fmt"
)

// Echo is a trivial tool that returns its input text. It exists to prove
// the agent loop end-to-end: LLM calls it, the loop executes it, the
// result lands in the transcript. Replace or extend it with real tools
// (read_file, etc.) in later builds.
type Echo struct{}

// Name returns the tool's identifier.
func (Echo) Name() string { return "echo" }

// Schema returns the OpenAI function-calling parameter schema.
func (Echo) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "The text to echo back.",
			},
		},
		"required": []string{"text"},
	}
}

// Execute parses the arguments and returns the text unchanged.
// Per the design, malformed arguments come back as an error *string*
// (not a Go error) so the LLM can react and retry.
func (Echo) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: could not parse echo arguments: %v", err), nil
	}
	if in.Text == "" {
		return "Error: text is required", nil
	}
	return in.Text, nil
}
