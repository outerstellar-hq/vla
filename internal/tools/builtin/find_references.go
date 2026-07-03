package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/lsp"
)

// FindReferences finds all places a named symbol is used. Tries LSP first
// (if available), falls back to the regex indexer.
type FindReferences struct {
	Index   *indexer.Indexer
	Manager *lsp.Manager // optional; nil = regex index only
	BaseDir string
}

func (FindReferences) Name() string { return "find_references" }

func (FindReferences) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"symbol": map[string]any{
				"type":        "string",
				"description": "The name of the symbol to find references to.",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Optional: file where the symbol is referenced (for LSP).",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "Optional: 1-based line number (for LSP).",
			},
			"column": map[string]any{
				"type":        "integer",
				"description": "Optional: 1-based column (for LSP).",
			},
		},
		"required": []string{"symbol"},
	}
}

func (f FindReferences) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Symbol string `json:"symbol"`
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Symbol == "" {
		return "Error: symbol is required", nil
	}

	// Try LSP first if a position is given and a manager is available.
	if f.Manager != nil && in.File != "" && in.Line > 0 {
		out, err := lspReferences(f.Manager, f.BaseDir, in.File, in.Line, in.Column)
		if err == nil && out != "" {
			return out, nil
		}
	}

	// Fall back to regex indexer.
	if f.Index == nil {
		return "Error: index not available (indexer not started)", nil
	}
	refs := f.Index.Index().LookupReferences(in.Symbol)
	if len(refs) == 0 {
		defs := f.Index.Index().LookupDefinition(in.Symbol)
		if len(defs) == 0 {
			return fmt.Sprintf("no definition or references found for %q", in.Symbol), nil
		}
		var b strings.Builder
		b.WriteString("no references found (defined at):\n")
		for _, d := range defs {
			fmt.Fprintf(&b, "  %s %s — %s:%d", d.Kind, d.Name, d.File, d.Line)
		}
		return b.String(), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d reference", len(refs))
	if len(refs) != 1 {
		b.WriteString("s")
	}
	b.WriteString(":\n")
	for _, r := range refs {
		fmt.Fprintf(&b, "  %s:%d\n", r.File, r.Line)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func lspReferences(mgr *lsp.Manager, baseDir, file string, line, column int) (string, error) {
	lang := lsp.InferLanguage(baseDir)
	if lang == "" {
		return "", fmt.Errorf("could not infer language")
	}
	client, err := mgr.Get(lang, baseDir)
	if err != nil {
		return "", err
	}
	absPath := file
	if !filepathIsAbs(file) {
		absPath = joinPath(baseDir, file)
	}
	uri := pathToURIString(absPath)

	result, err := client.Request("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": column - 1},
		"context":      map[string]any{"includeDeclaration": true},
	})
	if err != nil {
		return "", err
	}
	return formatLSPReferences(result, baseDir), nil
}

func formatLSPReferences(raw json.RawMessage, baseDir string) string {
	var locs []struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	}
	if err := json.Unmarshal(raw, &locs); err != nil {
		return ""
	}
	if len(locs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d references (LSP):\n", len(locs))
	for _, loc := range locs {
		relPath := uriToRelPath(loc.URI, baseDir)
		fmt.Fprintf(&b, "  %s:%d\n", relPath, loc.Range.Start.Line+1)
	}
	return strings.TrimRight(b.String(), "\n")
}
