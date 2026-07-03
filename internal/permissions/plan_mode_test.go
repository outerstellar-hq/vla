package permissions

import "testing"

func TestManager_AddOverride(t *testing.T) {
	mgr := &Manager{
		rules:   make(map[string]Action),
		Default: ActionAllow,
	}
	// Nothing blocked initially.
	if mgr.IsBlocked("write_file") {
		t.Error("write_file should not be blocked initially")
	}
	// Add override.
	mgr.AddOverride("write_file", ActionDeny)
	if !mgr.IsBlocked("write_file") {
		t.Error("write_file should be blocked after override")
	}
	if mgr.Check("write_file") != ActionDeny {
		t.Error("Check should return deny")
	}
	// Other tools still allowed.
	if mgr.IsBlocked("read_file") {
		t.Error("read_file should not be blocked")
	}
}

func TestManager_AddOverride_Replaces(t *testing.T) {
	mgr := &Manager{
		rules:   map[string]Action{"write_file": ActionAsk},
		Default: ActionAllow,
	}
	mgr.AddOverride("write_file", ActionDeny)
	if mgr.Check("write_file") != ActionDeny {
		t.Error("override should replace existing rule")
	}
}
