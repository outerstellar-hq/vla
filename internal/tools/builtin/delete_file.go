package builtin

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/abrandt/vla/internal/fsutil"
)

// DeleteFile removes a file. Refuses to remove directories (the LLM should
// delete files individually — directory deletion is too destructive).
// Confined to BaseDir.
type DeleteFile struct{ Ctx Ctx }

func (DeleteFile) Name() string { return "delete_file" }

func (DeleteFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to delete.",
			},
		},
		"required": []string{"path"},
	}
}

func (d DeleteFile) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	abs, err := fsutil.Confine(d.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Sprintf("Error: stat %s: %v", in.Path, err), nil
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: %s is a directory; delete_file only removes files", in.Path), nil
	}
	// Record old state for /undo before deleting.
	if d.Ctx.UndoStack != nil {
		_ = d.Ctx.UndoStack.Push(abs)
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Sprintf("Error: remove %s: %v", in.Path, err), nil
	}
	return fmt.Sprintf("deleted %s", in.Path), nil
}
