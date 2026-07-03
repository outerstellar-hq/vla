package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/lsp"
)

// GoToDefinition resolves where a named symbol is defined (the "ctrl+click"
// tool). It tries the LSP server first (if available for the project's
// language), then falls back to the regex-based indexer.
type GoToDefinition struct {
	Index   *indexer.Indexer
	Manager *lsp.Manager // optional; nil = regex index only
	BaseDir string
}

func (GoToDefinition) Name() string { return "go_to_definition" }

func (GoToDefinition) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"symbol": map[string]any{
				"type":        "string",
				"description": "The name of the symbol to find the definition of (function, class, method, variable).",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Optional: the file where the symbol is referenced (for LSP position-based lookup).",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "Optional: 1-based line number in the file (for LSP).",
			},
			"column": map[string]any{
				"type":        "integer",
				"description": "Optional: 1-based column number (for LSP).",
			},
		},
		"required": []string{"symbol"},
	}
}

func (g GoToDefinition) Execute(args json.RawMessage) (string, error) {
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
	if g.Manager != nil && in.File != "" && in.Line > 0 {
		out, err := lspDefinition(g.Manager, g.BaseDir, in.File, in.Line, in.Column)
		if err == nil && out != "" {
			return out, nil
		}
		// Fall through to regex index on LSP failure.
	}

	// Fall back to the regex indexer.
	if g.Index == nil {
		return "Error: index not available (indexer not started)", nil
	}
	defs := g.Index.Index().LookupDefinition(in.Symbol)
	if len(defs) == 0 {
		return fmt.Sprintf("no definition found for %q", in.Symbol), nil
	}

	var b strings.Builder
	for i, d := range defs {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s %s — %s:%d (%s)", d.Kind, d.Name, d.File, d.Line, d.Language)
	}
	return b.String(), nil
}

// lspDefinition queries the LSP server for textDocument/definition.
func lspDefinition(mgr *lsp.Manager, baseDir, file string, line, column int) (string, error) {
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

	// LSP requires didOpen before definition.
	_ = client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": string(lang),
			"version":    1,
		},
	})

	result, err := client.Request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": column - 1},
	})
	if err != nil {
		return "", err
	}
	return formatLSPDefinition(result, baseDir), nil
}

// formatLSPDefinition renders the LSP definition response (may be a single
// Location or an array).
func formatLSPDefinition(raw json.RawMessage, baseDir string) string {
	// Try array first.
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
		// Try single object.
		var loc struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		}
		if err := json.Unmarshal(raw, &loc); err != nil {
			return ""
		}
		locs = []struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		}{loc}
	}
	var b strings.Builder
	for i, loc := range locs {
		if i > 0 {
			b.WriteString("\n")
		}
		relPath := uriToRelPath(loc.URI, baseDir)
		fmt.Fprintf(&b, "%s:%d:%d", relPath, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}
	return b.String()
}
