package approval

import "testing"

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
