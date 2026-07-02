package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

func fakeStreamServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestStream_TextOnly(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":", world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if msg.Content != "Hello, world" {
		t.Errorf("Content = %q, want %q", msg.Content, "Hello, world")
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(msg.ToolCalls))
	}
}

func TestStream_ToolCallFragments(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"te"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"xt\":\"hi\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "call echo"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("tool call id = %q, want call_1", tc.ID)
	}
	if tc.Function.Name != "echo" {
		t.Errorf("function name = %q, want echo", tc.Function.Name)
	}
	wantArgs := `{"text":"hi"}`
	if tc.Function.Arguments != wantArgs {
		t.Errorf("arguments = %q, want %q", tc.Function.Arguments, wantArgs)
	}
}

func TestStream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
}

// TestStream_EmptyStream verifies the client handles a stream that sends
// only [DONE] with no content chunks — must return an empty assistant
// message, not error.
func TestStream_EmptyStream(t *testing.T) {
	srv := fakeStreamServer(t, nil) // no chunks, just [DONE]
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if msg.Content != "" {
		t.Errorf("Content = %q, want empty", msg.Content)
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(msg.ToolCalls))
	}
	if msg.Role != agent.RoleAssistant {
		t.Errorf("Role = %q, want assistant", msg.Role)
	}
}

// TestStream_ContentAndToolCallsTogether verifies the client handles a
// response where the assistant emits text AND requests a tool call in the
// same turn (common in real API responses).
func TestStream_ContentAndToolCallsTogether(t *testing.T) {
	chunks := []string{
		// First: text + tool call together.
		`{"choices":[{"delta":{"role":"assistant","content":"Let me check: ","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"x\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if msg.Content != "Let me check: " {
		t.Errorf("Content = %q, want %q", msg.Content, "Let me check: ")
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call alongside content, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "echo" {
		t.Errorf("tool name = %q", msg.ToolCalls[0].Function.Name)
	}
}

// TestStream_MultipleToolCalls verifies correct assembly when the stream
// interleaves fragments for two different tool calls (index 0 and 1).
func TestStream_MultipleToolCalls(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"c2","type":"function","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"one\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"two\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(msg.ToolCalls))
	}
	// Order must match the index order, not arrival order.
	if msg.ToolCalls[0].ID != "c1" || msg.ToolCalls[1].ID != "c2" {
		t.Errorf("tool call IDs = %q, %q; want c1, c2", msg.ToolCalls[0].ID, msg.ToolCalls[1].ID)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"text":"one"}` {
		t.Errorf("call 0 args = %q", msg.ToolCalls[0].Function.Arguments)
	}
	if msg.ToolCalls[1].Function.Arguments != `{"text":"two"}` {
		t.Errorf("call 1 args = %q", msg.ToolCalls[1].Function.Arguments)
	}
}

// TestStream_MalformedSSEChunk verifies the client returns an error (rather
// than panicking or silently dropping) when a chunk is not valid JSON.
func TestStream_MalformedSSEChunk(t *testing.T) {
	srv := fakeStreamServer(t, []string{`{this is not json`})
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error for malformed SSE chunk, got nil")
	}
}

// TestStream_NonDataLinesIgnored verifies the parser skips SSE lines that
// are not `data:` payloads (event:, id:, comments) without erroring.
func TestStream_NonDataLinesIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		// Mix in event/id/comment lines alongside data.
		w.Write([]byte(": this is a comment\n\n"))
		w.Write([]byte("event: message\n"))
		w.Write([]byte("id: 42\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if msg.Content != "hi" {
		t.Errorf("Content = %q, want hi", msg.Content)
	}
}

// TestStreamTo_WritesDeltasToOutput verifies that text deltas are written to
// the provided writer in arrival order (the streaming UX).
func TestStreamTo_WritesDeltasToOutput(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"A"}}]}`,
		`{"choices":[{"delta":{"content":"B"}}]}`,
		`{"choices":[{"delta":{"content":"C"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	var out strings.Builder
	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, err := c.StreamTo([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, &out)
	if err != nil {
		t.Fatalf("StreamTo: %v", err)
	}
	// Output must contain "ABC" (the deltas in order), plus a trailing newline.
	if !strings.HasPrefix(out.String(), "ABC") {
		t.Errorf("streamed output = %q, want prefix ABC", out.String())
	}
}
