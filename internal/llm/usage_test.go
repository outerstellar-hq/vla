package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

func TestStream_CapturesUsage(t *testing.T) {
	// The final chunk includes usage when stream_options.include_usage=true.
	chunks := []string{
		`{"choices":[{"delta":{"role":"assistant","content":"hi"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`,
	}
	srv := usageStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	usage := c.TotalUsage()
	if usage.PromptTokens != 100 {
		t.Errorf("prompt tokens = %d, want 100", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("completion tokens = %d, want 50", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("total tokens = %d, want 150", usage.TotalTokens)
	}
}

func TestStream_AccumulatesUsage(t *testing.T) {
	// Two calls should accumulate usage.
	chunks := []string{
		`{"choices":[{"delta":{"content":"x"}}]}`,
		`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}
	srv := usageStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, _ = c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "a"}}, nil)
	_, _ = c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "b"}}, nil)

	usage := c.TotalUsage()
	if usage.PromptTokens != 20 {
		t.Errorf("accumulated prompt = %d, want 20", usage.PromptTokens)
	}
	if usage.TotalTokens != 30 {
		t.Errorf("accumulated total = %d, want 30", usage.TotalTokens)
	}
}

func TestStream_NoUsage(t *testing.T) {
	// If the server doesn't send usage, TotalUsage should be zero.
	chunks := []string{
		`{"choices":[{"delta":{"content":"hi"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := usageStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, _ = c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	usage := c.TotalUsage()
	if usage.TotalTokens != 0 {
		t.Errorf("expected 0 usage, got %d", usage.TotalTokens)
	}
}

func usageStreamServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}
