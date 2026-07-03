package mcp

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/tools"
)

// startMockServer starts a mock MCP server on net.Pipe and returns a Client
// wired to it (post-handshake, with tools loaded). The caller gets the server
// side to close when done.
func startMockServer(t *testing.T, toolDefs []ToolDef) (*Client, *mockServer, net.Conn) {
	t.Helper()
	tools_list := toolDefs
	if tools_list == nil {
		tools_list = []ToolDef{
			{Name: "echo", Description: "Echo text", InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
				"required": []string{"text"},
			}},
			{Name: "add", Description: "Add numbers", InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "number"},
					"b": map[string]any{"type": "number"},
				},
			}},
			{Name: "fail", Description: "Always fails", InputSchema: map[string]any{"type": "object"}},
		}
	}

	srvConn, clientConn := net.Pipe()
	srv := newMockServer(t, tools_list)
	go srv.serve(srvConn)

	c := &Client{
		stdin:  &pipeWriter{clientConn},
		stdout: &pipeReader{clientConn},
		done:   make(chan struct{}),
	}
	c.Start()

	// Handshake.
	_, err := c.Request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_ = c.Notify("notifications/initialized", map[string]any{})
	_, _ = c.ListTools()

	return c, srv, srvConn
}

func TestMCPTool_Name(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	tool := MCPTool{ServerName: "github", Def: ToolDef{Name: "create_issue"}, Client: c}
	if tool.Name() != "github__create_issue" {
		t.Errorf("got %q", tool.Name())
	}
}

func TestMCPTool_Schema(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	schema := map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"},
	}}
	tool := MCPTool{ServerName: "gh", Def: ToolDef{Name: "x", InputSchema: schema}, Client: c}
	got := tool.Schema()
	if got["type"] != "object" {
		t.Errorf("schema type = %v", got["type"])
	}
}

func TestMCPTool_Schema_DefaultWhenMissing(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	tool := MCPTool{ServerName: "gh", Def: ToolDef{Name: "x"}, Client: c}
	got := tool.Schema()
	if got["type"] != "object" {
		t.Errorf("expected default object schema, got %v", got)
	}
}

func TestMCPTool_Execute_Success(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	tool := MCPTool{ServerName: "mock", Def: ToolDef{Name: "echo"}, Client: c}
	result, err := tool.Execute(json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello" {
		t.Errorf("got %q, want hello", result)
	}
}

func TestMCPTool_Execute_MalformedArgs(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	tool := MCPTool{ServerName: "mock", Def: ToolDef{Name: "echo"}, Client: c}
	result, err := tool.Execute(json.RawMessage(`{bad json`))
	if err != nil {
		t.Fatalf("Execute should return error as content, not Go error: %v", err)
	}
	if !strings.HasPrefix(result, "Error:") {
		t.Errorf("expected error string, got %q", result)
	}
}

func TestMCPTool_Execute_IsErrorResult(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	tool := MCPTool{ServerName: "mock", Def: ToolDef{Name: "fail"}, Client: c}
	result, err := tool.Execute(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should return error as content, not Go error: %v", err)
	}
	if !strings.HasPrefix(result, "Error:") {
		t.Errorf("expected error prefix, got %q", result)
	}
}

func TestRegisterAll(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	mgr := NewManager()
	mgr.clients["mock"] = c

	reg := tools.NewRegistry()
	if err := RegisterAll(reg, mgr); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	// All 3 mock tools should be registered with the "mock__" prefix.
	for _, name := range []string{"mock__echo", "mock__add", "mock__fail"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected tool %q registered", name)
		}
	}

	schemas := reg.Schemas()
	if len(schemas) != 3 {
		t.Errorf("expected 3 schemas, got %d", len(schemas))
	}
}

func TestRegisterAll_EmptyManager(t *testing.T) {
	mgr := NewManager()
	reg := tools.NewRegistry()
	if err := RegisterAll(reg, mgr); err != nil {
		t.Fatalf("RegisterAll with empty manager: %v", err)
	}
	if len(reg.Schemas()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(reg.Schemas()))
	}
}

func TestClient_Tools(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	got := c.Tools()
	if len(got) != 3 {
		t.Errorf("expected 3 tools, got %d", len(got))
	}
	// Verify it's a copy (modifying the returned slice doesn't affect internal state).
	got[0].Name = "mutated"
	again := c.Tools()
	if again[0].Name == "mutated" {
		t.Error("Tools() returned a reference, not a copy")
	}
}

func TestClient_Close(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()

	err := c.Close()
	if err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestManager_AllTools(t *testing.T) {
	c, srv, conn := startMockServer(t, nil)
	defer conn.Close()
	defer srv.close()
	defer c.Close()

	mgr := NewManager()
	mgr.clients["server1"] = c

	all := mgr.AllTools()
	if len(all) != 3 {
		t.Errorf("expected 3 tools, got %d", len(all))
	}
	for _, st := range all {
		if st.ServerName != "server1" {
			t.Errorf("server name = %q", st.ServerName)
		}
		if st.Client == nil {
			t.Error("client is nil")
		}
	}
}

func TestManager_Close(t *testing.T) {
	mgr := NewManager()
	mgr.Close() // should not panic even with empty clients
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	dir = dir + "/.vla"
	_ = osMkdirAll(dir)
	_ = osWriteFile(dir+"/mcp.json", []byte(`{not valid json`), 0644)

	_, err := LoadConfig(dir + "/..")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadConfig_EmptyServers(t *testing.T) {
	dir := t.TempDir()
	dir = dir + "/.vla"
	_ = osMkdirAll(dir)
	_ = osWriteFile(dir+"/mcp.json", []byte(`{}`), 0644)

	cfg, err := LoadConfig(dir + "/..")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Servers == nil {
		t.Error("Servers should be initialized to empty map, not nil")
	}
}

// TestCallResult_Text verifies the text extraction from a call result.
func TestCallResult_Text(t *testing.T) {
	r := CallResult{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "line 1"},
			{Type: "text", Text: "line 2"},
			{Type: "image", Text: "ignored"},
		},
	}
	if r.Text() != "line 1\nline 2" {
		t.Errorf("got %q", r.Text())
	}
}

func TestCallResult_Text_Empty(t *testing.T) {
	r := CallResult{}
	if r.Text() != "" {
		t.Errorf("expected empty, got %q", r.Text())
	}
}
