package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/indexer"
)

// GoToDefinition resolves where a named symbol is defined (the "ctrl+click"
// tool). It queries the live index maintained by the background indexer.
// If multiple definitions exist, all are returned.
type GoToDefinition struct {
	Index *indexer.Indexer
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
		},
		"required": []string{"symbol"},
	}
}

func (g GoToDefinition) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Symbol string `json:"symbol"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Symbol == "" {
		return "Error: symbol is required", nil
	}
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
