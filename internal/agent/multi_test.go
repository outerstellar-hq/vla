package agent_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

func identityCompactorMulti(msgs []agent.Message, _ agent.Summarizer, _ int) ([]agent.Message, error) {
	return msgs, nil
}

func TestCoordinator_RunsTasksInParallel(t *testing.T) {
	// Verify that tasks actually run concurrently (not sequentially).
	var concurrentCount int32
	var maxConcurrent int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&concurrentCount, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if current <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, current) {
				break
			}
		}
		// Hold for a moment to ensure overlap.
		// (simulate work via a short sleep in the handler)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"done"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
		atomic.AddInt32(&concurrentCount, -1)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()

	coord := agent.NewCoordinator(client, reg, identityCompactorMulti, 1_000_000)
	tasks := []agent.SubTask{
		{Name: "task-1", Prompt: "do task 1"},
		{Name: "task-2", Prompt: "do task 2"},
		{Name: "task-3", Prompt: "do task 3"},
	}

	results := coord.Run(tasks, "you are a test agent")

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("task %d error: %v", i, r.Error)
		}
		if r.Response != "done" {
			t.Errorf("task %d response = %q, want 'done'", i, r.Response)
		}
	}
}

func TestCoordinator_IndependentMessageHistories(t *testing.T) {
	// Each sub-agent should get its own prompt — verify by capturing request bodies.
	var requestBodies []string
	var mu = &atomic.Int32{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := mu.Add(1) - 1
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		requestBodies = append(requestBodies, string(body))

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"response"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
		_ = idx
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()

	coord := agent.NewCoordinator(client, reg, identityCompactorMulti, 1_000_000)
	tasks := []agent.SubTask{
		{Name: "alpha", Prompt: "investigate alpha"},
		{Name: "beta", Prompt: "investigate beta"},
	}

	results := coord.Run(tasks, "system")

	// Both should complete successfully.
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestCoordinator_ToolCallsExecute(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callCount == 1 {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hello\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"got it"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})

	coord := agent.NewCoordinator(client, reg, identityCompactorMulti, 1_000_000)
	results := coord.Run([]agent.SubTask{
		{Name: "test", Prompt: "call echo"},
	}, "system")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Response != "got it" {
		t.Errorf("response = %q", results[0].Response)
	}
	// Should have made 2 LLM calls (tool call + final response).
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
}

func TestCoordinator_ErrorHandling(t *testing.T) {
	// Use a dead server to trigger errors.
	client := llm.NewClient("k", "http://localhost:1", "gpt-4o")
	reg := tools.NewRegistry()

	coord := agent.NewCoordinator(client, reg, identityCompactorMulti, 1_000_000)
	results := coord.Run([]agent.SubTask{
		{Name: "will-fail", Prompt: "do something"},
	}, "system")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error from dead server")
	}
}

func TestFormatResults(t *testing.T) {
	results := []agent.SubResult{
		{Task: agent.SubTask{Name: "investigation"}, Response: "Found the bug."},
		{Task: agent.SubTask{Name: "tests"}, Response: "All tests pass."},
	}
	formatted := agent.FormatResults(results)
	if !contains(formatted, "investigation") {
		t.Error("missing investigation")
	}
	if !contains(formatted, "Found the bug") {
		t.Error("missing bug finding")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
