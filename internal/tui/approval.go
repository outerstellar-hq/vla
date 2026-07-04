package tui

import (
	"fmt"
	"sync/atomic"
)

// ApprovalReq is sent to the TUI when a tool call needs human approval.
// The TUI displays it and sends back the decision on the Resp channel.
type ApprovalReq struct {
	ID      string
	Tool    string
	Preview string
	Resp    chan bool
}

// TUIApprover implements agent.ToolApprover by sending approval requests to
// the TUI via a channel and blocking until the user responds. This replaces
// ReadlineApprover in TUI mode (which would deadlock trying to read os.Stdin
// that bubbletea owns in raw mode).
//
// The TUI is expected to:
//  1. Receive ApprovalReq values from the Approvals channel
//  2. Display the tool name and preview to the user
//  3. Collect y/n/a input via bubbletea
//  4. Send the decision (true=approve, false=deny) on req.Resp
type TUIApprover struct {
	approvals chan ApprovalReq // TUI reads from this
	idCounter uint64
}

// NewTUIApprover creates an approver that routes approval requests through
// the given channel. The TUI should read from Approvals() in its event loop.
func NewTUIApprover() *TUIApprover {
	return &TUIApprover{
		approvals: make(chan ApprovalReq, 4),
	}
}

// Approvals returns the channel the TUI should read approval requests from.
func (a *TUIApprover) Approvals() <-chan ApprovalReq {
	return a.approvals
}

// RequiresApproval returns true for destructive tools. Same set as
// ReadlineApprover: write_file, update_file, delete_file, git_commit.
func (a *TUIApprover) RequiresApproval(toolName string) bool {
	switch toolName {
	case "write_file", "update_file", "delete_file", "git_commit":
		return true
	}
	return false
}

// Approve sends a request to the TUI and blocks until the user responds.
// Implements agent.ToolApprover.
func (a *TUIApprover) Approve(toolName string, args map[string]any, preview string) bool {
	id := fmt.Sprintf("approval-%d", atomic.AddUint64(&a.idCounter, 1))
	resp := make(chan bool, 1)

	req := ApprovalReq{
		ID:      id,
		Tool:    toolName,
		Preview: preview,
		Resp:    resp,
	}

	// Send the request (non-blocking send would be wrong — the loop MUST
	// wait for a decision before proceeding).
	a.approvals <- req

	// Block until the TUI sends back a decision.
	return <-resp
}
