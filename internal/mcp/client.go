// Package mcp implements an MCP (Model Context Protocol) client for VLA.
// MCP lets external tools plug into the agent via a standard protocol —
// the same protocol used by Claude Code, OpenCode, Cursor, etc.
//
// MCP stdio transport uses newline-delimited JSON-RPC 2.0 (one JSON object
// per line, no Content-Length framing — that's LSP). The client launches
// the server as a subprocess and communicates over its stdin/stdout.
//
// Lifecycle: initialize → initialized → tools/list → tools/call.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client is a JSON-RPC 2.0 client that speaks MCP over stdio (newline-
// delimited JSON, one message per line).
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	writeMu sync.Mutex
	id      atomic.Int64
	pending sync.Map // map[int64]chan *jsonResponse
	tools   []ToolDef
	toolsMu sync.RWMutex
	done    chan struct{}
}

// jsonResponse is the JSON-RPC response envelope.
type jsonResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *jsonError      `json:"error"`
}

type jsonError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *jsonError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// ToolDef is the MCP tool definition (from tools/list).
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// CallResult is the result of tools/call.
type CallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// Text returns the concatenated text content of the call result.
func (r CallResult) Text() string {
	var s string
	for _, c := range r.Content {
		if c.Type == "text" {
			if s != "" {
				s += "\n"
			}
			s += c.Text
		}
	}
	return s
}

// Start launches an MCP server subprocess, performs the initialize handshake,
// and fetches the tool list. Returns the list of tools the server exposes.
func Start(command string, args []string, env []string) (*Client, []ToolDef, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(cmd.Env, env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("mcp: start %s: %w", command, err)
	}

	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		done:   make(chan struct{}),
	}
	c.Start()

	// Drain stderr to prevent pipe deadlock.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if err != nil || n == 0 {
				return
			}
		}
	}()

	// Initialize handshake.
	_, err = c.Request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "vla",
			"version": "dev",
		},
	})
	if err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("mcp: initialize: %w", err)
	}

	// Send initialized notification.
	if err := c.Notify("notifications/initialized", map[string]any{}); err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("mcp: initialized notification: %w", err)
	}

	// Fetch tools.
	tools, err := c.ListTools()
	if err != nil {
		c.Close()
		return nil, nil, fmt.Errorf("mcp: tools/list: %w", err)
	}

	return c, tools, nil
}

// Start begins the read loop (for processing responses and notifications).
func (c *Client) Start() {
	go c.readLoop()
}

// Request sends a JSON-RPC request and waits for the response.
func (c *Client) Request(method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	ch := make(chan *jsonResponse, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.writeMessage(msgBytes); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-c.done:
		return nil, fmt.Errorf("mcp: client closed")
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: marshal notification: %w", err)
	}
	return c.writeMessage(msgBytes)
}

// ListTools calls tools/list and caches the result.
func (c *Client) ListTools() ([]ToolDef, error) {
	result, err := c.Request("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list: %w", err)
	}
	c.toolsMu.Lock()
	c.tools = resp.Tools
	c.toolsMu.Unlock()
	return resp.Tools, nil
}

// CallTool invokes a tool by name with the given arguments.
func (c *Client) CallTool(name string, args map[string]any) (CallResult, error) {
	result, err := c.Request("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return CallResult{}, err
	}
	var callResult CallResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return CallResult{}, fmt.Errorf("mcp: parse tools/call result: %w", err)
	}
	return callResult, nil
}

// Tools returns the cached tool definitions.
func (c *Client) Tools() []ToolDef {
	c.toolsMu.RLock()
	defer c.toolsMu.RUnlock()
	out := make([]ToolDef, len(c.tools))
	copy(out, c.tools)
	return out
}

// Close shuts down the server process. Safe to call on manually-constructed
// clients (cmd may be nil in tests using net.Pipe).
func (c *Client) Close() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

// writeMessage writes one newline-delimited JSON message.
func (c *Client) writeMessage(msg []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(msg); err != nil {
		return fmt.Errorf("mcp: write: %w", err)
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("mcp: write newline: %w", err)
	}
	return nil
}

// readLoop continuously reads newline-delimited JSON from the server.
func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			ID     *json.Number    `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *jsonError      `json:"error"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue // skip malformed
		}

		// If it has an ID and no method, it's a response.
		if envelope.ID != nil && envelope.Method == "" {
			id, err := envelope.ID.Int64()
			if err != nil {
				continue
			}
			if val, ok := c.pending.Load(id); ok {
				ch := val.(chan *jsonResponse)
				ch <- &jsonResponse{
					Result: envelope.Result,
					Error:  envelope.Error,
				}
			}
		}
		// Notifications (method but no id) are ignored for now.
	}
}
