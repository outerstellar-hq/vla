package agent

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/abrandt/vla/internal/tools"
)

// testTool is a minimal Tool implementation for testing.
type testTool struct {
	name    string
	schema  map[string]any
	execute func(args string) (string, error)
}

func (t *testTool) Name() string           { return t.name }
func (t *testTool) Schema() map[string]any { return t.schema }
func (t *testTool) Execute(args json.RawMessage) (string, error) {
	return t.execute(string(args))
}

func newEchoTool() *testTool {
	return &testTool{
		name:   "echo",
		schema: map[string]any{"type": "object"},
		execute: func(s string) (string, error) {
			return "echoed", nil
		},
	}
}

func newErrorTool() *testTool {
	return &testTool{
		name:   "badtool",
		schema: map[string]any{"type": "object"},
		execute: func(s string) (string, error) {
			return "Error: something went wrong", nil
		},
	}
}

// fakeStreamer is a test Streamer that returns a canned message without
// calling any real LLM API. If msgs is set, it returns them in sequence
// (one per StreamTo call); otherwise it returns msg every time.
type fakeStreamer struct {
	msgs []Message
	msg  Message // used if msgs is empty
	call int
}

func (f *fakeStreamer) StreamTo(messages []Message, toolDefs []map[string]any, out io.Writer) (Message, error) {
	var msg Message
	if len(f.msgs) > 0 {
		if f.call >= len(f.msgs) {
			f.call = len(f.msgs) - 1
		}
		msg = f.msgs[f.call]
		f.call++
	} else {
		msg = f.msg
	}
	if msg.Content != "" && out != nil {
		io.WriteString(out, msg.Content)
	}
	return msg, nil
}

// fakeUsageStreamer wraps fakeStreamer to also satisfy UsageProvider.
type fakeUsageStreamer struct {
	fakeStreamer
	usage Usage
}

func (f *fakeUsageStreamer) UsageSnapshot() Usage {
	return f.usage
}

func registerTool(t *testing.T, reg *tools.Registry, tool tools.Tool) {
	t.Helper()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// noopCompactor returns the messages unchanged (no compaction).
func noopCompactor(msgs []Message, _ Summarizer, _ int) ([]Message, error) {
	return msgs, nil
}

// noopSummarizer returns an empty string.
func noopSummarizer(_ []Message) (string, error) {
	return "", nil
}

func TestLoopEmitsToolEvents(t *testing.T) {
	reg := tools.NewRegistry()
	registerTool(t, reg, newEchoTool())

	events := make(chan Event, 16)
	loop := NewLoop(
		&fakeStreamer{msgs: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "tc1", Type: "function",
				Function: FunctionCall{Name: "echo", Arguments: `{}`},
			}}},
			{Role: RoleAssistant, Content: "done"}, // second call: no tool calls, ends the turn
		}},
		reg, noopCompactor, noopSummarizer, 10000,
	)
	loop.SetEventChan(events)

	if err := loop.turn(io.Discard); err != nil {
		t.Fatalf("turn: %v", err)
	}

	close(events)
	var got []EventType
	for ev := range events {
		got = append(got, ev.Type)
	}

	expectSequence(t, got, []EventType{
		EventTurnStart, EventToolStart, EventToolResult, EventTurnEnd,
	})
}

func TestLoopEmitsUsageEvent(t *testing.T) {
	reg := tools.NewRegistry()
	events := make(chan Event, 16)

	streamer := &fakeUsageStreamer{
		fakeStreamer: fakeStreamer{msg: Message{Role: RoleAssistant, Content: "hi"}},
		usage:        Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}

	loop := NewLoop(streamer, reg, noopCompactor, noopSummarizer, 10000)
	loop.SetEventChan(events)

	if err := loop.turn(io.Discard); err != nil {
		t.Fatalf("turn: %v", err)
	}

	close(events)
	var hasUsage bool
	for ev := range events {
		if ev.Type == EventUsage && ev.Usage != nil {
			hasUsage = true
			if ev.Usage.TotalTokens != 150 {
				t.Errorf("usage tokens = %d, want 150", ev.Usage.TotalTokens)
			}
		}
	}
	if !hasUsage {
		t.Error("expected EventUsage in emitted events")
	}
}

func TestLoopNoPanicsWithoutEventChan(t *testing.T) {
	reg := tools.NewRegistry()
	registerTool(t, reg, newEchoTool())

	loop := NewLoop(
		&fakeStreamer{msgs: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "tc1", Type: "function",
				Function: FunctionCall{Name: "echo", Arguments: `{}`},
			}}},
			{Role: RoleAssistant, Content: "done"},
		}},
		reg, noopCompactor, noopSummarizer, 10000,
	)
	if err := loop.turn(io.Discard); err != nil {
		t.Fatalf("turn: %v", err)
	}
}

func TestLoopToolResultEventHasErrorFlag(t *testing.T) {
	reg := tools.NewRegistry()
	registerTool(t, reg, newErrorTool())

	events := make(chan Event, 16)
	loop := NewLoop(
		&fakeStreamer{msgs: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "tc1", Type: "function",
				Function: FunctionCall{Name: "badtool", Arguments: `{}`},
			}}},
			{Role: RoleAssistant, Content: "done"},
		}},
		reg, noopCompactor, noopSummarizer, 10000,
	)
	loop.SetEventChan(events)

	_ = loop.turn(io.Discard)

	close(events)
	for ev := range events {
		if ev.Type == EventToolResult {
			if !ev.Error {
				t.Error("expected Error=true for error result")
			}
			if !strings.Contains(ev.Result, "Error:") {
				t.Errorf("expected result to contain 'Error:', got %q", ev.Result)
			}
			return
		}
	}
	t.Fatal("no EventToolResult emitted")
}

func TestEventChanNonBlocking(t *testing.T) {
	reg := tools.NewRegistry()
	registerTool(t, reg, newEchoTool())

	events := make(chan Event, 1)
	loop := NewLoop(
		&fakeStreamer{msgs: []Message{
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID: "tc1", Type: "function",
				Function: FunctionCall{Name: "echo", Arguments: `{}`},
			}}},
			{Role: RoleAssistant, Content: "done"},
		}},
		reg, noopCompactor, noopSummarizer, 10000,
	)
	loop.SetEventChan(events)

	done := make(chan struct{})
	go func() {
		_ = loop.turn(io.Discard)
		close(done)
	}()
	select {
	case <-done:
		// Good — turn completed despite full channel.
	case <-time.After(5 * time.Second):
		t.Fatal("turn blocked on full event channel")
	}
}

func expectSequence(t *testing.T, got, want []EventType) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("event sequence length: got %d, want %d\n  got: %v\n  want: %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("event[%d]: got %d, want %d\n  got: %v\n  want: %v", i, got[i], want[i], got, want)
			return
		}
	}
}
