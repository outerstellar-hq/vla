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

// keepRecent mirrors compaction.KeepRecent: the number of most-recent turns
// always preserved verbatim when compacting. Duplicated here to avoid an
// import cycle (compaction imports agent for Message).
const keepRecent = 8

// Summarizer summarizes a slice of messages into a terse string. This
// mirrors compaction.Summarizer; it lives here to avoid the agent↔compaction
// import cycle.
type Summarizer func(msgs []Message) (string, error)

// Loop is the VLA agent loop. It is created once per session and run with
// a user-input reader and an output writer (the terminal).
type Loop struct {
	client     Streamer
	registry   *tools.Registry
	summarizer Summarizer
	threshold  int
	messages   []Message
}

// NewLoop returns a Loop wired to the given client and tool registry.
// threshold is the compaction character threshold. client must satisfy
// Streamer (e.g. *llm.Client).
func NewLoop(client Streamer, registry *tools.Registry, threshold int) *Loop {
	return &Loop{
		client:     client,
		registry:   registry,
		threshold:  threshold,
		summarizer: defaultSummarizer(client),
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
		view, err := compact(l.messages, l.summarizer, l.threshold)
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

// compact is a local replica of compaction.Compact. It exists here to avoid
// an import cycle: compaction imports agent (for Message), so agent cannot
// import compaction back. The logic is identical and kept in sync.
//
// If the total character count is below threshold, or there are too few
// messages to summarize, the input is returned unchanged. Otherwise the
// oldest turns (all but the most recent keepRecent) are replaced by a single
// system message produced by sum.
func compact(msgs []Message, sum Summarizer, threshold int) ([]Message, error) {
	if totalChars(msgs) < threshold {
		return msgs, nil
	}
	if len(msgs) <= keepRecent {
		return msgs, nil
	}

	split := len(msgs) - keepRecent
	old := msgs[:split]
	recent := msgs[split:]

	summary, err := sum(old)
	if err != nil {
		return nil, fmt.Errorf("agent: summarize: %w", err)
	}
	out := make([]Message, 0, 1+len(recent))
	out = append(out, Message{
		Role:    RoleSystem,
		Content: "Summary of earlier conversation:\n\n" + summary,
	})
	out = append(out, recent...)
	return out, nil
}

func totalChars(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}

// defaultSummarizer returns a Summarizer that calls the LLM to produce a
// terse summary of older turns. Used in production; tests don't trigger
// compaction (threshold is set to 1_000_000 in tests).
func defaultSummarizer(c Streamer) Summarizer {
	return func(msgs []Message) (string, error) {
		var b strings.Builder
		for _, m := range msgs {
			fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
		}
		summaryReq := []Message{{
			Role: RoleUser,
			Content: "Summarize the following conversation turns. Preserve: file paths mentioned, " +
				"decisions made, errors encountered, and any incomplete tasks. Be terse.\n\n" + b.String(),
		}}
		resp, err := c.StreamTo(summaryReq, nil, io.Discard)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}
