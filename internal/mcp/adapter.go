package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/abrandt/vla/internal/tools"
)

// MCPTool adapts an MCP server tool to VLA's tools.Tool interface. The LLM
// sees it as a native VLA tool; when called, the adapter forwards to the MCP
// server's tools/call and returns the text content.
type MCPTool struct {
	ServerName string  // prefix for the tool name (e.g. "github")
	Def        ToolDef // the MCP tool definition
	Client     *Client // the connection to the MCP server
}

// Name returns the prefixed tool name (e.g. "github__create_issue") to
// avoid collisions between MCP servers and VLA's built-in tools.
func (t MCPTool) Name() string {
	return t.ServerName + "__" + t.Def.Name
}

// Schema returns the MCP tool's input schema as VLA expects it.
func (t MCPTool) Schema() map[string]any {
	if t.Def.InputSchema != nil {
		return t.Def.InputSchema
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// Execute calls the MCP tool and returns its text result. Per VLA's
// convention, MCP tool errors come back as result strings ("Error: ...").
func (t MCPTool) Execute(args json.RawMessage) (string, error) {
	var argMap map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &argMap); err != nil {
			return fmt.Sprintf("Error: could not parse arguments: %v", err), nil
		}
	}
	result, err := t.Client.CallTool(t.Def.Name, argMap)
	if err != nil {
		return fmt.Sprintf("Error: %s: %v", t.Def.Name, err), nil
	}
	if result.IsError {
		return "Error: " + result.Text(), nil
	}
	text := result.Text()
	if text == "" {
		return "(tool returned no output)", nil
	}
	return text, nil
}

// RegisterAll wraps every MCP tool as a MCPTool and registers it with the
// VLA tool registry.
func RegisterAll(registry *tools.Registry, mgr *Manager) error {
	for _, st := range mgr.AllTools() {
		tool := MCPTool{
			ServerName: st.ServerName,
			Def:        st.Tool,
			Client:     st.Client,
		}
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("mcp: register %s: %w", tool.Name(), err)
		}
	}
	return nil
}

// Compile-time check.
var _ tools.Tool = MCPTool{}
