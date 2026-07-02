// Package fsutil provides shared filesystem helpers for VLA tools: path
// confinement to a BaseDir (so the LLM cannot escape the project), and
// a ToolContext that carries session-scoped settings every tool needs.
package fsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// MaxReadBytes is the default cap on read_file output. Prevents the LLM
// from slurping a 2GB file into the context window. Tools may override
// per-call via their arguments.
const MaxReadBytes = 256 * 1024 // 256 KiB

// Confine resolves relPath against baseDir and guarantees the result is
// inside baseDir. Returns an error (as a string, suitable for returning
// from Tool.Execute) if the path escapes via "../" or symlinks-which-
// we-don't-follow (we use filepath.IsAbs + Clean + HasPrefix on the
// cleaned lexical form — lexical confinement, not symlink-safe).
//
// The returned path is absolute and cleaned.
func Confine(baseDir, relPath string) (string, error) {
	// If relPath is absolute and already inside base, accept it; otherwise
	// treat it as relative to base. This matches how a human uses paths in
	// a project: sometimes "src/foo.go", sometimes the absolute path.
	cleaned := filepath.Clean(relPath)
	var candidate string
	if filepath.IsAbs(cleaned) {
		candidate = cleaned
	} else {
		candidate = filepath.Join(baseDir, cleaned)
	}
	candidate = filepath.Clean(candidate)

	base := filepath.Clean(baseDir)
	// Ensure both end with a separator so prefix match doesn't catch
	// /home/foo matching /home/foobar.
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	if candidate == filepath.Clean(baseDir) {
		// Exactly the base dir is allowed.
		return candidate, nil
	}
	if !strings.HasPrefix(candidate+string(filepath.Separator), base) {
		return "", fmt.Errorf("path %q escapes project root", relPath)
	}
	return candidate, nil
}
