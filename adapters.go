package main

import (
	"github.com/abrandt/vla/internal/approval"
	"github.com/abrandt/vla/internal/permissions"
)

// approverAdapter wraps a ReadlineApprover to satisfy agent.ToolApprover.
type approverAdapter struct {
	rl *approval.ReadlineApprover
}

func (a approverAdapter) RequiresApproval(toolName string) bool {
	return a.rl.RequiresApproval(toolName)
}

func (a approverAdapter) Approve(toolName string, args map[string]any, preview string) bool {
	return a.rl.Approve(toolName, args, preview)
}

// alwaysApprover auto-approves everything (--yes flag).
type alwaysApprover struct{}

func (alwaysApprover) RequiresApproval(string) bool                { return false } // never asks
func (alwaysApprover) Approve(string, map[string]any, string) bool { return true }

// permChecker bridges permissions.Manager to agent.ToolPermissionChecker.
type permChecker struct {
	mgr *permissions.Manager
}

func (p permChecker) IsBlocked(toolName string) bool {
	return p.mgr.IsBlocked(toolName)
}
