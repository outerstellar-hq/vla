package indexer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
// Phase 1: parse all files for definitions. Phase 2: re-scan all files for
// references to any known symbol (cross-file reference resolution).
// Returns the number of files indexed.
func (ix *Indexer) Build() (int, error) {
	count := 0
	type fileEntry struct {
		absPath string
		relPath string
		source  string
	}
	var files []fileEntry

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
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel := ix.rel(path)
		files = append(files, fileEntry{absPath: path, relPath: rel, source: string(data)})
		count++
		return nil
	})
	if err != nil {
		return count, err
	}

	// Phase 1: extract and store all definitions.
	allDefNames := map[string]bool{}
	for _, f := range files {
		symbols, _ := ParseFile(f.source, f.relPath)
		for _, s := range symbols {
			ix.index.AddSymbol(s)
			allDefNames[s.Name] = true
		}
	}

	// Phase 2: find references to any known symbol in every file.
	defSnapshot := ix.index.SymbolsSnapshot()
	for _, f := range files {
		refs := findReferences(f.source, f.relPath, allDefNames, defSnapshot)
		for _, r := range refs {
			ix.index.AddReference(r)
		}
	}

	return count, nil
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
