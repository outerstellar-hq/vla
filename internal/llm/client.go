// Package llm implements an OpenAI-compatible streaming chat-completions client.
// It posts a request with stream:true, parses the SSE response token by token,
// prints text deltas to an output writer as they arrive, and accumulates
// tool-call fragments into complete calls.
package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abrandt/vla/internal/agent"
)

// Client is an OpenAI-compatible streaming chat-completions client.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// NewClient returns a streaming client for the given config.
func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{},
	}
}

// request is the body posted to /chat/completions.
type request struct {
	Model    string           `json:"model"`
	Messages []agent.Message  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

// streamChunk models the relevant fields of one SSE chunk.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Stream sends messages to the model, streams text deltas to os.Stdout as they
// arrive, and returns the fully-assembled assistant message. toolDefs is the
// output of tools.Registry.Schemas() (may be nil).
//
// Returns an error on HTTP failures (non-2xx), network errors, or malformed
// SSE lines — these are infrastructure failures the agent loop treats as aborts.
func (c *Client) Stream(messages []agent.Message, toolDefs []map[string]any) (agent.Message, error) {
	return c.StreamTo(messages, toolDefs, nil)
}

// StreamTo is like Stream but writes text deltas to out instead of discarding
// them. If out is nil, deltas are accumulated but not printed (useful for tests
// and for the summarization call during compaction, which uses io.Discard).
func (c *Client) StreamTo(messages []agent.Message, toolDefs []map[string]any, out io.Writer) (agent.Message, error) {
	body, err := json.Marshal(request{
		Model:    c.model,
		Messages: messages,
		Tools:    toolDefs,
		Stream:   true,
	})
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return agent.Message{}, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return parseSSE(resp.Body, out)
}

// parseSSE reads the SSE stream, writes text deltas to out (if non-nil), and
// accumulates the assistant message (content + assembled tool calls).
func parseSSE(r io.Reader, out io.Writer) (agent.Message, error) {
	msg := agent.Message{Role: agent.RoleAssistant}
	type tcAcc struct {
		ID   string
		Name string
		Args strings.Builder
	}
	acc := map[int]*tcAcc{}
	var order []int

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return msg, fmt.Errorf("llm: parse SSE chunk: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			msg.Content += delta.Content
			if out != nil {
				if _, err := io.WriteString(out, delta.Content); err != nil {
					return msg, fmt.Errorf("llm: write stream output: %w", err)
				}
			}
		}
		for _, fr := range delta.ToolCalls {
			a, ok := acc[fr.Index]
			if !ok {
				a = &tcAcc{}
				acc[fr.Index] = a
				order = append(order, fr.Index)
			}
			if fr.ID != "" {
				a.ID = fr.ID
			}
			if fr.Function.Name != "" {
				a.Name = fr.Function.Name
			}
			if fr.Function.Arguments != "" {
				a.Args.WriteString(fr.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return msg, fmt.Errorf("llm: read stream: %w", err)
	}

	for _, idx := range order {
		a := acc[idx]
		msg.ToolCalls = append(msg.ToolCalls, agent.ToolCall{
			ID:   a.ID,
			Type: "function",
			Function: agent.FunctionCall{
				Name:      a.Name,
				Arguments: a.Args.String(),
			},
		})
	}
	if out != nil && msg.Content != "" {
		_, _ = io.WriteString(out, "\n")
	}
	return msg, nil
}
