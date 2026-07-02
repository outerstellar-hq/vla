// Package compaction implements the VLA context-window compaction strategy.
// When the transcript grows too long, the oldest turns are summarized into
// a single system message. The on-disk transcript is NEVER modified —
// Compact is a pure view transform that returns the messages to send to
// the LLM.
package compaction

import (
	"fmt"

	"github.com/abrandt/vla/internal/agent"
)

// CharThreshold is the rough transcript size (in characters) above which
// compaction kicks in. ~100K chars ≈ 25K tokens, leaving headroom under
// a 32K context window. Tunable.
const CharThreshold = 100_000

// KeepRecent is the number of most-recent turns always preserved verbatim.
// Everything older than this is eligible for summarization.
const KeepRecent = 8

// Summarizer summarizes a slice of messages into a terse string.
// The agent loop supplies a real implementation that calls the LLM.
type Summarizer func(msgs []agent.Message) (string, error)

// Compact returns the message list to send to the LLM. If the total
// character count is below threshold, or there are too few messages
// to summarize, the input is returned unchanged. Otherwise the oldest
// turns (all but the most recent KeepRecent) are replaced by a single
// system message produced by sum.
func Compact(msgs []agent.Message, sum Summarizer, threshold int) ([]agent.Message, error) {
	if totalChars(msgs) < threshold {
		return msgs, nil
	}
	if len(msgs) <= KeepRecent {
		return msgs, nil
	}

	split := len(msgs) - KeepRecent
	old := msgs[:split]
	recent := msgs[split:]

	summary, err := sum(old)
	if err != nil {
		return nil, fmt.Errorf("compaction: summarize: %w", err)
	}
	out := make([]agent.Message, 0, 1+len(recent))
	out = append(out, agent.Message{
		Role:    agent.RoleSystem,
		Content: "Summary of earlier conversation:\n\n" + summary,
	})
	out = append(out, recent...)
	return out, nil
}

func totalChars(msgs []agent.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}
