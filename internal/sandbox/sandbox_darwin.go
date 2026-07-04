//go:build darwin

package sandbox

import (
	"os"
	"os/exec"
	"strings"
)

// Detect returns the sandbox mode available on this system. On macOS it
// checks for the sandbox-exec binary (ships with macOS, located at
// /usr/bin/sandbox-exec).
func Detect() Mode {
	if _, err := exec.LookPath("sandbox-exec"); err == nil {
		return ModeMacOSSandboxExec
	}
	return ModeNone
}

// Command wraps the VLA invocation in a sandbox-exec command that
// restricts filesystem access to the project directory. The generated
// Seatbelt profile allows:
//   - Full read/write to the project directory
//   - Read-only access to system paths (/usr, /bin, /lib, /System)
//   - Read/write to temp directories and the user's home .vla directory
//   - Network access (for LLM API calls)
//
// Everything else is denied by default.
func Command(mode Mode, projectDir string, args []string) (string, []string) {
	if mode != ModeMacOSSandboxExec {
		return args[0], args[1:]
	}

	profile := macosProfile(projectDir)
	return "sandbox-exec", append([]string{"-p", profile, "--"}, args...)
}

// macosProfile generates a Seatbelt sandbox profile for the given project.
// The profile uses (allow ...) rules; anything not explicitly allowed is
// denied.
func macosProfile(projectDir string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")

	// Allow the VLA process to run normally.
	b.WriteString("(allow process-info* (target self))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow file-read-metadata)\n")

	// Project directory: full access.
	b.WriteString("    (allow file* (subpath \"")
	b.WriteString(projectDir)
	b.WriteString("\"))\n")

	// System paths: read-only (need to read shared libs, binaries).
	for _, p := range []string{"/usr", "/bin", "/sbin", "/lib", "/System", "/dev", "/etc"} {
		b.WriteString("    (allow file-read* (subpath \"")
		b.WriteString(p)
		b.WriteString("\"))\n")
	}

	// Temp directories.
	b.WriteString("    (allow file* (subpath \"/tmp\"))\n")
	b.WriteString("    (allow file* (subpath \"/var/tmp\"))\n")

	// User's VLA config directory (~/.vla) — for sessions, memory, config.
	if home := homeDir(); home != "" {
		vlaDir := home + "/.vla"
		b.WriteString("    (allow file* (subpath \"")
		b.WriteString(vlaDir)
		b.WriteString("\"))\n")
	}

	// Network access for LLM API calls.
	b.WriteString("    (allow network*)\n")

	// Process execution (the LLM may spawn tools, git, etc.).
	b.WriteString("    (allow process-exec)\n")
	b.WriteString("    (allow process-fork)\n")

	return b.String()
}

// homeDir returns the user's home directory, or empty string on error.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}
