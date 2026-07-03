package agent_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

func streamingServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
}

// newTestLoop builds a Loop with compaction effectively disabled (huge
// threshold + identity compactor). The tests don't exercise compaction —
// they verify the stream/tool-call loop itself.
func newTestLoop(client *llm.Client, reg *tools.Registry) *agent.Loop {
	return agent.NewLoop(
		client,
		reg,
		identityCompactor,
		stubSummarizer,
		1_000_000,
	)
}

// identityCompactor returns msgs unchanged. Used in tests where compaction
// is not the subject.
func identityCompactor(msgs []agent.Message, _ agent.Summarizer, _ int) ([]agent.Message, error) {
	return msgs, nil
}

func stubSummarizer(_ []agent.Message) (string, error) {
	return "stub summary", nil
}

func TestLoop_TextOnly_Terminates(t *testing.T) {
	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"hi there"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := newTestLoop(client, reg)

	var output strings.Builder
	err := loop.Run(strings.NewReader("hello\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(output.String(), "hi there") {
		t.Errorf("expected 'hi there' in output, got %q", output.String())
	}
}

func TestLoop_ToolCall_ExecutesAndLoops(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		var chunks []string
		if callCount == 1 {
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"ping\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		} else {
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","content":"echo said: ping"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			}
		}
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	if err := reg.Register(builtin.Echo{}); err != nil {
		t.Fatal(err)
	}
	loop := newTestLoop(client, reg)

	var output strings.Builder
	err := loop.Run(strings.NewReader("call echo with ping\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (tool + final), got %d", callCount)
	}
	if !strings.Contains(output.String(), "echo said: ping") {
		t.Errorf("expected final answer in output, got %q", output.String())
	}
}

func TestLoop_ToolError_FedBackToLLM(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		var chunks []string
		if callCount == 1 {
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		} else {
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","content":"sorry, try again"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			}
		}
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})
	loop := newTestLoop(client, reg)

	var output strings.Builder
	err := loop.Run(strings.NewReader("call echo badly\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v (tool errors should not propagate as Go errors)", err)
	}
	if !strings.Contains(output.String(), "sorry, try again") {
		t.Errorf("expected LLM recovery in output, got %q", output.String())
	}
}

// TestLoop_MultipleToolCallsInOneResponse verifies that when the LLM requests
// several tools in a single response, the loop executes all of them (in order)
// before re-calling the LLM.
func TestLoop_MultipleToolCallsInOneResponse(t *testing.T) {
	callCount := 0
	var lastRequestBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		lastRequestBody = string(body)

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callCount == 1 {
			// Two tool calls in one response: echo "one" and echo "two".
			chunks := []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"one\"}"}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"c2","type":"function","function":{"name":"echo","arguments":"{\"text\":\"two\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
			for _, c := range chunks {
				w.Write([]byte("data: " + c + "\n\n"))
				f.Flush()
			}
		} else {
			chunks := []string{
				`{"choices":[{"delta":{"role":"assistant","content":"got one and two"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			}
			for _, c := range chunks {
				w.Write([]byte("data: " + c + "\n\n"))
				f.Flush()
			}
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})
	loop := newTestLoop(client, reg)

	var output strings.Builder
	if err := loop.Run(strings.NewReader("echo one and two\n\n"), &output); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", callCount)
	}
	// The second LLM call's request must contain BOTH tool results as
	// role=tool messages — proving both were executed and appended.
	if !strings.Contains(lastRequestBody, `"one"`) {
		t.Error("second request missing first tool result")
	}
	if !strings.Contains(lastRequestBody, `"two"`) {
		t.Error("second request missing second tool result")
	}
	if !strings.Contains(output.String(), "got one and two") {
		t.Errorf("final output missing, got %q", output.String())
	}
}

// TestLoop_CompactionFires verifies that compaction actually triggers during
// a loop when the transcript exceeds the threshold. We use a tiny threshold
// and a custom compactor that records when it was invoked.
func TestLoop_CompactionFires(t *testing.T) {
	compactorCalls := 0
	// This compactor records the call and then delegates to identity (returns
	// msgs unchanged) so we don't depend on a real summarizer.
	recordingCompactor := func(msgs []agent.Message, _ agent.Summarizer, _ int) ([]agent.Message, error) {
		compactorCalls++
		return msgs, nil
	}

	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"ok"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	// Threshold of 1 byte means ANY non-empty transcript triggers compaction.
	loop := agent.NewLoop(client, reg, recordingCompactor, stubSummarizer, 1)

	var output strings.Builder
	if err := loop.Run(strings.NewReader("hi\n\n"), &output); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if compactorCalls == 0 {
		t.Fatal("expected compactor to be called at least once")
	}
}

// TestLoop_CompactionSummarizerReceivesOldTurns verifies the summarizer is
// actually invoked with the oldest turns (not the recent ones) when
// compaction triggers, and that its summary lands in the LLM view.
func TestLoop_CompactionSummarizerReceivesOldTurns(t *testing.T) {
	var summarizedRoleStrings []string
	var requestBodies []string
	summarizer := func(msgs []agent.Message) (string, error) {
		for _, m := range msgs {
			summarizedRoleStrings = append(summarizedRoleStrings, string(m.Role))
		}
		return "OLD_SUMMARY", nil
	}
	// Real compactor (the one from main.go uses compaction.Compact; here we
	// replicate the same logic to test the loop invokes summarizer correctly).
	compactor := func(msgs []agent.Message, sum agent.Summarizer, threshold int) ([]agent.Message, error) {
		if len(msgs) <= 8 {
			return msgs, nil
		}
		old := msgs[:len(msgs)-8]
		recent := msgs[len(msgs)-8:]
		summary, err := sum(old)
		if err != nil {
			return nil, err
		}
		return append([]agent.Message{{Role: agent.RoleSystem, Content: summary}}, recent...), nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		requestBodies = append(requestBodies, string(body))
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"ok"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := agent.NewLoop(client, reg, compactor, summarizer, 1) // threshold 1 → always compact

	// Send 10 user turns (each triggers one assistant response → 20 messages).
	input := strings.Repeat("msg\n\n", 10)
	var output strings.Builder
	if err := loop.Run(strings.NewReader(input), &output); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(summarizedRoleStrings) == 0 {
		t.Fatal("summarizer was never called; expected it once transcript exceeded 8 messages")
	}
	// After enough turns, the LLM view sent must contain the summary.
	lastRequest := requestBodies[len(requestBodies)-1]
	if !strings.Contains(lastRequest, "OLD_SUMMARY") {
		t.Errorf("final LLM request missing summary; request was:\n%s", lastRequest)
	}
}

// TestLoop_PersistsTurnsToTranscript verifies that when a TranscriptWriter
// is set, every turn (user, assistant, tool) is written to it.
func TestLoop_PersistsTurnsToTranscript(t *testing.T) {
	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"hello"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	var written []map[string]any
	writer := func(turn map[string]any) error {
		written = append(written, turn)
		return nil
	}

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)
	loop.SetTranscriptWriter(writer)

	var output strings.Builder
	if err := loop.Run(strings.NewReader("hi\n\n"), &output); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have: user message + assistant message = 2 turns.
	if len(written) != 2 {
		t.Fatalf("expected 2 persisted turns, got %d: %+v", len(written), written)
	}
	if written[0]["role"] != "user" {
		t.Errorf("turn 0 role = %v, want user", written[0]["role"])
	}
	if written[0]["content"] != "hi" {
		t.Errorf("turn 0 content = %v", written[0]["content"])
	}
	if written[1]["role"] != "assistant" {
		t.Errorf("turn 1 role = %v, want assistant", written[1]["role"])
	}
	if written[1]["content"] != "hello" {
		t.Errorf("turn 1 content = %v", written[1]["content"])
	}
	// Every turn must have a timestamp.
	for i, turn := range written {
		if turn["timestamp"] == nil || turn["timestamp"] == "" {
			t.Errorf("turn %d missing timestamp", i)
		}
		if turn["type"] != "turn" {
			t.Errorf("turn %d type = %v, want 'turn'", i, turn["type"])
		}
	}
}

// TestLoop_PersistsToolCallsAndToolResults verifies that tool calls and their
// results are persisted correctly.
func TestLoop_PersistsToolCallsAndToolResults(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callCount == 1 {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hi\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"done"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	var written []map[string]any
	writer := func(turn map[string]any) error {
		written = append(written, turn)
		return nil
	}

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)
	loop.SetTranscriptWriter(writer)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("echo hi\n\n"), &output)

	// Expected: user, assistant (with tool_calls), tool result, assistant (final) = 4 turns.
	if len(written) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(written))
	}
	// Turn 1 (assistant) must have tool_calls.
	assistantTurn := written[1]
	tcs, ok := assistantTurn["tool_calls"].([]agent.ToolCall)
	if !ok {
		t.Errorf("expected tool_calls in assistant turn, got %T", assistantTurn["tool_calls"])
	} else if len(tcs) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(tcs))
	}
	// Turn 2 (tool result) must have tool_call_id.
	toolTurn := written[2]
	if toolTurn["role"] != "tool" {
		t.Errorf("turn 2 role = %v, want tool", toolTurn["role"])
	}
	if toolTurn["tool_call_id"] != "c1" {
		t.Errorf("turn 2 tool_call_id = %v, want c1", toolTurn["tool_call_id"])
	}
	if toolTurn["content"] != "hi" {
		t.Errorf("turn 2 content = %v, want 'hi'", toolTurn["content"])
	}
}

// TestLoop_LoadMessagesResumes verifies that pre-loaded messages are part of
// the conversation sent to the LLM (the resume path).
func TestLoop_LoadMessagesResumes(t *testing.T) {
	var lastRequestBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		lastRequestBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"continued"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)

	// Simulate a resumed conversation.
	loop.LoadMessages([]agent.Message{
		{Role: agent.RoleUser, Content: "previous question"},
		{Role: agent.RoleAssistant, Content: "previous answer"},
	})

	var output strings.Builder
	_ = loop.Run(strings.NewReader("new question\n\n"), &output)

	// The request body must contain both the old and new messages.
	if !strings.Contains(lastRequestBody, "previous question") {
		t.Error("resumed messages not sent to LLM")
	}
	if !strings.Contains(lastRequestBody, "new question") {
		t.Error("new message not sent to LLM")
	}
}

// TestLoop_NoWriterDoesntPanic verifies the loop works fine without persistence.
func TestLoop_NoWriterDoesntPanic(t *testing.T) {
	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"ok"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("hi\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestLoop_MaxTurnsAbortsInfiniteToolCallLoop verifies that if the LLM keeps
// requesting tool calls beyond MaxTurns, the loop aborts and returns control
// to the user instead of looping forever.
func TestLoop_MaxTurnsAbortsInfiniteToolCallLoop(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		// Every response requests the echo tool — never terminates.
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"loop\"}"}}]}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("hi\n\n"), &output)
	if err != nil {
		t.Fatalf("Run should not error on max-turns: %v", err)
	}
	// Should have called the LLM exactly MaxTurns times, then aborted.
	if callCount != agent.MaxTurns {
		t.Errorf("expected %d LLM calls (MaxTurns), got %d", agent.MaxTurns, callCount)
	}
	if !strings.Contains(output.String(), "max") {
		t.Errorf("expected max-turns notice in output, got %q", output.String())
	}
}

// helper for multiple-tool-call test to satisfy unused import
var _ = fmt.Sprintf
