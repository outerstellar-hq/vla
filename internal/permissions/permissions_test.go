package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NoFile(t *testing.T) {
	mgr, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if mgr.Check("write_file") != ActionAllow {
		t.Error("default should be allow when no config")
	}
	if mgr.IsBlocked("delete_file") {
		t.Error("nothing should be blocked by default")
	}
}

func TestLoad_WithRules(t *testing.T) {
	dir := t.TempDir()
	dir = dir + "/.vla"
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(`{
		"rules": [
			{"tool": "git_commit", "action": "deny"},
			{"tool": "write_file", "action": "ask"},
			{"tool": "read_file", "action": "allow"}
		],
		"default": "ask"
	}`), 0644)

	mgr, err := Load(filepath.Dir(dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if mgr.Check("git_commit") != ActionDeny {
		t.Error("git_commit should be denied")
	}
	if mgr.Check("write_file") != ActionAsk {
		t.Error("write_file should be ask")
	}
	if mgr.Check("read_file") != ActionAllow {
		t.Error("read_file should be allow")
	}
	if mgr.Check("unknown_tool") != ActionAsk {
		t.Error("default should be ask")
	}
}

func TestIsBlocked(t *testing.T) {
	mgr := &Manager{
		rules:   map[string]Action{"delete_file": ActionDeny, "read_file": ActionAllow},
		Default: ActionAsk,
	}
	if !mgr.IsBlocked("delete_file") {
		t.Error("delete_file should be blocked")
	}
	if mgr.IsBlocked("read_file") {
		t.Error("read_file should not be blocked")
	}
	if mgr.IsBlocked("write_file") {
		t.Error("write_file (ask) should not be blocked")
	}
}

func TestCheck_DefaultAllow(t *testing.T) {
	mgr := &Manager{
		rules:   make(map[string]Action),
		Default: ActionAllow,
	}
	if mgr.Check("anything") != ActionAllow {
		t.Error("should default to allow")
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	dir = dir + "/.vla"
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(`{bad json`), 0644)

	_, err := Load(filepath.Dir(dir))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
