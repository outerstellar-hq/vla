// Package hooks implements user-defined scripts that run before/after
// specific events. Configured via .vla/hooks.json.
//
// Hook events:
//   - before_tool: runs before any tool executes (can block by exiting non-zero)
//   - after_tool: runs after a tool completes
//   - on_write: runs after write_file/update_file (e.g. auto-format, auto-lint)
//   - on_session_start: runs when a session begins
//
// Each hook is a shell command. VLA runs it with the event details as
// environment variables (VLA_EVENT, VLA_TOOL, VLA_FILE, VLA_RESULT).
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Event is the type of hook trigger.
type Event string

const (
	EventBeforeTool   Event = "before_tool"
	EventAfterTool    Event = "after_tool"
	EventOnWrite      Event = "on_write"
	EventSessionStart Event = "on_session_start"
)

// Hook is one hook definition from .vla/hooks.json.
type Hook struct {
	Event   Event  `json:"event"`
	Tool    string `json:"tool,omitempty"` // filter: only run for this tool
	Command string `json:"command"`        // shell command to run
}

// Config is the shape of .vla/hooks.json.
type Config struct {
	Hooks []Hook `json:"hooks"`
}

// Manager stores and executes hooks.
type Manager struct {
	hooks []Hook
	root  string // project root for running commands
}

// Load reads .vla/hooks.json from the project root.
func Load(root string) *Manager {
	path := filepath.Join(root, ".vla", "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return &Manager{root: root} // no hooks = no-op
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Manager{root: root}
	}
	return &Manager{hooks: cfg.Hooks, root: root}
}

// Run executes all hooks matching the given event and tool. The env map
// provides context (VLA_TOOL, VLA_FILE, VLA_RESULT) as environment variables.
// Hooks run synchronously; a before_tool hook that exits non-zero blocks
// the tool call (returns error).
func (m *Manager) Run(event Event, toolName string, env map[string]string) error {
	for _, h := range m.hooks {
		if h.Event != event {
			continue
		}
		if h.Tool != "" && h.Tool != toolName {
			continue
		}
		if err := m.runCommand(h.Command, event, toolName, env); err != nil {
			if event == EventBeforeTool {
				return fmt.Errorf("hook blocked: %w", err)
			}
			// Non-blocking events: just log, don't fail.
		}
	}
	return nil
}

func (m *Manager) runCommand(command string, event Event, toolName string, env map[string]string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = m.root
	cmd.Env = append(os.Environ(),
		"VLA_EVENT="+string(event),
		"VLA_TOOL="+toolName,
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd.Run()
}

// HasHooks returns true if any hooks are configured.
func (m *Manager) HasHooks() bool {
	return len(m.hooks) > 0
}
