package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/fsutil"
)

// WriteFile creates or overwrites a file, creating parent directories as
// needed. Confined to BaseDir.
type WriteFile struct{ Ctx Ctx }

func (WriteFile) Name() string { return "write_file" }

func (WriteFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write. Relative to project root, or absolute within it.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The full contents to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (w WriteFile) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	abs, err := fsutil.Confine(w.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fmt.Sprintf("Error: create dirs for %s: %v", in.Path, err), nil
	}
	// Record old state for /undo before overwriting.
	if w.Ctx.UndoStack != nil {
		_ = w.Ctx.UndoStack.Push(abs)
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0644); err != nil {
		return fmt.Sprintf("Error: write %s: %v", in.Path, err), nil
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}
