package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/abrandt/vla/internal/fsutil"
)

// UpdateFile applies a find-and-replace within a file. The LLM provides
// old_string (must match uniquely, or use replace_all) and new_string.
// Confined to BaseDir.
type UpdateFile struct{ Ctx Ctx }

func (UpdateFile) Name() string { return "update_file" }

func (UpdateFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to update.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The exact text to find in the file. Must be unique unless replace_all is true.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The text to replace old_string with.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "If true, replace all occurrences. If false (default), old_string must match exactly once.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (u UpdateFile) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	abs, err := fsutil.Confine(u.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Sprintf("Error: read %s: %v", in.Path, err), nil
	}

	// Record old state for /undo before modifying.
	if u.Ctx.UndoStack != nil {
		_ = u.Ctx.UndoStack.Push(abs)
	}

	count := bytes.Count(data, []byte(in.OldString))
	if count == 0 {
		return fmt.Sprintf("Error: old_string not found in %s", in.Path), nil
	}
	if count > 1 && !in.ReplaceAll {
		return fmt.Sprintf("Error: old_string matches %d times in %s; pass \"replace_all\": true or make it unique", count, in.Path), nil
	}

	var newData []byte
	if in.ReplaceAll {
		newData = bytes.ReplaceAll(data, []byte(in.OldString), []byte(in.NewString))
	} else {
		// Single match guaranteed by the count check above.
		newData = bytes.Replace(data, []byte(in.OldString), []byte(in.NewString), 1)
	}
	if err := os.WriteFile(abs, newData, 0644); err != nil {
		return fmt.Sprintf("Error: write %s: %v", in.Path, err), nil
	}
	summary := fmt.Sprintf("updated %s (%d replacement", in.Path, count)
	if count != 1 {
		summary += "s"
	}
	summary += ")"
	if in.ReplaceAll {
		summary += " [replace_all]"
	}
	// Show a short diff preview.
	result := summary + "\n" + diffPreview(in.OldString, in.NewString)

	// Post-edit check: warn about swallowed errors in the updated code.
	if isSourceCode(in.Path) {
		warnings := CheckErrorHandling(in.Path, string(newData))
		result += FormatWarnings(warnings)
	}

	return result, nil
}

// diffPreview returns a terse 3-line preview of the change.
func diffPreview(old, new string) string {
	oldLine := firstLine(old)
	newLine := firstLine(new)
	if oldLine == newLine {
		// Multi-line or trailing-only change; just show lengths.
		return fmt.Sprintf("- (%d chars)\n+ (%d chars)", len(old), len(new))
	}
	return fmt.Sprintf("- %s\n+ %s", oldLine, newLine)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
