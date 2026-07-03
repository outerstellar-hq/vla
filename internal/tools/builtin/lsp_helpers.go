package builtin

import (
	"path/filepath"
	"strings"
)

// filepathIsAbs is filepath.IsAbs (aliased so the builtin package reads cleanly).
func filepathIsAbs(p string) bool { return filepath.IsAbs(p) }

// joinPath is filepath.Join (aliased).
func joinPath(elem ...string) string { return filepath.Join(elem...) }

// pathToURIString converts a filesystem path to an LSP file:// URI.
func pathToURIString(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}

// uriToRelPath converts an LSP file:// URI to a project-relative path.
func uriToRelPath(uri, baseDir string) string {
	path := strings.TrimPrefix(uri, "file://")
	// On Windows, strip the leading / before a drive letter: /C:/... → C:/...
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	rel, err := filepath.Rel(baseDir, filepath.FromSlash(path))
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
