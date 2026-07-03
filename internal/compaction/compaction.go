// Package compaction implements the VLA context-window compaction strategy.
// When the transcript grows too long, the oldest turns are summarized into
// a single system message. The on-disk transcript is NEVER modified —
// Compact is a pure view transform that returns the messages to send to
// the LLM.
//
// The threshold is expressed in TOKENS (estimated at ~4 chars/token). The
// caller passes the model's context window (from models.dev) and Compact
// triggers at 75% of it, leaving room for the response.
//
// Strategy (v2 — smarter than just "summarize everything old"):
//  1. If total tokens < threshold: return unchanged.
//  2. Split into [old, recent] where recent = last KeepRecent messages.
//  3. Summarize old messages as before.
//  4. Additionally: if any individual tool result in the recent window is
//     larger than MaxToolResultTokens, replace its content with a truncated
//     version + a "use read_file to see full output" hint. This prevents
//     one massive file read from consuming the entire context window.
package compaction

import (
	"fmt"

	"github.com/abrandt/vla/internal/agent"
)

// DefaultTokenThreshold is used when the caller doesn't know the model's
// context window. ~25K tokens = safe for 32K context models.
const DefaultTokenThreshold = 25_000

// KeepRecent is the number of most-recent messages always preserved verbatim
// (not summarized). These are the messages the LLM needs for immediate context.
const KeepRecent = 8

// MaxToolResultTokens is the per-result cap. If a single tool result exceeds
// this, it's truncated even in the "recent" window — no single file read
// should consume half the context.
const MaxToolResultTokens = 4_000 // ~16K chars

// CharsPerToken is the heuristic ratio for estimating tokens from chars.
const CharsPerToken = 4

// Compact returns the message list to send to the LLM. If the total estimated
// token count is below tokenThreshold, the input is returned unchanged.
// Otherwise the oldest messages (all but the most recent KeepRecent) are
// replaced by a single system message, and oversized tool results in the
// recent window are truncated.
func Compact(msgs []agent.Message, sum agent.Summarizer, tokenThreshold int) ([]agent.Message, error) {
	if tokenThreshold <= 0 {
		tokenThreshold = DefaultTokenThreshold
	}

	totalTokens := estimateTokens(msgs)
	if totalTokens < tokenThreshold {
		return msgs, nil
	}
	if len(msgs) <= KeepRecent {
		return msgs, nil
	}

	split := len(msgs) - KeepRecent
	old := msgs[:split]
	recent := msgs[split:]

	// Summarize the old messages.
	summary, err := sum(old)
	if err != nil {
		return nil, fmt.Errorf("compaction: summarize: %w", err)
	}

	// Build the compacted view: summary + truncated recent messages.
	out := make([]agent.Message, 0, 1+len(recent))
	out = append(out, agent.Message{
		Role:    agent.RoleSystem,
		Content: "Summary of earlier conversation:\n\n" + summary,
	})
	for _, m := range recent {
		// Truncate oversized tool results to prevent context bloat.
		if m.Role == agent.RoleTool && estimateTokensMsg(m) > MaxToolResultTokens {
			m.Content = truncateWithHint(m.Content, MaxToolResultTokens*CharsPerToken)
		}
		out = append(out, m)
	}
	return out, nil
}

// estimateTokens estimates the total token count for a message list.
func estimateTokens(msgs []agent.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokensMsg(m)
	}
	return total
}

// estimateTokensMsg estimates tokens for a single message (~4 chars/token).
func estimateTokensMsg(m agent.Message) int {
	chars := len(m.Content)
	for _, tc := range m.ToolCalls {
		chars += len(tc.Function.Arguments)
	}
	return chars / CharsPerToken
}

// truncateWithHint shortens content to maxChars and appends a notice that
// the full output was truncated (the LLM can use read_file to see it again).
func truncateWithHint(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n\n[...output truncated — use read_file to see full content]"
}

// TokensToChars converts a token count to an approximate character count.
// Used by main.go to convert the model's context limit (tokens) to the
// threshold that Compact expects (also tokens now).
func TokensToChars(tokens int) int {
	return tokens * CharsPerToken
}
