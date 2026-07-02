package compaction

import (
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

func TestCompact_BelowThreshold_Noop(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "short"},
		{Role: agent.RoleAssistant, Content: "reply"},
	}
	out, err := Compact(msgs, stubSummarizer, 1_000_000)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(out) != len(msgs) {
		t.Errorf("expected noop, got %d messages (want %d)", len(out), len(msgs))
	}
}

func TestCompact_AboveThreshold_Summarizes(t *testing.T) {
	var msgs []agent.Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, agent.Message{
			Role:    agent.RoleUser,
			Content: strings.Repeat("x", 1000),
		})
	}
	out, err := Compact(msgs, stubSummarizer, 5000)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(out) != 9 {
		t.Errorf("expected 9 messages after compaction, got %d", len(out))
	}
	if out[0].Role != agent.RoleSystem {
		t.Errorf("expected first message to be system summary, got role %q", out[0].Role)
	}
	if !strings.Contains(out[0].Content, "SUMMARY") {
		t.Errorf("expected summary content, got %q", out[0].Content)
	}
}

func TestCompact_TooFewToCompact_Noop(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: strings.Repeat("x", 10000)},
	}
	out, err := Compact(msgs, stubSummarizer, 100)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 message (too few to compact), got %d", len(out))
	}
}

func TestCompact_PreservesRecentOrder(t *testing.T) {
	var msgs []agent.Message
	for i := 0; i < 12; i++ {
		msgs = append(msgs, agent.Message{Role: agent.RoleUser, Content: strings.Repeat("x", 1000)})
	}
	out, _ := Compact(msgs, stubSummarizer, 5000)
	if len(out) != 9 {
		t.Fatalf("expected 9 messages, got %d", len(out))
	}
	for i := 0; i < 8; i++ {
		original := msgs[len(msgs)-8+i]
		got := out[1+i]
		if got.Content != original.Content {
			t.Errorf("recent message %d changed: %q vs %q", i, got.Content, original.Content)
		}
	}
}

func stubSummarizer(msgs []agent.Message) (string, error) {
	return "SUMMARY of " + itoa(len(msgs)) + " messages", nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
