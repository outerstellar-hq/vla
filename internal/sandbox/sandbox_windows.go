//go:build windows

package sandbox

// Detect returns ModeNone on Windows. Windows doesn't have a simple
// external-binary sandbox equivalent to sandbox-exec or bwrap.
//
// Path confinement on Windows relies on:
//   - fsutil.Confine (lexical + symlink resolution)
//   - Permission system (.vla/permissions.json)
//   - Diff approval (human-in-the-loop for destructive tools)
//
// For stronger isolation on Windows, run VLA under a restricted user
// account or use Windows AppContainer / WDAG (Windows Defender Application
// Guard) configured at the OS level.
func Detect() Mode {
	return ModeNone
}

// Command returns the args unchanged on Windows (no sandbox available).
func Command(_ Mode, _ string, args []string) (string, []string) {
	return args[0], args[1:]
}
