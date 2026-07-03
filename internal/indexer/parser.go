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
	case ".kt", ".kts":
		return kotlinParser{}
	case ".java":
		return javaParser{}
	case ".cs":
		return csharpParser{}
	case ".php":
		return phpParser{}
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		return jsParser{}
	case ".css", ".scss", ".sass", ".less":
		return cssParser{}
	case ".html", ".htm":
		return htmlParser{}
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

// --- Kotlin ---

type kotlinParser struct{}

var (
	ktFunRe   = regexp.MustCompile(`^\s*(?:private|public|protected|internal|open|override|abstract|suspend|inline|operator|infix)?\s*fun\s+(?:<[^>]*>\s+)?(\w+)\s*\(`)
	ktClassRe = regexp.MustCompile(`^\s*(?:public|private|protected|internal|open|abstract|sealed|data|enum)?\s*(class|interface|object|enum\s+class)\s+(\w+)`)
	ktVarRe   = regexp.MustCompile(`^\s*(?:val|var)\s+(\w+)\s*[:=]`)
)

func (kotlinParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := ktFunRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "kotlin"})
			knownDefs[m[1]] = true
		}
		if m := ktClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: SymbolClass, File: relPath, Line: ln, Language: "kotlin"})
			knownDefs[m[2]] = true
		}
		if m := ktVarRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "kotlin"})
			knownDefs[m[1]] = true
		}
	}
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

// --- Java ---

type javaParser struct{}

var (
	javaClassRe  = regexp.MustCompile(`^\s*(?:public|private|protected|abstract|final|static)?\s*(class|interface|enum|record)\s+(\w+)`)
	javaMethodRe = regexp.MustCompile(`^\s*(?:public|private|protected|static|final|abstract|synchronized)?\s+(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)
	javaVarRe    = regexp.MustCompile(`^\s*(?:final\s+)?(?:[\w<>\[\]]+)\s+(\w+)\s*=`)
)

func (javaParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := javaClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: SymbolClass, File: relPath, Line: ln, Language: "java"})
			knownDefs[m[2]] = true
			continue
		}
		// Methods are tricky with regex; match lines that look like method declarations.
		// Skip lines that are actually control statements (if, for, while, switch, catch).
		if m := javaMethodRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			// Filter out common false positives.
			if name != "if" && name != "for" && name != "while" && name != "switch" && name != "catch" && name != "return" && name != "new" {
				symbols = append(symbols, Symbol{Name: name, Kind: SymbolMethod, File: relPath, Line: ln, Language: "java"})
				knownDefs[name] = true
			}
		}
	}
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

// --- JavaScript / TypeScript ---

type jsParser struct{}

var (
	jsFunRe   = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	jsClassRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`)
	jsArrowRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s*)?\(?.*?\)?\s*=>`)
	jsConstRe = regexp.MustCompile(`^\s*(?:export\s+)?const\s+(\w+)\s*=`)
	jsIntRe   = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+(\w+)`)
	jsTypeRe  = regexp.MustCompile(`^\s*(?:export\s+)?type\s+(\w+)\s*=`)
)

func (jsParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := jsFunRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "javascript"})
			knownDefs[m[1]] = true
			continue
		}
		if m := jsClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "javascript"})
			knownDefs[m[1]] = true
			continue
		}
		if m := jsArrowRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "javascript"})
			knownDefs[m[1]] = true
			continue
		}
		if m := jsIntRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "javascript"})
			knownDefs[m[1]] = true
			continue
		}
		if m := jsTypeRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "javascript"})
			knownDefs[m[1]] = true
			continue
		}
		// const declarations that aren't arrow functions.
		if m := jsConstRe.FindStringSubmatch(line); m != nil {
			if !knownDefs[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "javascript"})
				knownDefs[m[1]] = true
			}
		}
	}
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

// --- CSS / SCSS / SASS / LESS ---

type cssParser struct{}

var (
	cssClassRe = regexp.MustCompile(`\.([a-zA-Z_][\w-]*)`)
	cssIdRe    = regexp.MustCompile(`#([a-zA-Z][\w-]*)`)
	cssMixinRe = regexp.MustCompile(`@mixin\s+([\w-]+)`)
	cssIncRe   = regexp.MustCompile(`@include\s+([\w-]+)`)
	cssVarRe   = regexp.MustCompile(`^\s*(\$|--[\w-]+)\s*[:=]`)
)

func (cssParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		// SCSS mixins.
		if m := cssMixinRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "css"})
			knownDefs[m[1]] = true
		}
		// SCSS @include is a reference, not a definition.
		if m := cssIncRe.FindStringSubmatch(line); m != nil {
			references = append(references, Reference{Symbol: m[1], File: relPath, Line: ln})
		}
		// CSS class selectors.
		for _, m := range cssClassRe.FindAllStringSubmatch(line, -1) {
			name := m[1]
			if !knownDefs[name] {
				symbols = append(symbols, Symbol{Name: name, Kind: SymbolVariable, File: relPath, Line: ln, Language: "css"})
				knownDefs[name] = true
			}
		}
		// ID selectors.
		for _, m := range cssIdRe.FindAllStringSubmatch(line, -1) {
			if !knownDefs[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "css"})
				knownDefs[m[1]] = true
			}
		}
		// SCSS/CSS custom properties and SCSS variables.
		if m := cssVarRe.FindStringSubmatch(line); m != nil {
			if !knownDefs[m[1]] {
				symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "css"})
				knownDefs[m[1]] = true
			}
		}
	}
	// @include references: link to known mixin definitions.
	for i, line := range lines {
		if m := cssIncRe.FindStringSubmatch(line); m != nil {
			if knownDefs[m[1]] {
				// Already added above, avoid double counting.
				_ = i
			}
		}
	}
	return symbols, references
}

// --- HTML ---

type htmlParser struct{}

var (
	htmlIdRe    = regexp.MustCompile(`\bid=["']([^"']+)["']`)
	htmlClassRe = regexp.MustCompile(`\bclass=["']([^"']+)["']`)
	htmlTagOpen = regexp.MustCompile(`<(\w+)[\s>]`)
)

func (htmlParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		// IDs.
		for _, m := range htmlIdRe.FindAllStringSubmatch(line, -1) {
			name := "id:" + m[1]
			if !knownDefs[name] {
				symbols = append(symbols, Symbol{Name: name, Kind: SymbolVariable, File: relPath, Line: ln, Language: "html"})
				knownDefs[name] = true
			}
		}
		// Classes.
		for _, m := range htmlClassRe.FindAllStringSubmatch(line, -1) {
			for _, cls := range strings.Fields(m[1]) {
				name := "class:" + cls
				if !knownDefs[name] {
					symbols = append(symbols, Symbol{Name: name, Kind: SymbolVariable, File: relPath, Line: ln, Language: "html"})
					knownDefs[name] = true
				}
			}
		}
	}
	return symbols, nil
}

type csharpParser struct{}

var (
	csClassRe  = regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|sealed|abstract|partial)?\s*(?:class|interface|struct|record|enum)\s+(\w+)`)
	csMethodRe = regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|virtual|override|abstract|async|sealed)?\s+(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)
	csVarRe    = regexp.MustCompile(`^\s*(?:var|int|string|bool|double|float|decimal|long)\s+(\w+)\s*=`)
)

func (csharpParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := csClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolClass, File: relPath, Line: ln, Language: "csharp"})
			knownDefs[m[1]] = true
			continue
		}
		if m := csMethodRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			if name != "if" && name != "for" && name != "foreach" && name != "while" && name != "switch" && name != "catch" && name != "using" && name != "return" {
				symbols = append(symbols, Symbol{Name: name, Kind: SymbolMethod, File: relPath, Line: ln, Language: "csharp"})
				knownDefs[name] = true
			}
		}
		if m := csVarRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "csharp"})
			knownDefs[m[1]] = true
		}
	}
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

// --- PHP ---

type phpParser struct{}

var (
	phpFunRe   = regexp.MustCompile(`^\s*(?:public|private|protected|static|final|abstract)?\s*function\s+(\w+)\s*\(`)
	phpClassRe = regexp.MustCompile(`^\s*(?:final|abstract)?\s*(class|interface|trait|enum)\s+(\w+)`)
	phpVarRe   = regexp.MustCompile(`^\s*(?:public|private|protected|static)?\s*\$(\w+)\s*=`)
	phpConstRe = regexp.MustCompile(`^\s*const\s+(\w+)\s*=`)
)

func (phpParser) Parse(source, relPath string) ([]Symbol, []Reference) {
	var symbols []Symbol
	var references []Reference
	lines := strings.Split(source, "\n")
	knownDefs := map[string]bool{}

	for i, line := range lines {
		ln := i + 1
		if m := phpFunRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolFunction, File: relPath, Line: ln, Language: "php"})
			knownDefs[m[1]] = true
		}
		if m := phpClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: SymbolClass, File: relPath, Line: ln, Language: "php"})
			knownDefs[m[2]] = true
		}
		if m := phpVarRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: "$" + m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "php"})
			knownDefs["$"+m[1]] = true
		}
		if m := phpConstRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: SymbolVariable, File: relPath, Line: ln, Language: "php"})
			knownDefs[m[1]] = true
		}
	}
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
