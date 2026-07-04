package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// mdRenderer is a cached glamour renderer, recreated only when the width
// changes. glamour.TermRenderer is safe for concurrent use after creation.
var (
	mdRenderer    *glamour.TermRenderer
	mdRendererW   int
	mdRendererErr error
)

// renderMarkdown renders a markdown string to ANSI-styled text suitable for
// the conversation pane. It uses a dark-themed auto-style (the TUI runs in
// alt-screen mode which is typically dark). The output has trailing
// whitespace/newlines trimmed so it slots cleanly into the block renderer.
//
// If glamour fails to initialize (e.g. no terminal info in test envs),
// the function returns the input unchanged — the TUI still works, just
// without syntax highlighting.
func renderMarkdown(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	// No code blocks or markdown syntax? Skip the renderer for speed.
	if !looksLikeMarkdown(text) {
		return text
	}

	r := getRenderer(width)
	if r == nil {
		return text
	}

	out, err := r.Render(text)
	if err != nil {
		return text // rendering failed — show raw text rather than crashing
	}
	return strings.TrimRight(out, "\n\r ")
}

// getRenderer returns a cached renderer for the given width, recreating it
// only when the width changes. Returns nil if the renderer can't be created.
func getRenderer(width int) *glamour.TermRenderer {
	w := width - 4 // account for left padding in the viewport
	if w < 20 {
		w = 80
	}
	if mdRenderer != nil && mdRendererW == w {
		return mdRenderer
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		mdRendererErr = err
		return nil
	}
	mdRenderer = r
	mdRendererW = w
	return r
}

// looksLikeMarkdown does a quick heuristic check so we don't run the full
// markdown renderer on plain text responses (which is wasteful and can add
// unwanted styling to simple answers).
func looksLikeMarkdown(s string) bool {
	// Code fences
	if strings.Contains(s, "```") {
		return true
	}
	// Headings (# through ######)
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "#") {
			// Count leading #
			n := 0
			for n < len(trimmed) && trimmed[n] == '#' {
				n++
			}
			if n <= 6 && n < len(trimmed) && trimmed[n] == ' ' {
				return true
			}
		}
		// List items (- or * at start)
		if (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) && len(trimmed) > 2 {
			return true
		}
		// Bold/italic
		if strings.Contains(line, "**") || strings.Contains(line, "`") {
			return true
		}
	}
	// Tables (pipe-delimited rows)
	if strings.Contains(s, "|") && strings.Contains(s, "---") {
		return true
	}
	return false
}
