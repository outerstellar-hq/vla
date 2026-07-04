package approval

import (
	"testing"
)

func TestAlwaysApprover(t *testing.T) {
	a := AlwaysApprover{}
	d := a.Approve(Request{ToolName: "write_file", Preview: "test"})
	if d.Action != ActionAllow {
		t.Error("AlwaysApprover should allow")
	}
}

func TestRequiresApproval(t *testing.T) {
	// Destructive tools require approval.
	for _, tool := range []string{"write_file", "update_file", "delete_file", "git_commit"} {
		if !RequiresApproval(tool) {
			t.Errorf("expected %q to require approval", tool)
		}
	}
	// Read-only tools don't.
	for _, tool := range []string{"read_file", "search", "list_files", "go_to_definition", "memory_save"} {
		if RequiresApproval(tool) {
			t.Errorf("expected %q to NOT require approval", tool)
		}
	}
}

func TestAllowAll(t *testing.T) {
	a := AllowAll()
	d := a.Approve(Request{ToolName: "delete_file"})
	if d.Action != ActionAllow {
		t.Error("AllowAll should allow")
	}
}

func TestAlwaysApprover_DifferentTools(t *testing.T) {
	a := AlwaysApprover{}
	// Should allow regardless of tool name.
	for _, tool := range []string{"write_file", "delete_file", "git_commit", "unknown_tool"} {
		d := a.Approve(Request{ToolName: tool})
		if d.Action != ActionAllow {
			t.Errorf("AlwaysApprover should allow %q", tool)
		}
	}
}

func TestAlwaysApprover_WithArgs(t *testing.T) {
	a := AlwaysApprover{}
	d := a.Approve(Request{
		ToolName: "write_file",
		Args:     map[string]any{"path": "/foo", "content": "bar"},
		Preview:  "WRITE /foo (3 bytes): bar",
	})
	if d.Action != ActionAllow {
		t.Error("should allow with args")
	}
}

func TestRequiresApproval_EmptyString(t *testing.T) {
	if RequiresApproval("") {
		t.Error("empty string should not require approval")
	}
}

func TestRequiresApproval_UnknownTool(t *testing.T) {
	if RequiresApproval("some_random_tool") {
		t.Error("unknown tool should not require approval")
	}
}

func TestRequireApprovalMap(t *testing.T) {
	// Verify the map has exactly the expected entries.
	expected := map[string]bool{
		"write_file":  true,
		"update_file": true,
		"delete_file": true,
		"git_commit":  true,
	}
	if len(RequireApproval) != len(expected) {
		t.Errorf("RequireApproval has %d entries, expected %d", len(RequireApproval), len(expected))
	}
	for tool := range expected {
		if !RequireApproval[tool] {
			t.Errorf("RequireApproval missing %q", tool)
		}
	}
}

func TestActionConstants(t *testing.T) {
	// Ensure action constants have distinct values.
	if ActionAllow == ActionDeny {
		t.Error("ActionAllow and ActionDeny should be distinct")
	}
}

func TestDecision_Fields(t *testing.T) {
	d := Decision{Action: ActionDeny, Message: "blocked by policy"}
	if d.Action != ActionDeny {
		t.Error("should be deny")
	}
	if d.Message != "blocked by policy" {
		t.Error("message mismatch")
	}
}

func TestRequest_Fields(t *testing.T) {
	r := Request{
		ToolName: "update_file",
		Args:     map[string]any{"path": "/foo", "old": "a", "new": "b"},
		Preview:  "UPDATE /foo",
	}
	if r.ToolName != "update_file" {
		t.Error("toolname mismatch")
	}
	if len(r.Args) != 3 {
		t.Error("args mismatch")
	}
	if r.Preview != "UPDATE /foo" {
		t.Error("preview mismatch")
	}
}

// --- ReadlineApprover tests ---

func TestReadlineApprover_RequiresApproval(t *testing.T) {
	a := NewReadlineApprover()
	// Should match the global RequireApproval map.
	if !a.RequiresApproval("write_file") {
		t.Error("write_file should require approval")
	}
	if a.RequiresApproval("read_file") {
		t.Error("read_file should NOT require approval")
	}
}

func TestReadlineApprover_ApproveAllFlag(t *testing.T) {
	a := NewReadlineApprover()
	a.approveAll = true

	// When approveAll is set, should always return true without prompting.
	if !a.Approve("write_file", nil, "test") {
		t.Error("should auto-approve when approveAll=true")
	}
	if !a.Approve("delete_file", nil, "test") {
		t.Error("should auto-approve all tools when approveAll=true")
	}
}

func TestReadlineApprover_processAnswer_Yes(t *testing.T) {
	a := NewReadlineApprover()
	for _, input := range []string{"y", "yes", "Y", "YES", "Yes"} {
		a.approveAll = false // reset
		if !a.processAnswer(input) {
			t.Errorf("processAnswer(%q) should return true", input)
		}
		if a.approveAll {
			t.Errorf("processAnswer(%q) should NOT set approveAll", input)
		}
	}
}

func TestReadlineApprover_processAnswer_All(t *testing.T) {
	a := NewReadlineApprover()
	for _, input := range []string{"a", "all", "A", "ALL", "All"} {
		a.approveAll = false // reset
		if !a.processAnswer(input) {
			t.Errorf("processAnswer(%q) should return true", input)
		}
		if !a.approveAll {
			t.Errorf("processAnswer(%q) should set approveAll=true", input)
		}
	}
}

func TestReadlineApprover_processAnswer_Deny(t *testing.T) {
	a := NewReadlineApprover()
	for _, input := range []string{"n", "no", "N", "NO", "", "garbage", "x", "maybe"} {
		a.approveAll = false // reset
		if a.processAnswer(input) {
			t.Errorf("processAnswer(%q) should return false", input)
		}
		if a.approveAll {
			t.Errorf("processAnswer(%q) should NOT set approveAll", input)
		}
	}
}

func TestReadlineApprover_processAnswer_AllThenAutoApprove(t *testing.T) {
	a := NewReadlineApprover()

	// First call with "a" → approve + set approveAll.
	if !a.processAnswer("a") {
		t.Error("first 'a' should return true")
	}
	if !a.approveAll {
		t.Error("approveAll should be set after 'a'")
	}

	// After approveAll, Approve() short-circuits without calling processAnswer.
	if !a.Approve("write_file", nil, "test") {
		t.Error("after approveAll, Approve() should auto-approve")
	}
}
