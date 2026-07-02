// Package indexer maintains a live in-memory index of symbols (definitions)
// and references in a codebase. It is the backbone of VLA's navigation tools
// (go-to-definition, find-references) — the "live index" differentiator.
//
// The index is populated by a Parser, which is language-specific. The
// prototype ships with a regex-based parser for Python and Go; the
// architecture allows swapping in a tree-sitter parser later without
// changing the Index API or the navigation tools.
package indexer

import "sync"

// SymbolKind classifies what a symbol definition is.
type SymbolKind string

const (
	SymbolFunction  SymbolKind = "function"
	SymbolClass     SymbolKind = "class"
	SymbolMethod    SymbolKind = "method"
	SymbolVariable  SymbolKind = "variable"
	SymbolImport    SymbolKind = "import"
)

// Symbol is a definition found in the codebase: a function, class, method,
// etc. It's what go-to-definition resolves TO.
type Symbol struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	File     string     `json:"file"`     // project-relative path
	Line     int        `json:"line"`     // 1-based line where defined
	Language string     `json:"language"` // "python", "go", etc.
}

// Reference is a usage of a symbol somewhere in the codebase.
type Reference struct {
	Symbol string `json:"symbol"` // the name being referenced
	File   string `json:"file"`
	Line   int    `json:"line"` // 1-based
}

// Index is the in-memory symbol/reference database. It is safe for
// concurrent read access. Writes happen during indexing (single goroutine).
type Index struct {
	mu        sync.RWMutex
	symbols   map[string][]Symbol    // keyed by symbol name
	byFile    map[string][]Symbol    // keyed by file path
	references map[string][]Reference // keyed by symbol name
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{
		symbols:    make(map[string][]Symbol),
		byFile:     make(map[string][]Symbol),
		references: make(map[string][]Reference),
	}
}

// AddSymbol records a definition. Safe to call from multiple goroutines
// (used by the background indexer's per-file parsing).
func (idx *Index) AddSymbol(s Symbol) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.symbols[s.Name] = append(idx.symbols[s.Name], s)
	idx.byFile[s.File] = append(idx.byFile[s.File], s)
}

// AddReference records a usage. Safe to call from multiple goroutines.
func (idx *Index) AddReference(r Reference) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.references[r.Symbol] = append(idx.references[r.Symbol], r)
}

// ClearFile removes all symbols and references associated with a file.
// Called before re-indexing a changed file.
func (idx *Index) ClearFile(file string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	// Remove from byFile and symbols.
	if syms, ok := idx.byFile[file]; ok {
		for _, s := range syms {
			filtered := idx.symbols[s.Name][:0]
			for _, existing := range idx.symbols[s.Name] {
				if existing.File != file {
					filtered = append(filtered, existing)
				}
			}
			if len(filtered) == 0 {
				delete(idx.symbols, s.Name)
			} else {
				idx.symbols[s.Name] = filtered
			}
		}
		delete(idx.byFile, file)
	}
	// Remove references where File matches.
	for name, refs := range idx.references {
		filtered := refs[:0]
		for _, r := range refs {
			if r.File != file {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(idx.references, name)
		} else {
			idx.references[name] = filtered
		}
	}
}

// LookupDefinition returns all symbols with the given name. A name may have
// multiple definitions (different files, or overloads).
func (idx *Index) LookupDefinition(name string) []Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]Symbol, len(idx.symbols[name]))
	copy(out, idx.symbols[name])
	return out
}

// LookupReferences returns all known usages of the named symbol.
func (idx *Index) LookupReferences(name string) []Reference {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]Reference, len(idx.references[name]))
	copy(out, idx.references[name])
	return out
}

// AllSymbols returns every indexed symbol (used for "list all symbols" /
// workspace outline). Mostly for debugging and outline tools.
func (idx *Index) AllSymbols() []Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []Symbol
	for _, syms := range idx.symbols {
		out = append(out, syms...)
	}
	return out
}

// SymbolCount returns the total number of indexed definitions.
func (idx *Index) SymbolCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	total := 0
	for _, syms := range idx.symbols {
		total += len(syms)
	}
	return total
}

// SymbolsSnapshot returns a copy of the internal symbol map (name → defs).
// Used by the indexer's reference-finding phase to avoid holding the lock
// while scanning many files.
func (idx *Index) SymbolsSnapshot() map[string][]Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make(map[string][]Symbol, len(idx.symbols))
	for k, v := range idx.symbols {
		cp := make([]Symbol, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
