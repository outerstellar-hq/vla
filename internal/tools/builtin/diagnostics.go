package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abrandt/vla/internal/fsutil"
	"github.com/abrandt/vla/internal/lsp"
)

// Diagnostics returns lint/type errors for a file. Opens the file in the LSP
// server (didOpen), waits for publishDiagnostics, and returns them. This is
// the "red squiggles" from an IDE, as a tool.
type Diagnostics struct {
	Manager *lsp.Manager
	BaseDir string
}

func (Diagnostics) Name() string { return "diagnostics" }

func (Diagnostics) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "Path to the file to check.",
			},
		},
		"required": []string{"file"},
	}
}

func (d Diagnostics) Execute(args json.RawMessage) (string, error) {
	var in struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.File == "" {
		return "Error: file is required", nil
	}
	if d.Manager == nil {
		return "Error: LSP manager not available (diagnostics requires a language server)", nil
	}

	abs, err := fsutil.Confine(d.BaseDir, in.File)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	lang := lsp.InferLanguage(d.BaseDir)
	if lang == "" {
		return "Error: could not infer language for this project", nil
	}
	client, err := d.Manager.Get(lang, d.BaseDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	uri := pathToURIString(abs)

	// Read file content for didOpen.
	content, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Sprintf("Error: read file: %v", err), nil
	}

	// Clear any previous diagnostics for this URI.
	// (We can't clear, but the next didOpen will trigger a fresh publish.)

	_ = client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": string(lang),
			"version":    1,
			"text":       string(content),
		},
	})

	// Poll for diagnostics (LSP pushes them asynchronously).
	// The client buffers them; wait up to 3 seconds.
	var diags json.RawMessage
	for i := 0; i < 30; i++ {
		diags = client.GetDiagnostics(uri)
		if diags != nil {
			break
		}
		sleep100ms()
	}

	if diags == nil || string(diags) == "null" || string(diags) == "[]" {
		return "no diagnostics (file is clean)", nil
	}

	var items []struct {
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
		Severity int    `json:"severity"`
		Message  string `json:"message"`
		Source   string `json:"source"`
	}
	if err := json.Unmarshal(diags, &items); err != nil {
		return string(diags), nil // return raw if parse fails
	}
	if len(items) == 0 {
		return "no diagnostics (file is clean)", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d diagnostic", len(items))
	if len(items) != 1 {
		b.WriteString("s")
	}
	b.WriteString(":\n")
	for _, item := range items {
		severity := severityName(item.Severity)
		loc := fmt.Sprintf("line %d:%d", item.Range.Start.Line+1, item.Range.Start.Character+1)
		if item.Source != "" {
			fmt.Fprintf(&b, "  [%s] %s %s: %s\n", severity, item.Source, loc, item.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", severity, loc, item.Message)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func severityName(s int) string {
	switch s {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return fmt.Sprintf("sev%d", s)
	}
}

func sleep100ms() {
	time.Sleep(100 * time.Millisecond)
}
