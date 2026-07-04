// Package indexer implements VLA's background symbol index. It parses source
// files with language-specific regex parsers to extract symbol definitions
// (functions, classes, types, variables) and cross-file references. The
// index is used for the go-to-definition and find-references tools as a
// fallback when an LSP server is not available.
//
// The indexer supports 9 languages: Go, Python, Kotlin, Java, C#, PHP,
// JavaScript/TypeScript, CSS/SCSS, and HTML. Each language has a Parser
// implementation in its own file (*_parser.go).
//
// The index is built in parallel (4-goroutine worker pool) and updated by
// a polling watcher that rescans every 5 seconds (stdlib-only, no fsnotify).
package indexer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Indexer walks a project tree, parses each supported source file, and
// populates an Index. It does NOT run in the background by itself — that's
// the job of the Watcher (see watcher.go). The Indexer is the synchronous
// "index everything now" entry point, used for the initial build and tests.
type Indexer struct {
	root  string // absolute path to project root
	index *Index
}

// New creates an Indexer for the given project root.
func New(root string) *Indexer {
	return &Indexer{root: root, index: NewIndex()}
}

// Index returns the underlying index (populated after Build/ReindexFile).
func (ix *Indexer) Index() *Index { return ix.index }

// Root returns the project root.
func (ix *Indexer) Root() string { return ix.root }

// Build walks the entire project tree and indexes every supported file.
// Uses a worker pool (4 goroutines) for parallel file parsing — significantly
// faster on large codebases.
// Phase 1: parse all files for definitions (parallel).
// Phase 2: re-scan all files for references (parallel).
// Returns the number of files indexed.
func (ix *Indexer) Build() (int, error) {
	type fileEntry struct {
		relPath string
		source  string
	}
	var paths []string

	// Walk: collect supported file paths.
	err := filepath.WalkDir(ix.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel := ix.rel(path)
			if isIgnoredDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if parserFor(ext) == nil {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Phase 1: parse all files in parallel for definitions.
	type parseResult struct {
		symbols []Symbol
		refs    []Reference // unused in phase 1
		relPath string
		source  string
	}
	results := make(chan parseResult, len(paths))
	jobs := make(chan string, len(paths))
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	const workers = 4
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for absPath := range jobs {
				data, err := os.ReadFile(absPath)
				if err != nil {
					continue
				}
				rel := ix.rel(absPath)
				symbols, _ := ParseFile(string(data), rel)
				results <- parseResult{symbols: symbols, relPath: rel, source: string(data)}
			}
		}()
	}
	wg.Wait()
	close(results)

	// Collect results: store definitions and keep source for phase 2.
	allDefNames := map[string]bool{}
	var files []fileEntry
	for r := range results {
		for _, s := range r.symbols {
			ix.index.AddSymbol(s)
			allDefNames[s.Name] = true
		}
		files = append(files, fileEntry{relPath: r.relPath, source: r.source})
	}

	// Phase 2: find references (parallel).
	defSnapshot := ix.index.SymbolsSnapshot()
	refResults := make(chan []Reference, len(files))
	refJobs := make(chan fileEntry, len(files))
	for _, f := range files {
		refJobs <- f
	}
	close(refJobs)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range refJobs {
				refs := findReferences(f.source, f.relPath, allDefNames, defSnapshot)
				refResults <- refs
			}
		}()
	}
	wg.Wait()
	close(refResults)

	for refs := range refResults {
		for _, r := range refs {
			ix.index.AddReference(r)
		}
	}

	return len(paths), nil
}

// ReindexFile clears and re-indexes a single file. Used by the Watcher on
// file change. path may be absolute or relative to root.
func (ix *Indexer) ReindexFile(path string) error {
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(ix.root, path)
	}
	rel := ix.rel(abs)
	ix.index.ClearFile(rel)
	return ix.indexFile(abs)
}

// indexFile reads and parses one file, adding its symbols and references
// to the index. References are found by scanning for any known symbol name.
func (ix *Indexer) indexFile(absPath string) error {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	rel := ix.rel(absPath)
	source := string(data)
	symbols, _ := ParseFile(source, rel)
	for _, s := range symbols {
		ix.index.AddSymbol(s)
	}
	// Find references to all known symbols (cross-file).
	defNames := make(map[string]bool, len(ix.index.symbols))
	for name := range ix.index.SymbolsSnapshot() {
		defNames[name] = true
	}
	defSnapshot := ix.index.SymbolsSnapshot()
	refs := findReferences(source, rel, defNames, defSnapshot)
	for _, r := range refs {
		ix.index.AddReference(r)
	}
	return nil
}

// rel converts an absolute path to project-relative (forward-slash).
func (ix *Indexer) rel(absPath string) string {
	rel, err := filepath.Rel(ix.root, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// isIgnoredDir mirrors builtin.ignoredDirNames without the import cycle.
func isIgnoredDir(rel string) bool {
	if rel == "." {
		return false
	}
	base := filepath.Base(rel)
	for _, d := range []string{
		".git", "node_modules", "vendor", "__pycache__", ".venv", "venv",
		"dist", "build", "target", ".idea", ".vscode", ".cache",
	} {
		if base == d {
			return true
		}
	}
	return false
}
