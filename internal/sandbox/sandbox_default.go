//go:build !darwin && !linux && !windows

package sandbox

// Detect returns ModeNone on unsupported platforms (BSD, etc.).
func Detect() Mode {
	return ModeNone
}

// Command returns the args unchanged on unsupported platforms.
func Command(_ Mode, _ string, args []string) (string, []string) {
	return args[0], args[1:]
}
