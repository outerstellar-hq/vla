package commands

import (
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/tools"
)

func testCtx() Context {
	reg := tools.NewRegistry()
	return Context{
		Registry:  reg,
		Model:     "test-model",
		SessionID: "test-session",
		ToolCount: 5,
	}
}

func TestIsSlashCommand(t *testing.T) {
	if !IsSlashCommand("/help") {
		t.Error("expected /help to be a slash command")
	}
	if !IsSlashCommand("  /tools") {
		t.Error("expected indented /tools to be a slash command")
	}
	if IsSlashCommand("hello world") {
		t.Error("regular message should not be a slash command")
	}
	if IsSlashCommand("") {
		t.Error("empty string should not be a slash command")
	}
}

func TestExecute_Help(t *testing.T) {
	r := Execute("/help", testCtx())
	if !r.Handled {
		t.Fatal("expected /help to be handled")
	}
	if !strings.Contains(r.Output, "Slash Commands") {
		t.Errorf("output missing help text: %q", r.Output)
	}
	if !strings.Contains(r.Output, "/tools") {
		t.Errorf("help should list /tools")
	}
}

func TestExecute_Model(t *testing.T) {
	r := Execute("/model", testCtx())
	if !r.Handled {
		t.Fatal("expected /model to be handled")
	}
	if !strings.Contains(r.Output, "test-model") {
		t.Errorf("expected model name in output: %q", r.Output)
	}
}

func TestExecute_Session(t *testing.T) {
	r := Execute("/session", testCtx())
	if !r.Handled {
		t.Fatal("expected /session to be handled")
	}
	if !strings.Contains(r.Output, "test-session") {
		t.Errorf("expected session ID: %q", r.Output)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	r := Execute("/nonexistent", testCtx())
	if !r.Handled {
		t.Fatal("unknown commands should be Handled (with error message)")
	}
	if !strings.Contains(r.Output, "Unknown command") {
		t.Errorf("expected unknown command message: %q", r.Output)
	}
}

func TestExecute_NotSlashCommand(t *testing.T) {
	// Non-slash input: IsSlashCommand returns false, so the loop never calls
	// Execute. This test verifies IsSlashCommand filters correctly.
	if IsSlashCommand("hello world") {
		t.Error("regular message should not be detected as slash command")
	}
}

func TestExecute_Tools(t *testing.T) {
	ctx := testCtx()
	// We can't import builtin here (import cycle), so we test with an
	// empty registry — should show "0 tools".
	r := Execute("/tools", ctx)
	if !r.Handled {
		t.Fatal("expected /tools to be handled")
	}
	if !strings.Contains(r.Output, "0 tools") {
		t.Errorf("expected 0 tools: %q", r.Output)
	}
}

func TestExecute_MemorySearch(t *testing.T) {
	ctx := testCtx()
	ctx.MemSearch = func(q string) (string, error) {
		return "found: " + q, nil
	}
	r := Execute("/memory search test query", ctx)
	if !r.Handled {
		t.Fatal("expected /memory search to be handled")
	}
	if !strings.Contains(r.Output, "test query") {
		t.Errorf("expected query in output: %q", r.Output)
	}
}

func TestExecute_MemorySave(t *testing.T) {
	ctx := testCtx()
	ctx.MemSave = func(text string) (string, error) {
		return "saved: " + text, nil
	}
	r := Execute("/memory save important fact", ctx)
	if !r.Handled {
		t.Fatal("expected /memory save to be handled")
	}
	if !strings.Contains(r.Output, "important fact") {
		t.Errorf("expected saved text: %q", r.Output)
	}
}

func TestExecute_MemoryNoArgs(t *testing.T) {
	r := Execute("/memory", testCtx())
	if !r.Handled {
		t.Fatal("expected /memory to be handled")
	}
	if !strings.Contains(r.Output, "Usage:") {
		t.Errorf("expected usage message: %q", r.Output)
	}
}

func TestExecute_Compact(t *testing.T) {
	ctx := testCtx()
	ctx.TriggerCompact = func() {}
	r := Execute("/compact", ctx)
	if !r.Handled {
		t.Fatal("expected /compact to be handled")
	}
	if !strings.Contains(r.Output, "Compaction") {
		t.Errorf("expected compaction message: %q", r.Output)
	}
}

func TestExecute_CompactNotAvailable(t *testing.T) {
	ctx := testCtx()
	r := Execute("/compact", ctx)
	if !r.Handled {
		t.Fatal("expected /compact to be handled")
	}
	if !strings.Contains(r.Output, "not available") {
		t.Errorf("expected not-available message: %q", r.Output)
	}
}
