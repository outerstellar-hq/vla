package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerConfig defines one MCP server to launch, from .vla/mcp.json.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ConfigFile is the shape of .vla/mcp.json.
type ConfigFile struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// LoadConfig reads .vla/mcp.json from the given project root. Returns an
// empty config if the file doesn't exist (MCP is optional).
func LoadConfig(projectRoot string) (*ConfigFile, error) {
	path := filepath.Join(projectRoot, ".vla", "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ConfigFile{Servers: map[string]ServerConfig{}}, nil
		}
		return nil, fmt.Errorf("mcp: read %s: %w", path, err)
	}
	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("mcp: parse %s: %w", path, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	return &cfg, nil
}

// Manager owns a pool of MCP server connections. One connection per server
// defined in .vla/mcp.json. Tools from all servers are aggregated.
type Manager struct {
	clients map[string]*Client // keyed by server name
}

// NewManager is a placeholder until servers are started.
func NewManager() *Manager {
	return &Manager{clients: make(map[string]*Client)}
}

// StartAll launches all MCP servers defined in the config, performing the
// initialize handshake and fetching tools from each. Returns the combined
// tool list. Servers that fail to start are skipped with a warning printed
// to the provided writer (usually os.Stderr).
func (m *Manager) StartAll(cfg *ConfigFile, warn func(format string, args ...any)) {
	for name, sc := range cfg.Servers {
		// Build env slice from the map.
		var envSlice []string
		for k, v := range sc.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		client, _, err := Start(sc.Command, sc.Args, envSlice)
		if err != nil {
			if warn != nil {
				warn("vla: mcp server %s failed to start: %v\n", name, err)
			}
			continue
		}
		m.clients[name] = client
		if warn != nil {
			tools := client.Tools()
			warn("vla: mcp server %s connected (%d tools)\n", name, len(tools))
		}
	}
}

// Close shuts down all MCP servers.
func (m *Manager) Close() {
	for _, c := range m.clients {
		_ = c.Close()
	}
	m.clients = make(map[string]*Client)
}

// AllTools returns tool definitions from all connected MCP servers, prefixed
// with the server name to avoid collisions (e.g. "github__create_issue").
func (m *Manager) AllTools() []ServerTool {
	var all []ServerTool
	for name, client := range m.clients {
		for _, t := range client.Tools() {
			all = append(all, ServerTool{
				ServerName: name,
				Tool:       t,
				Client:     client,
			})
		}
	}
	return all
}

// ServerTool pairs an MCP tool definition with the client that owns it.
type ServerTool struct {
	ServerName string
	Tool       ToolDef
	Client     *Client
}
