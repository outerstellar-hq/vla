package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

func TestStreamTo_SendsImageContentParts(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"I see an image"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, "gpt-4o")
	msg := agent.Message{
		Role: agent.RoleUser,
		ContentParts: []agent.ContentPart{
			{Type: "text", Text: "What's in this image?"},
			{Type: "image_url", ImageURL: &struct {
				URL string `json:"url"`
			}{URL: "data:image/png;base64,iVBORw0KGgo="}},
		},
	}
	_, err := c.StreamTo([]agent.Message{msg}, nil, nil)
	if err != nil {
		t.Fatalf("StreamTo: %v", err)
	}

	// Verify the request body has content as an array, not a string.
	var req map[string]any
	if err := json.Unmarshal([]byte(capturedBody), &req); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	messages := req["messages"].([]any)
	first := messages[0].(map[string]any)
	content := first["content"]

	contentArr, ok := content.([]any)
	if !ok {
		t.Fatalf("expected content to be an array for multimodal, got %T", content)
	}
	if len(contentArr) != 2 {
		t.Errorf("expected 2 content parts, got %d", len(contentArr))
	}
	// Verify the text part.
	textPart := contentArr[0].(map[string]any)
	if textPart["type"] != "text" || textPart["text"] != "What's in this image?" {
		t.Errorf("text part wrong: %v", textPart)
	}
	// Verify the image part.
	imgPart := contentArr[1].(map[string]any)
	if imgPart["type"] != "image_url" {
		t.Errorf("expected image_url type, got %v", imgPart["type"])
	}
}

func TestStreamTo_PlainContentWhenNoParts(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, "gpt-4o")
	msg := agent.Message{Role: agent.RoleUser, Content: "plain text message"}
	_, _ = c.StreamTo([]agent.Message{msg}, nil, nil)

	var req map[string]any
	json.Unmarshal([]byte(capturedBody), &req)
	messages := req["messages"].([]any)
	first := messages[0].(map[string]any)
	content := first["content"]

	// Should be a string, not an array.
	if _, ok := content.([]any); ok {
		t.Error("content should be a string when no ContentParts")
	}
	if !strings.Contains(content.(string), "plain text") {
		t.Errorf("unexpected content: %v", content)
	}
}

func TestMessage_HasImage(t *testing.T) {
	m := agent.Message{
		ContentParts: []agent.ContentPart{
			{Type: "text", Text: "look"},
			{Type: "image_url", ImageURL: &struct {
				URL string `json:"url"`
			}{URL: "data:..."}},
		},
	}
	if !m.HasImage() {
		t.Error("expected HasImage=true")
	}

	m2 := agent.Message{Content: "just text"}
	if m2.HasImage() {
		t.Error("expected HasImage=false for text-only message")
	}
}
