package sandbox

import (
	"strings"
	"testing"
)

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNone, "none"},
		{ModeMacOSSandboxExec, "sandbox-exec (macOS Seatbelt)"},
		{ModeLinuxBwrap, "bwrap (Linux bubblewrap)"},
	}
	for _, tt := range tests {
		got := tt.mode.String()
		if got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	// On Windows, Enabled() should return false.
	// On macOS/Linux with the binary present, it returns true.
	// We just verify it doesn't panic.
	_ = Enabled()
}

func TestDetectDoesNotPanic(t *testing.T) {
	mode := Detect()
	if mode < ModeNone || mode > ModeLinuxBwrap {
		t.Errorf("Detect returned invalid mode: %d", mode)
	}
}

func TestCommandNone(t *testing.T) {
	// With ModeNone, Command should return args unchanged.
	args := []string{"vla", "--resume", "abc"}
	name, cmdArgs := Command(ModeNone, "/proj", args)
	if name != "vla" {
		t.Errorf("expected name 'vla', got %q", name)
	}
	if len(cmdArgs) != 2 || cmdArgs[0] != "--resume" || cmdArgs[1] != "abc" {
		t.Errorf("unexpected args: %v", cmdArgs)
	}
}

func TestCommandContainsProjectDir(t *testing.T) {
	// On platforms with a real sandbox, the Command output should embed
	// the project directory. On Windows (ModeNone), skip this check.
	mode := Detect()
	if mode == ModeNone {
		t.Skip("no sandbox available on this platform")
	}

	args := []string{"vla", "--resume", "test123"}
	name, cmdArgs := Command(mode, "/my/project", args)

	if name == "" {
		t.Fatal("expected non-empty command name")
	}

	// Verify the project dir appears somewhere in the generated command.
	allArgs := strings.Join(cmdArgs, " ")
	if !strings.Contains(allArgs, "/my/project") {
		t.Errorf("expected project dir in command args: %s", allArgs)
	}
}

func TestCommandPreservesOriginalArgs(t *testing.T) {
	// The original VLA args should appear in the sandboxed command.
	mode := Detect()
	if mode == ModeNone {
		t.Skip("no sandbox available on this platform")
	}

	args := []string{"/path/to/vla", "--resume", "abc123"}
	_, cmdArgs := Command(mode, "/proj", args)

	joined := strings.Join(cmdArgs, " ")
	if !strings.Contains(joined, "abc123") {
		t.Error("original args (abc123) should be preserved in sandbox command")
	}
	if !strings.Contains(joined, "--resume") {
		t.Error("original args (--resume) should be preserved in sandbox command")
	}
}
