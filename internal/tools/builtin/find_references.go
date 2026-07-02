package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/indexer"
)

// FindReferences finds all places a named symbol is used (the "ctrl+click on
// a reference" tool). Queries the live index.
type FindReferences struct {
	Index *indexer.Indexer
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
		},
		"required": []string{"symbol"},
	}
}

func (f FindReferences) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Symbol string `json:"symbol"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Symbol == "" {
		return "Error: symbol is required", nil
	}
	if f.Index == nil {
		return "Error: index not available (indexer not started)", nil
	}

	refs := f.Index.Index().LookupReferences(in.Symbol)
	if len(refs) == 0 {
		// Also show the definition so the user knows the symbol exists.
		defs := f.Index.Index().LookupDefinition(in.Symbol)
		if len(defs) == 0 {
			return fmt.Sprintf("no definition or references found for %q", in.Symbol), nil
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("no references found (defined at):\n"))
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
