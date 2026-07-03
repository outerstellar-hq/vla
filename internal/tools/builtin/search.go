package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/abrandt/vla/internal/fsutil"
	"github.com/abrandt/vla/internal/gitignore"
)

// Search does a codebase-wide text search (the "ctrl+f" tool). If ripgrep
// (rg) is on PATH it delegates to rg for speed; otherwise it falls back to
// a pure-Go recursive grep. Results are returned as "path:line: match".
// Confined to BaseDir.
type Search struct{ Ctx Ctx }

func (Search) Name() string { return "search" }

func (Search) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The text or pattern to search for.",
			},
			"regex": map[string]any{
				"type":        "boolean",
				"description": "If true, treat pattern as a regular expression. Default false (literal match).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional directory to scope the search to (relative to project root). Default: whole project.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Optional cap on number of matches. Default 100.",
			},
		},
		"required": []string{"pattern"},
	}
}

func (s Search) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Pattern    string `json:"pattern"`
		Regex      bool   `json:"regex"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Pattern == "" {
		return "Error: pattern is required", nil
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 100
	}

	abs, err := fsutil.Confine(s.Ctx.BaseDir, in.Path)
	if err != nil {
		return "Error: " + err.Error(), nil
	}

	var results []string
	if _, err := exec.LookPath("rg"); err == nil {
		results, err = searchWithRipgrep(abs, in.Pattern, in.Regex, in.MaxResults)
	} else {
		results, err = searchNative(s.Ctx.BaseDir, abs, in.Pattern, in.Regex, in.MaxResults)
	}
	if err != nil {
		return fmt.Sprintf("Error: search: %v", err), nil
	}
	if len(results) == 0 {
		return "no matches found", nil
	}
	// Normalize paths to project-relative (forward-slash) for stable output
	// regardless of which backend produced them.
	for i, r := range results {
		results[i] = relativizeResult(s.Ctx.BaseDir, r)
	}
	return strings.Join(results, "\n"), nil
}

// relativizeResult converts the path portion of "path:line:match" from
// absolute to relative-to-base, with forward slashes. Handles both rg's
// format (path:line:match, no space) and our native format.
// The tricky part: on Windows the path itself contains colons (C:\...).
// Strategy: the match is after the last colon, the line number is after
// the second-to-last colon. Everything before that is the path.
func relativizeResult(baseDir, result string) string {
	// Walk from the end: split off "match" (after last ':'), then "line"
	// (after next ':'). The rest is the path.
	lastColon := strings.LastIndexByte(result, ':')
	if lastColon < 0 {
		return result
	}
	match := result[lastColon+1:]
	rest := result[:lastColon]
	secondColon := strings.LastIndexByte(rest, ':')
	if secondColon < 0 {
		return result
	}
	lineNum := rest[secondColon+1:]
	absPath := rest[:secondColon]

	rel, err := filepath.Rel(baseDir, absPath)
	if err != nil {
		rel = absPath
	}
	return filepath.ToSlash(rel) + ":" + lineNum + ":" + match
}

// searchWithRipgrep runs `rg --line-number --no-heading --color never`.
func searchWithRipgrep(dir, pattern string, regex bool, max int) ([]string, error) {
	args := []string{"--line-number", "--no-heading", "--color", "never"}
	if !regex {
		args = append(args, "--fixed-strings")
	}
	// Exclude the same noise directories list_files ignores.
	for _, d := range ignoredDirNames {
		args = append(args, "-g", "!**/"+d+"/**")
	}
	args = append(args, "-m", fmt.Sprintf("%d", max), pattern, dir)
	cmd := exec.Command("rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// rg exits with 1 when no matches found — not an error for us.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg: %s", bytes.TrimSpace(stderr.Bytes()))
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

// searchNative is the pure-Go fallback: walks the tree (respecting the same
// ignore rules as list_files) and does a literal or regex substring search.
func searchNative(baseDir, dir, pattern string, regex bool, max int) ([]string, error) {
	var re *regexp.Regexp
	if regex {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
	}

	var results []string
	gi := gitignore.Load(baseDir)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if ignoredDir(relSlash) || gi.IsIgnored(relSlash, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if gi.IsIgnored(relSlash, false) {
			return nil
		}
		if isBinaryExt(filepath.Ext(path)) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable
		}
		for i, line := range strings.Split(string(data), "\n") {
			if len(results) >= max {
				return nil
			}
			var matched bool
			if regex {
				matched = re.MatchString(line)
			} else {
				matched = strings.Contains(line, pattern)
			}
			if matched {
				results = append(results, fmt.Sprintf("%s:%d:%s", path, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(results)
	return results, nil
}

// isBinaryExt returns true for file extensions we never search.
func isBinaryExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".zip", ".gz", ".tar", ".tgz", ".rar", ".7z",
		".exe", ".dll", ".so", ".dylib", ".o", ".a",
		".pdf", ".woff", ".woff2", ".ttf", ".eot",
		".mp3", ".mp4", ".avi", ".mov", ".wav":
		return true
	}
	return false
}
