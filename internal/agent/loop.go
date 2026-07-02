package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/abrandt/vla/internal/tools"
)

// Streamer is the minimal LLM-client surface the loop depends on. It is
// satisfied by *llm.Client; defining it here breaks what would otherwise be
// an import cycle (llm imports agent for Message, so agent cannot import llm).
type Streamer interface {
	StreamTo(messages []Message, toolDefs []map[string]any, out io.Writer) (Message, error)
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
// a user-input reader and an output writer (the terminal).
type Loop struct {
	client     Streamer
	registry   *tools.Registry
	summarizer Summarizer
	compactor  Compactor
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

// Run reads user messages from in (one per blank-line-terminated block) and
// writes assistant responses + tool results to out. Continues reading input
// until EOF or error.
func (l *Loop) Run(in io.Reader, out io.Writer) error {
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

		l.messages = append(l.messages, Message{Role: RoleUser, Content: text})
		if err := l.turn(out); err != nil {
			return err
		}
	}
}

// turn executes one full agent turn: call LLM → stream → execute tool calls
// → loop until the LLM responds without tool calls.
func (l *Loop) turn(out io.Writer) error {
	for {
		view, err := l.compactor(l.messages, l.summarizer, l.threshold)
		if err != nil {
			return err
		}

		msg, err := l.client.StreamTo(view, l.registry.Schemas(), out)
		if err != nil {
			return err
		}
		l.messages = append(l.messages, msg)

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, tc := range msg.ToolCalls {
			result := l.executeToolCall(tc)
			l.messages = append(l.messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
			fmt.Fprintf(out, "[tool %s → %s]\n", tc.Function.Name, truncate(result, 200))
		}
	}
}

// executeToolCall looks up the tool by name and runs it. Per the design,
// tool errors are returned as result strings — they never break the loop.
func (l *Loop) executeToolCall(tc ToolCall) string {
	tool, ok := l.registry.Get(tc.Function.Name)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
	}
	result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
	if err != nil {
		return fmt.Sprintf("Error: %s: %v", tc.Function.Name, err)
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
