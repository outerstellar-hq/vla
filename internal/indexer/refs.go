package indexer

import (
	"strings"
)

// findReferences scans source for usages of any name in knownDefNames,
// excluding the lines where those names are defined (to avoid counting
// definitions as references). defLines maps symbol name → set of line
// numbers where it's defined (to skip those).
func findReferences(source, relPath string, knownDefNames map[string]bool, defMap map[string][]Symbol) []Reference {
	if len(knownDefNames) == 0 {
		return nil
	}
	var refs []Reference
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		ln := i + 1
		for name := range knownDefNames {
			if !containsWord(line, name) {
				continue
			}
			// Skip if this line IS a definition line for this name.
			if isDefLine(defMap, name, relPath, ln) {
				continue
			}
			refs = append(refs, Reference{Symbol: name, File: relPath, Line: ln})
		}
	}
	return refs
}

// isDefLine checks whether (file, line) is a definition site for name.
func isDefLine(defMap map[string][]Symbol, name, file string, line int) bool {
	for _, s := range defMap[name] {
		if s.File == file && s.Line == line {
			return true
		}
	}
	return false
}
