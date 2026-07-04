// Package fsutil provides shared filesystem helpers for VLA tools: path
// confinement to a BaseDir (so the LLM cannot escape the project), and
// a ToolContext that carries session-scoped settings every tool needs.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxReadBytes is the default cap on read_file output. Prevents the LLM
// from slurping a 2GB file into the context window. Tools may override
// per-call via their arguments.
const MaxReadBytes = 256 * 1024 // 256 KiB

// Confine resolves relPath against baseDir and guarantees the result is
// inside baseDir. Returns an error if the path escapes via "../" or via
// symlinks pointing outside the project root.
//
// The confinement is two-layer:
//  1. Lexical: Clean + prefix-match catches "../" traversals.
//  2. Symlink-safe: filepath.EvalSymlinks resolves any symlinks in the
//     path and re-checks the resolved path hasn't escaped. This closes
//     the gap where a symlink inside the project points to /etc/passwd.
//
// For paths that don't exist yet (e.g., write_file creating a new file),
// the symlink check resolves the parent directory instead.
//
// The returned path is absolute and cleaned. If the path exists, it is
// fully symlink-resolved.
func Confine(baseDir, relPath string) (string, error) {
	cleaned := filepath.Clean(relPath)
	var candidate string
	if filepath.IsAbs(cleaned) {
		candidate = cleaned
	} else {
		candidate = filepath.Join(baseDir, cleaned)
	}
	candidate = filepath.Clean(candidate)

	// Layer 1: lexical confinement.
	if err := checkWithin(candidate, baseDir, relPath); err != nil {
		return "", err
	}

	// Layer 2: symlink resolution (defense-in-depth).
	// Resolve the baseDir first so we're comparing apples to apples
	// (e.g., on Windows, temp paths may have 8.3 short names that differ
	// from the EvalSymlinks-resolved form).
	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		resolvedBase = baseDir // fall back to lexical if base doesn't resolve
	}

	// Try to resolve the full candidate path (handles existing files/dirs).
	resolved, err := filepath.EvalSymlinks(candidate)
	if err == nil {
		if err := checkWithin(resolved, resolvedBase, relPath); err != nil {
			return "", fmt.Errorf("path %q (symlink resolves outside project root): %w", relPath, err)
		}
		return resolved, nil
	}

	// Path doesn't exist (new file). Resolve the parent directory to catch
	// symlinks in intermediate path components.
	parent := filepath.Dir(candidate)
	if parent != candidate { // guard against root
		resolvedParent, perr := filepath.EvalSymlinks(parent)
		if perr == nil {
			reconstructed := filepath.Join(resolvedParent, filepath.Base(candidate))
			if err := checkWithin(reconstructed, resolvedBase, relPath); err != nil {
				return "", fmt.Errorf("path %q (parent symlink resolves outside project root): %w", relPath, err)
			}
			return reconstructed, nil
		}
	}

	// Neither the path nor its parent exists. Fall back to lexical confinement
	// (the path will fail at os.Open/os.WriteFile time if it's truly invalid).
	return candidate, nil
}

// checkWithin verifies that candidate is inside baseDir. Returns an error
// if the path escapes.
func checkWithin(candidate, baseDir, relPath string) error {
	base := filepath.Clean(baseDir)
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	if candidate == filepath.Clean(baseDir) {
		return nil // exactly the base dir is allowed
	}
	if !strings.HasPrefix(candidate+string(filepath.Separator), base) {
		return fmt.Errorf("path %q escapes project root", relPath)
	}
	return nil
}

// IsSymlink returns true if the path is a symlink. Useful for tools that
// want to warn or block symlinked paths explicitly.
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
