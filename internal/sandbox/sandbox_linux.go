//go:build linux

package sandbox

import (
	"os"
	"os/exec"
)

// Detect returns the sandbox mode available on this system. On Linux it
// checks for the bwrap (bubblewrap) binary.
func Detect() Mode {
	if _, err := exec.LookPath("bwrap"); err == nil {
		return ModeLinuxBwrap
	}
	return ModeNone
}

// Command wraps the VLA invocation in a bwrap command that restricts
// filesystem access using bubblewrap (user namespaces). The sandbox:
//   - Bind-mounts the project directory read-write
//   - Provides a minimal /usr, /bin, /lib (read-only from host)
//   - Provides /tmp and the user's ~/.vla directory
//   - Allows network access (for LLM API calls)
//   - Denies access to everything else on the host filesystem
func Command(mode Mode, projectDir string, args []string) (string, []string) {
	if mode != ModeLinuxBwrap {
		return args[0], args[1:]
	}

	bwrapArgs := []string{
		// Create a new mount namespace (the core of the sandbox).
		"--unshare-all",
		// Share the network namespace (need network for API calls).
		"--share-net",

		// Mount a minimal proc and dev.
		"--proc", "/proc",
		"--dev", "/dev",

		// Bind-mount system directories read-only.
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/lib64", "/lib64",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/sbin", "/sbin",
		"--ro-bind", "/etc", "/etc",
		"--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf",

		// Temp directories.
		"--bind", "/tmp", "/tmp",

		// Project directory: read-write.
		"--bind", projectDir, projectDir,
	}

	// Bind-mount the user's ~/.vla directory (sessions, memory, config).
	if home, err := os.UserHomeDir(); err == nil {
		vlaDir := home + "/.vla"
		bwrapArgs = append(bwrapArgs, "--bind", vlaDir, vlaDir)
	}

	// Run the VLA binary with its original arguments.
	bwrapArgs = append(bwrapArgs, "--")
	bwrapArgs = append(bwrapArgs, args...)

	return "bwrap", bwrapArgs
}
