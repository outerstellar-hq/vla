// Package sandbox provides OS-level process isolation for VLA. When
// enabled via the --sandbox flag, VLA re-executes itself inside a sandbox
// that restricts filesystem access to the project directory.
//
// Supported platforms:
//   - macOS: sandbox-exec (Seatbelt sandbox profiles)
//   - Linux: bwrap (bubblewrap — user namespaces, no root needed)
//   - Windows: not supported (lexical confinement + symlink check still apply)
//
// The sandbox is defense-in-depth on top of fsutil.Confine, which already
// provides lexical path confinement and symlink resolution. The OS sandbox
// provides a hard kernel-level guarantee that even if a bug in path
// validation allows an escape, the kernel will block the actual file
// operation.
package sandbox

// Mode identifies the sandbox mechanism to use.
type Mode int

const (
	ModeNone Mode = iota
	ModeMacOSSandboxExec
	ModeLinuxBwrap
)

// String returns a human-readable name for the mode.
func (m Mode) String() string {
	switch m {
	case ModeMacOSSandboxExec:
		return "sandbox-exec (macOS Seatbelt)"
	case ModeLinuxBwrap:
		return "bwrap (Linux bubblewrap)"
	default:
		return "none"
	}
}

// Enabled returns true if a sandbox mechanism is available on this platform.
func Enabled() bool {
	return Detect() != ModeNone
}
