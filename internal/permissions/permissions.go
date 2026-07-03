// Package permissions implements allow/deny rules per tool, configurable via
// .vla/permissions.json. Rules are evaluated before the approval system —
// if a tool is explicitly denied, it never reaches the approver.
//
// Rule evaluation order:
//  1. If a specific rule exists for the tool, use its action.
//  2. Otherwise, use the default action.
//
// Actions: "allow" (proceed, no approval needed), "deny" (blocked entirely),
// "ask" (requires human approval via the Approver).
package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Action is the permission decision for a tool.
type Action string

const (
	ActionAllow Action = "allow" // proceed without asking
	ActionDeny  Action = "deny"  // blocked entirely
	ActionAsk   Action = "ask"   // requires human approval
)

// Rule maps a single tool name to an action.
type Rule struct {
	Tool   string `json:"tool"`
	Action Action `json:"action"`
}

// Config is the shape of .vla/permissions.json.
type Config struct {
	Rules   []Rule `json:"rules"`
	Default Action `json:"default"` // what to do when no rule matches
}

// Manager evaluates permission rules for tool calls.
type Manager struct {
	rules   map[string]Action // tool name → action
	Default Action
}

// Load reads .vla/permissions.json from the project root. Returns a Manager
// with the configured rules. If the file doesn't exist, returns a permissive
// default (ask for destructive tools, allow everything else — the approval
// package handles the "ask" logic).
func Load(projectRoot string) (*Manager, error) {
	path := filepath.Join(projectRoot, ".vla", "permissions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config = permissive: allow everything, let the approval
			// system handle destructive tools.
			return &Manager{
				rules:   make(map[string]Action),
				Default: ActionAllow,
			}, nil
		}
		return nil, fmt.Errorf("permissions: read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("permissions: parse %s: %w", path, err)
	}

	m := &Manager{
		rules:   make(map[string]Action, len(cfg.Rules)),
		Default: ActionAllow,
	}
	if cfg.Default != "" {
		m.Default = cfg.Default
	}
	for _, r := range cfg.Rules {
		m.rules[r.Tool] = r.Action
	}
	return m, nil
}

// Check returns the permission action for a tool. If a specific rule exists,
// it takes precedence; otherwise the default action is returned.
func (m *Manager) Check(toolName string) Action {
	if action, ok := m.rules[toolName]; ok {
		return action
	}
	return m.Default
}

// IsBlocked returns true if the tool is explicitly denied (never reaches
// the approver).
func (m *Manager) IsBlocked(toolName string) bool {
	return m.Check(toolName) == ActionDeny
}

// AddOverride adds or replaces a permission rule at runtime. Used by plan
// mode to deny all destructive tools without editing the config file.
func (m *Manager) AddOverride(toolName string, action Action) {
	if m.rules == nil {
		m.rules = make(map[string]Action)
	}
	m.rules[toolName] = action
}
