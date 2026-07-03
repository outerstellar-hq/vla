// Package integration contains end-to-end integration tests that exercise
// multiple subsystems together. These tests use a mock LLM server (no API
// key needed) but real filesystem operations, real tools, and real session
// persistence — everything wired exactly as in production except the LLM.
package integration

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/fsutil"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

// streamingServer simulates the OpenAI SSE streaming endpoint. It accepts a
// function that maps call number → SSE chunks, so different responses can be
// returned for successive calls (e.g. first call = tool request, second =
// final answer).
func streamingServer(handler func(callNum int) []string) *httptest.Server {
	callNum := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, chunk := range handler(callNum) {
			w.Write([]byte("data: " + chunk + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
}

// setupProject creates a temp project dir with a small codebase and returns
// a fully wired tool registry + session for that project.
func setupProject(t *testing.T) (dir string, reg *tools.Registry, sess *session.Session) {
	t.Helper()
	dir = t.TempDir()

	// Create a small codebase.
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("def greet(name):\n    return f'Hello, {name}!'\n\ngreet('World')\n"), 0644)
	os.WriteFile(filepath.Join(dir, "utils.py"), []byte("def helper():\n    pass\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "tests"), 0755)
	os.WriteFile(filepath.Join(dir, "tests", "test_main.py"), []byte("from main import greet\n"), 0644)

	// Create a session in this dir.
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	s, err := session.New(session.WithDir(dir), session.WithModel("test-model"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	reg = tools.NewRegistry()
	ctx := builtin.Ctx{BaseDir: dir}
	for _, tool := range []tools.Tool{
		builtin.ReadFile{Ctx: ctx},
		builtin.WriteFile{Ctx: ctx},
		builtin.UpdateFile{Ctx: ctx},
		builtin.DeleteFile{Ctx: ctx},
		builtin.ListFiles{Ctx: ctx},
		builtin.Search{Ctx: ctx},
	} {
		if err := reg.Register(tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}

	return dir, reg, s
}

// noOpSummarizer is a stub that never actually summarizes (compaction
// disabled in these tests by using a huge threshold).
func noOpSummarizer(msgs []agent.Message) (string, error) {
	return "summary", nil
}

// identityCompactor returns messages unchanged.
func identityCompactor(msgs []agent.Message, _ agent.Summarizer, _ int) ([]agent.Message, error) {
	return msgs, nil
}

// TestE2E_ReadFileViaToolCall verifies the full loop: user asks to read a
// file → LLM calls read_file → tool reads from disk → result fed back →
// LLM responds with final answer.
func TestE2E_ReadFileViaToolCall(t *testing.T) {
	_, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			// LLM requests read_file.
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"main.py\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		// Second call: LLM reports what it read.
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"The file defines a greet function."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	err := loop.Run(strings.NewReader("Read main.py and tell me what it does\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The LLM should have read the file and reported its contents.
	if !strings.Contains(output.String(), "greet function") {
		t.Errorf("expected final answer mentioning greet function, got:\n%s", output.String())
	}
	// The tool call should have executed successfully.
	if !strings.Contains(output.String(), "[tool read_file") {
		t.Errorf("expected tool call indicator in output:\n%s", output.String())
	}

	// The transcript should have been persisted.
	turns, _, err := sess.Read()
	if err != nil {
		t.Fatalf("session.Read: %v", err)
	}
	if len(turns) < 3 {
		t.Errorf("expected >=3 turns persisted (user+assistant+tool), got %d", len(turns))
	}
}

// TestE2E_WriteFileViaToolCall verifies the LLM can create a new file through
// the write_file tool, and that file actually exists on disk.
func TestE2E_WriteFileViaToolCall(t *testing.T) {
	dir, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"new_module.py\",\"content\":\"def new_func():\\n    return 42\\n\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"Created new_module.py"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("Create a new file called new_module.py\n\n"), &output)

	// File should exist on disk.
	data, err := os.ReadFile(filepath.Join(dir, "new_module.py"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "new_func") {
		t.Errorf("file content unexpected: %q", data)
	}
}

// TestE2E_UpdateFileViaToolCall verifies the LLM can modify an existing file
// via update_file, and the change persists on disk.
func TestE2E_UpdateFileViaToolCall(t *testing.T) {
	dir, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"update_file","arguments":"{\"path\":\"main.py\",\"old_string\":\"Hello, {name}!\",\"new_string\":\"Goodbye, {name}!\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"Updated the greeting."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("Change the greeting in main.py\n\n"), &output)

	// Verify the file was updated on disk.
	data, _ := os.ReadFile(filepath.Join(dir, "main.py"))
	if !strings.Contains(string(data), "Goodbye") {
		t.Errorf("file not updated, still contains old text: %q", data)
	}
	if strings.Contains(string(data), "Hello, {name}!") {
		t.Errorf("old text should be gone: %q", data)
	}
}

// TestE2E_SearchViaToolCall verifies the search tool returns real results
// from the temp project's files.
func TestE2E_SearchViaToolCall(t *testing.T) {
	_, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"search","arguments":"{\"pattern\":\"greet\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"Found the greet function."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("Search for greet\n\n"), &output)

	// The search result should contain main.py (where greet is defined).
	if !strings.Contains(output.String(), "main.py") {
		t.Errorf("search result missing main.py:\n%s", output.String())
	}
}

// TestE2E_PathConfinementEnforced verifies that a tool call with an escape
// path (../../etc/passwd) is blocked by fsutil.Confine — even through the
// full agent loop.
func TestE2E_PathConfinementEnforced(t *testing.T) {
	_, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"../../../../etc/passwd\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"OK"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("Read /etc/passwd\n\n"), &output)

	// The tool result should contain an escape error, not file contents.
	if !strings.Contains(output.String(), "escapes") {
		t.Errorf("expected path escape error, got:\n%s", output.String())
	}
}

// TestE2E_MultiToolCall verifies that when the LLM requests multiple tools in
// one response (e.g. read two files), both execute before the next LLM call.
func TestE2E_MultiToolCall(t *testing.T) {
	_, reg, sess := setupProject(t)

	var lastRequestBody string
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		lastRequestBody = string(body)

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callNum == 1 {
			// Two tool calls in one response.
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"main.py\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"c2","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"utils.py\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"read both files"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("Read both files\n\n"), &output)

	// The second LLM call should have BOTH tool results in the request.
	if !strings.Contains(lastRequestBody, "greet") {
		t.Error("first file result (main.py with greet) missing from second request")
	}
	if !strings.Contains(lastRequestBody, "helper") {
		t.Error("second file result (utils.py with helper) missing from second request")
	}
}

// TestE2E_TranscriptPersistence verifies that a full conversation is written
// to the NDJSON transcript file and can be read back losslessly.
func TestE2E_TranscriptPersistence(t *testing.T) {
	_, reg, sess := setupProject(t)

	srv := streamingServer(func(callNum int) []string {
		if callNum == 1 {
			return []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"list_files","arguments":"{}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		}
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"I see the project structure."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("List the files\n\n"), &output)

	// Read the transcript back.
	turns, meta, err := sess.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Expect: user message + assistant (tool_calls) + tool result + assistant (final) = 4.
	if len(turns) != 4 {
		t.Errorf("expected 4 turns, got %d", len(turns))
	}
	// Verify metadata.
	if meta["model"] != "test-model" {
		t.Errorf("meta model = %v", meta["model"])
	}
	// Verify the user message round-tripped.
	if turns[0]["role"] != "user" || !strings.Contains(turns[0]["content"].(string), "List") {
		t.Errorf("turn 0 unexpected: %v", turns[0])
	}
	// Verify tool calls round-tripped.
	assistantTurn := turns[1]
	if assistantTurn["role"] != "assistant" {
		t.Errorf("turn 1 role = %v", assistantTurn["role"])
	}
	tcs, ok := assistantTurn["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Errorf("expected 1 tool call in turn 1, got: %v", assistantTurn["tool_calls"])
	}
}

// TestE2E_SessionResumeAndContinue verifies that a session can be saved,
// reopened, and the conversation continues from where it left off.
func TestE2E_SessionResumeAndContinue(t *testing.T) {
	_, reg, sess := setupProject(t)

	// Phase 1: run a conversation.
	srv1 := streamingServer(func(callNum int) []string {
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"First response."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	client1 := llm.NewClient("test", srv1.URL, "test-model")
	loop1 := agent.NewLoop(client1, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop1.SetTranscriptWriter(sess.Append)
	var output1 strings.Builder
	_ = loop1.Run(strings.NewReader("Hello\n\n"), &output1)
	srv1.Close()

	// Phase 2: reopen the session.
	resumed, err := session.Open(sess.Path())
	if err != nil {
		t.Fatalf("session.Open: %v", err)
	}

	// Load messages from the transcript.
	msgs, err := loadTranscriptMessages(resumed)
	if err != nil {
		t.Fatalf("loadMessages: %v", err)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected >=2 messages from transcript, got %d", len(msgs))
	}

	// Phase 3: continue the conversation with a second LLM call.
	srv2 := streamingServer(func(callNum int) []string {
		return []string{
			`{"choices":[{"delta":{"role":"assistant","content":"Continuing our conversation."}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
	})
	defer srv2.Close()

	client2 := llm.NewClient("test", srv2.URL, "test-model")
	loop2 := agent.NewLoop(client2, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop2.SetTranscriptWriter(resumed.Append)
	loop2.LoadMessages(msgs)

	var output2 strings.Builder
	_ = loop2.Run(strings.NewReader("Continue\n\n"), &output2)

	// The second session's transcript should now have 4+ turns (2 from phase 1 + 2 from phase 2).
	turns, _, _ := resumed.Read()
	if len(turns) < 4 {
		t.Errorf("expected >=4 turns after resume, got %d", len(turns))
	}
}

// TestE2E_MaxTurnsProtection verifies the loop aborts after MaxTurns
// iterations of tool calls.
func TestE2E_MaxTurnsProtection(t *testing.T) {
	_, reg, sess := setupProject(t)

	// Register echo tool so the loop has something to call.
	_ = reg.Register(builtin.Echo{})

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		// Every response requests echo — infinite loop.
		w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"loop\"}"}}]}}]}` + "\n\n"))
		w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("test", srv.URL, "test-model")
	loop := agent.NewLoop(client, reg, identityCompactor, noOpSummarizer, 1_000_000)
	loop.SetTranscriptWriter(sess.Append)

	var output strings.Builder
	err := loop.Run(strings.NewReader("Echo forever\n\n"), &output)
	if err != nil {
		t.Fatalf("Run should not error: %v", err)
	}
	if callCount != agent.MaxTurns {
		t.Errorf("expected %d LLM calls, got %d", agent.MaxTurns, callCount)
	}
	if !strings.Contains(output.String(), "max") {
		t.Errorf("expected max-turns message in output")
	}
}

// --- helpers ---

// loadTranscriptMessages converts session transcript turns back to agent messages.
func loadTranscriptMessages(sess *session.Session) ([]agent.Message, error) {
	turns, _, err := sess.Read()
	if err != nil {
		return nil, err
	}
	var msgs []agent.Message
	for _, t := range turns {
		roleStr, _ := t["role"].(string)
		if roleStr == "" {
			continue
		}
		msg := agent.Message{Role: agent.Role(roleStr)}
		msg.Content, _ = t["content"].(string)
		msg.ToolCallID, _ = t["tool_call_id"].(string)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// silence unused imports (fsutil is used indirectly via tools)
var _ = fsutil.MaxReadBytes
var _ = time.Second
var _ = compaction.DefaultTokenThreshold
