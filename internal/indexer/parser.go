package indexer

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Parser extracts symbols and references from a single source file. Each
// language implements this; the indexer dispatches to the right one based
// on file extension.
type Parser interface {
	// Parse scans source and returns the symbols defined in it and the
	// references (usages) found. relPath is the project-relative path,
	// used to populate Symbol.File / Reference.File.
	Parse(source string, relPath string) ([]Symbol, []Reference)
}

// parserFor returns the Parser for a file extension, or nil if unsupported.
func parserFor(ext string) Parser {
	switch strings.ToLower(ext) {
	case ".py":
		return pythonParser{}
	case ".go":
		return goParser{}
	default:
		return nil
	}
}

// ParseFile dispatches to the right parser based on extension. Returns
// nil, nil if the file type is unsupported.
func ParseFile(source, relPath string) ([]Symbol, []Reference) {
	p := parserFor(filepath.Ext(relPath))
	if p == nil {
		return nil, nil
	}
	return p.Parse(source, relPath)
}

// --- Python ---

type pythonParser struct{}

var (
	// Matches: def func_name(params):
	pyFuncRe = regexp.MustCompile(`^\s*def\s+(\w+)\s*\(`)
	// Matches: class ClassName(...) or class ClassName:
	pyClassRe = regexp.MustCompile(`^\s*class\s+(\w+)\s*[\(:]`)
	// Matches: async def func_name(params):
	pyAsyncFuncRe = regexp.MustCompile(`^\s*async\s+def\s+(\w+)\s*\(`)
)

func (pythonParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1

		// Definitions.
		if m := pyAsyncFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "python"})
			knownDefs[m[1]] = true
			continue
		}
		if m := pyFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "python"})
			knownDefs[m[1]] = true
			continue
		}
		if m := pyClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "python"})
			knownDefs[m[1]] = true
			continue
		}
	}

	// References: any identifier that matches a known def name and is NOT
	// on the def line itself. This is conservative — it will catch calls,
	// attribute access, and type annotations.
	for i, line := range lines {
		for name := range knownDefs {
			if containsWord(line, name) {
				// Check it's not the definition line.
				isDefLine := false
				for _, s := range symbols {
					if s.Name == name && s.Line == i+1 {
						isDefLine = true
						break
					}
				}
				if !isDefLine {
					references = append(references, Reference{Symbol: name, File: relPath, Line: i + 1})
				}
			}
		}
	}

	return symbols, references
}

// --- Go ---

type goParser struct{}

var (
	goFuncRe  = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(`)
	goTypeRe  = regexp.MustCompile(`^type\s+(\w+)\s+`)
	goVarRe   = regexp.MustCompile(`^var\s+(\w+)\s+`)
	goConstRe = regexp.MustCompile(`^const\s+(\w+)\s+`)
)

func (goParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := goFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "go"})
			knownDefs[m[1]] = true
		}
		if m := goTypeRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "go"})
			knownDefs[m[1]] = true
		}
		if m := goVarRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "go"})
			knownDefs[m[1]] = true
		}
		if m := goConstRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "go"})
			knownDefs[m[1]] = true
		}
	}

	// References.
	for i, line := range lines {
		for name := range knownDefs {
			if containsWord(line, name) {
				isDefLine := false
				for _, s := range symbols {
					if s.Name == name && s.Line == i+1 {
						isDefLine = true
						break
					}
				}
				if !isDefLine {
					references = append(references, Reference{Symbol: name, File: relPath, Line: i + 1})
				}
			}
		}
	}

	return symbols, references
}

// containsWord reports whether s contains name as a whole word (not a
// substring of a larger identifier).
func containsWord(s, name string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], name)
		if pos < 0 {
			return false
		}
		pos += idx
		// Check boundaries.
		before := pos == 0 || !isIdentChar(s[pos-1])
		after := pos+len(name) == len(s) || !isIdentChar(s[pos+len(name)])
		if before && after {
			return true
		}
		idx = pos + len(name)
	}
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}
