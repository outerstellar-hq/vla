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
