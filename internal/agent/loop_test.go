package agent_test

import (
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

func TestLoop_TextOnly_Terminates(t *testing.T) {
	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"hi there"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := agent.NewLoop(client, reg, 1_000_000)

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
	loop := agent.NewLoop(client, reg, 1_000_000)

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
	loop := agent.NewLoop(client, reg, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("call echo badly\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v (tool errors should not propagate as Go errors)", err)
	}
	if !strings.Contains(output.String(), "sorry, try again") {
		t.Errorf("expected LLM recovery in output, got %q", output.String())
	}
}
