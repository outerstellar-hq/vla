package mcp

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
)

// mockServer simulates an MCP server over net.Pipe. It reads JSON-RPC
// requests and responds according to the method.
type mockServer struct {
	conn  net.Conn
	tools []ToolDef
	mu    sync.Mutex
}

func newMockServer(t *testing.T, tools []ToolDef) *mockServer {
	t.Helper()
	return &mockServer{tools: tools}
}

func (s *mockServer) serve(conn net.Conn) {
	s.conn = conn
	scanner := newLineScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		id := msg["id"]

		switch method {
		case "initialize":
			s.send(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
				},
			})
		case "notifications/initialized":
			// No response needed.
		case "tools/list":
			s.send(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{"tools": s.tools},
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]any)
			result := s.handleCall(name, args)
			s.send(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  result,
			})
		}
	}
}

func (s *mockServer) send(msg map[string]any) {
	data, _ := json.Marshal(msg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.Write(append(data, '\n'))
}

func (s *mockServer) handleCall(name string, args map[string]any) map[string]any {
	switch name {
	case "echo":
		text, _ := args["text"].(string)
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		}
	case "add":
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("%v", a+b)},
			},
		}
	case "fail":
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "something went wrong"},
			},
			"isError": true,
		}
	default:
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "unknown tool"},
			},
			"isError": true,
		}
	}
}

func (s *mockServer) close() {
	if s.conn != nil {
		s.conn.Close()
	}
}

// lineScanner reads newline-delimited lines from a net.Conn.
type lineScanner struct {
	conn net.Conn
	buf  []byte
}

func newLineScanner(conn net.Conn) *lineScanner {
	return &lineScanner{conn: conn, buf: make([]byte, 0, 4096)}
}

func (s *lineScanner) Scan() bool {
	return true // blocking read is handled in Bytes/Text below
}

func (s *lineScanner) Bytes() []byte {
	// Read until newline.
	var line []byte
	one := make([]byte, 1)
	for {
		n, err := s.conn.Read(one)
		if err != nil || n == 0 {
			return nil
		}
		if one[0] == '\n' {
			return line
		}
		line = append(line, one[0])
	}
}

func (s *lineScanner) Text() string {
	b := s.Bytes()
	if b == nil {
		return ""
	}
	return string(b)
}

// --- Tests ---

func TestClient_InitializeAndListTools(t *testing.T) {
	tools := []ToolDef{
		{Name: "echo", Description: "Echo back text", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		}},
		{Name: "add", Description: "Add two numbers", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		}},
	}

	srvConn, clientConn := net.Pipe()
	defer srvConn.Close()

	srv := newMockServer(t, tools)
	go srv.serve(srvConn)
	defer srv.close()

	// Create client manually (bypass Start which uses exec).
	c := &Client{
		cmd:    nil,
		stdin:  &pipeWriter{clientConn},
		stdout: &pipeReader{clientConn},
		done:   make(chan struct{}),
	}
	c.Start()

	// Initialize.
	_, err := c.Request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	_ = c.Notify("notifications/initialized", map[string]any{})

	got, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}
	if got[0].Name != "echo" {
		t.Errorf("tool 0 name = %q", got[0].Name)
	}
}

func TestClient_CallTool(t *testing.T) {
	tools := []ToolDef{
		{Name: "echo", InputSchema: map[string]any{"type": "object"}},
		{Name: "add", InputSchema: map[string]any{"type": "object"}},
	}

	srvConn, clientConn := net.Pipe()
	defer srvConn.Close()

	srv := newMockServer(t, tools)
	go srv.serve(srvConn)
	defer srv.close()

	c := &Client{
		stdin:  &pipeWriter{clientConn},
		stdout: &pipeReader{clientConn},
		done:   make(chan struct{}),
	}
	c.Start()

	_, _ = c.Request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	_ = c.Notify("notifications/initialized", map[string]any{})

	// Call echo.
	result, err := c.CallTool("echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Text() != "hello" {
		t.Errorf("got %q, want hello", result.Text())
	}

	// Call add.
	result2, _ := c.CallTool("add", map[string]any{"a": 3, "b": 4})
	if result2.Text() != "7" {
		t.Errorf("got %q, want 7", result2.Text())
	}
}

func TestClient_CallToolError(t *testing.T) {
	tools := []ToolDef{{Name: "fail", InputSchema: map[string]any{"type": "object"}}}

	srvConn, clientConn := net.Pipe()
	defer srvConn.Close()

	srv := newMockServer(t, tools)
	go srv.serve(srvConn)
	defer srv.close()

	c := &Client{
		stdin:  &pipeWriter{clientConn},
		stdout: &pipeReader{clientConn},
		done:   make(chan struct{}),
	}
	c.Start()

	_, _ = c.Request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	_ = c.Notify("notifications/initialized", map[string]any{})

	result, _ := c.CallTool("fail", nil)
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if result.Text() != "something went wrong" {
		t.Errorf("got %q", result.Text())
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(cfg.Servers))
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	dir = dir + "/.vla"
	_ = osMkdirAll(dir)
	_ = osWriteFile(dir+"/mcp.json", []byte(`{
		"servers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {"GITHUB_TOKEN": "ghp_xxx"}
			}
		}
	}`), 0644)

	cfg, err := LoadConfig(dir + "/..")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	srv, ok := cfg.Servers["github"]
	if !ok {
		t.Fatal("missing github server")
	}
	if srv.Command != "npx" {
		t.Errorf("command = %q", srv.Command)
	}
	if len(srv.Args) != 2 {
		t.Errorf("args = %v", srv.Args)
	}
	if srv.Env["GITHUB_TOKEN"] != "ghp_xxx" {
		t.Errorf("env token = %q", srv.Env["GITHUB_TOKEN"])
	}
}

// --- pipe adapters to satisfy io.ReadCloser / io.WriteCloser ---

type pipeReader struct{ conn net.Conn }

func (p *pipeReader) Read(buf []byte) (int, error) { return p.conn.Read(buf) }
func (p *pipeReader) Close() error                 { return nil }

type pipeWriter struct{ conn net.Conn }

func (p *pipeWriter) Write(buf []byte) (int, error) { return p.conn.Write(buf) }
func (p *pipeWriter) Close() error                  { return nil }
