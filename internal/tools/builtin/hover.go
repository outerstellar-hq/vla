package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/lsp"
)

// Hover returns type/signature/documentation for the symbol at a position.
// This is the LSP textDocument/hover operation — what you see when you hover
// over a symbol in an IDE. Requires an LSP server for the project's language.
type Hover struct {
	Manager *lsp.Manager
	BaseDir string
}

func (Hover) Name() string { return "hover" }

func (Hover) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "Path to the file.",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "1-based line number.",
			},
			"column": map[string]any{
				"type":        "integer",
				"description": "1-based column number.",
			},
		},
		"required": []string{"file", "line"},
	}
}

func (h Hover) Execute(args json.RawMessage) (string, error) {
	var in struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.File == "" {
		return "Error: file is required", nil
	}
	if in.Line <= 0 {
		return "Error: line is required (1-based)", nil
	}
	if h.Manager == nil {
		return "Error: LSP manager not available (hover requires a language server)", nil
	}

	lang := lsp.InferLanguage(h.BaseDir)
	if lang == "" {
		return "Error: could not infer language for this project", nil
	}
	client, err := h.Manager.Get(lang, h.BaseDir)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	absPath := in.File
	if !filepathIsAbs(in.File) {
		absPath = joinPath(h.BaseDir, in.File)
	}
	uri := pathToURIString(absPath)

	// Ensure the server knows about the file.
	_ = client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": string(lang),
			"version":    1,
		},
	})

	result, err := client.Request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": in.Line - 1, "character": in.Column - 1},
	})
	if err != nil {
		return fmt.Sprintf("Error: hover: %v", err), nil
	}

	var hover struct {
		Contents struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil || hover.Contents.Value == "" {
		return "no hover information available at this position", nil
	}
	return strings.TrimSpace(hover.Contents.Value), nil
}
