package builtin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/abrandt/vla/internal/fsutil"
)

// ReadFile reads a file's contents, confined to BaseDir. Capped at
// MaxReadBytes to protect the context window.
type ReadFile struct{ Ctx Ctx }

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read. Relative to project root, or absolute within it.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Optional byte offset to start reading from. Default 0.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional max bytes to read. Default and cap is 256 KiB.",
			},
		},
		"required": []string{"path"},
	}
}

func (r ReadFile) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Path   string `json:"path"`
		Offset int64  `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	abs, err := fsutil.Confine(r.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if in.Limit <= 0 || in.Limit > fsutil.MaxReadBytes {
		in.Limit = fsutil.MaxReadBytes
	}
	f, err := os.Open(abs)
	if err != nil {
		return fmt.Sprintf("Error: open %s: %v", in.Path, err), nil
	}
	defer f.Close()
	if in.Offset > 0 {
		if _, err := f.Seek(in.Offset, 0); err != nil {
			return fmt.Sprintf("Error: seek %s: %v", in.Path, err), nil
		}
	}
	buf := make([]byte, in.Limit)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		// A real read error (not just EOF). Partial reads are still useful,
		// so only error if we got zero bytes.
		if n == 0 {
			return fmt.Sprintf("Error: read %s: %v", in.Path, err), nil
		}
	}
	if n == in.Limit {
		// We read exactly the cap — check if there's more, to signal truncation.
		fi, _ := f.Stat()
		if fi.Size() > in.Offset+int64(n) {
			return string(buf[:n]) + fmt.Sprintf("\n...[truncated: %d more bytes, use offset to continue]", fi.Size()-in.Offset-int64(n)), nil
		}
	}
	return string(buf[:n]), nil
}
