package main

import "github.com/abrandt/vla/internal/hooks"

// hookAdapter bridges hooks.Manager to agent.HookRunner.
type hookAdapter struct {
	mgr *hooks.Manager
}

func (h hookAdapter) RunBeforeTool(toolName string) error {
	return h.mgr.Run(hooks.EventBeforeTool, toolName, nil)
}

func (h hookAdapter) RunAfterTool(toolName, result string) {
	env := map[string]string{"VLA_RESULT": truncateResult(result)}
	_ = h.mgr.Run(hooks.EventAfterTool, toolName, env)
	if toolName == "write_file" || toolName == "update_file" {
		_ = h.mgr.Run(hooks.EventOnWrite, toolName, env)
	}
}

func truncateResult(s string) string {
	if len(s) > 1000 {
		return s[:1000] + "…"
	}
	return s
}
