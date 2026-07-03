package agent_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

// fakeApprover is a test ToolApprover that records what it sees and returns
// a pre-set decision.
type fakeApprover struct {
	approveResult bool
	seen          []string
}

func (f *fakeApprover) RequiresApproval(toolName string) bool {
	return toolName == "write_file" || toolName == "delete_file"
}
func (f *fakeApprover) Approve(toolName string, args map[string]any, preview string) bool {
	f.seen = append(f.seen, toolName)
	return f.approveResult
}

// fakePermCheck blocks specific tools.
type fakePermCheck struct {
	blocked map[string]bool
}

func (p fakePermCheck) IsBlocked(toolName string) bool {
	return p.blocked[toolName]
}

// TestLoop_Approval_Denied verifies that when the approver denies a tool call,
// the tool does NOT execute and the denial message is fed back to the LLM.
func TestLoop_Approval_Denied(t *testing.T) {
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callNum == 1 {
			// LLM asks to write a file.
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"x.txt\",\"content\":\"data\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			// LLM reacts to denial.
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"ok I won't write it"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.WriteFile{Ctx: builtin.Ctx{BaseDir: dir}})

	approver := &fakeApprover{approveResult: false} // deny everything

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)
	loop.SetApprover(approver)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("write a file\n\n"), &output)

	// The approver should have been asked about write_file.
	if len(approver.seen) == 0 {
		t.Fatal("approver was never called")
	}
	if approver.seen[0] != "write_file" {
		t.Errorf("expected write_file, got %q", approver.seen[0])
	}

	// The file should NOT exist (denied).
	if fileExists(dir + "/x.txt") {
		t.Error("file was created despite denial")
	}

	// The output should show the denial was fed to the LLM.
	if !strings.Contains(output.String(), "denied") {
		t.Errorf("expected denial message in output:\n%s", output.String())
	}
}

// TestLoop_Approval_Approved verifies that when the approver allows the tool,
// it executes normally.
func TestLoop_Approval_Approved(t *testing.T) {
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callNum == 1 {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"y.txt\",\"content\":\"hello\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"file written"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.WriteFile{Ctx: builtin.Ctx{BaseDir: dir}})

	approver := &fakeApprover{approveResult: true} // approve everything

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)
	loop.SetApprover(approver)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("write a file\n\n"), &output)

	// The file SHOULD exist (approved).
	if !fileExists(dir + "/y.txt") {
		t.Error("file was NOT created despite approval")
	}
}

// TestLoop_PermissionBlocked verifies that a tool blocked by permissions
// never executes and never reaches the approver.
func TestLoop_PermissionBlocked(t *testing.T) {
	callNum := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if callNum == 1 {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"delete_file","arguments":"{\"path\":\"x.txt\"}"}}]}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			w.Write([]byte(`data: {"choices":[{"delta":{"role":"assistant","content":"understood"}}]}` + "\n\n"))
			w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.DeleteFile{Ctx: builtin.Ctx{BaseDir: dir}})

	approver := &fakeApprover{approveResult: true} // would approve
	permCheck := fakePermCheck{blocked: map[string]bool{"delete_file": true}}

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	loop := agent.NewLoop(client, reg, identityCompactor, stubSummarizer, 1_000_000)
	loop.SetApprover(approver)
	loop.SetPermissionChecker(permCheck)

	var output strings.Builder
	_ = loop.Run(strings.NewReader("delete x.txt\n\n"), &output)

	// The approver should NOT have been called (blocked before reaching it).
	if len(approver.seen) != 0 {
		t.Errorf("approver should not be called for blocked tool, got: %v", approver.seen)
	}
	// The output should show the block message.
	if !strings.Contains(output.String(), "blocked") {
		t.Errorf("expected block message in output:\n%s", output.String())
	}
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(filepath.FromSlash(path))
	return err == nil
}
