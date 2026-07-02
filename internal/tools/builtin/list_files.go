package builtin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abrandt/vla/internal/fsutil"
)

// ListFiles walks a directory tree (or the whole project if path is empty)
// and returns a relative-path listing. Skips common noise directories
// (.git, node_modules, vendor, build output, etc.) to avoid flooding the
// context window. Confined to BaseDir.
type ListFiles struct{ Ctx Ctx }

func (ListFiles) Name() string { return "list_files" }

func (ListFiles) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to list, relative to project root. Empty or \".\" = whole project.",
			},
		},
	}
}

func (l ListFiles) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	abs, err := fsutil.Confine(l.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	var files []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		rel, _ := filepath.Rel(l.Ctx.BaseDir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if ignoredDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return fmt.Sprintf("Error: walk %s: %v", in.Path, err), nil
	}
	if len(files) == 0 {
		return "no files found", nil
	}
	sort.Strings(files)
	const maxList = 500
	if len(files) > maxList {
		return strings.Join(files[:maxList], "\n") + fmt.Sprintf("\n...[%d more files omitted]", len(files)-maxList), nil
	}
	return strings.Join(files, "\n"), nil
}

// ignoredDir returns true for directories we never descend into.
func ignoredDir(rel string) bool {
	if rel == "." {
		return false
	}
	switch filepath.Base(rel) {
	case ".git", "node_modules", "vendor", "__pycache__", ".venv", "venv",
		"dist", "build", "target", ".idea", ".vscode", ".cache":
		return true
	}
	return false
}
