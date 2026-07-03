// Package gitignore reads .gitignore files and provides pattern matching
// so tools (list_files, search, indexer) can respect them instead of using
// a hardcoded ignore list.
//
// Supports the common .gitignore patterns: exact names, wildcards (*),
// directory markers (trailing /), and negation (!). Does NOT support
// double-star (**) — that requires a full glob library. For most projects
// the simple pattern matching here is sufficient.
package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Matcher checks whether a relative path matches any .gitignore pattern.
type Matcher struct {
	patterns []pattern
}

type pattern struct {
	negative    bool
	dirOnly     bool
	match       string // the pattern text (simplified)
	segments    []string
	hasWildcard bool
}

// Load reads .gitignore from the given project root. If no .gitignore
// exists, returns an empty matcher (nothing ignored).
func Load(root string) *Matcher {
	m := &Matcher{}
	gitignorePath := filepath.Join(root, ".gitignore")
	f, err := os.Open(gitignorePath)
	if err != nil {
		return m
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.addPattern(line)
	}
	return m
}

func (m *Matcher) addPattern(line string) {
	p := pattern{}
	if strings.HasPrefix(line, "!") {
		p.negative = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	p.match = line
	p.segments = strings.Split(line, "/")
	p.hasWildcard = strings.ContainsAny(line, "*?")
	m.patterns = append(m.patterns, p)
}

// IsIgnored returns true if the relative path matches a .gitignore pattern
// (and is not un-ignored by a later negation).
func (m *Matcher) IsIgnored(relPath string, isDir bool) bool {
	relPath = filepath.ToSlash(relPath)
	ignored := false
	base := filepath.Base(relPath)

	for _, p := range m.patterns {
		// A dir-only pattern matches the directory itself OR any path inside it.
		if p.dirOnly && !isDir {
			// Check if the path is inside an ignored directory.
			if strings.Contains(relPath, p.match+"/") {
				if p.negative {
					ignored = false
				} else {
					ignored = true
				}
			}
			continue
		}
		var matched bool
		if p.hasWildcard {
			matched = wildcardMatch(p.match, relPath) || wildcardMatch(p.match, base)
		} else {
			matched = p.match == base || p.match == relPath || strings.Contains(relPath, p.match+"/")
		}
		if matched {
			if p.negative {
				ignored = false
			} else {
				ignored = true
			}
		}
	}
	return ignored
}

// wildcardMatch does simple glob matching (* and ?).
func wildcardMatch(pattern, name string) bool {
	// Simple implementation: split on *, match segments.
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	// First segment must be a prefix.
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	for _, part := range parts[1:] {
		idx := strings.Index(name, part)
		if idx < 0 {
			return false
		}
		name = name[idx+len(part):]
	}
	return true
}

// IgnoredDirs returns the set of directory names that are ignored by
// .gitignore. Used by the indexer and tools to skip entire directories
// during walks.
func (m *Matcher) IgnoredDirs() []string {
	var dirs []string
	for _, p := range m.patterns {
		if p.negative || p.hasWildcard {
			continue
		}
		// A bare name or name/ pattern means "ignore this directory everywhere".
		if len(p.segments) == 1 {
			dirs = append(dirs, p.match)
		}
	}
	return dirs
}
