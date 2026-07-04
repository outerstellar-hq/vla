package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// diffStatus classifies a line in a diff result.
type diffStatus int

const (
	diffUnchanged diffStatus = iota
	diffAdded
	diffRemoved
)

// diffLine is one line of a computed diff.
type diffLine struct {
	text   string
	status diffStatus
}

// maxDiffLines caps the diff output to prevent pathological cases (very
// large file writes) from overwhelming the TUI. 200 lines is plenty for
// a visual preview — the user can expand the tool block for the full output.
const maxDiffLines = 200

// diffStyles holds the lipgloss styles for diff rendering.
var (
	diffAddStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	diffDelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	diffContextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // gray
	diffHeaderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	diffBorderVDark  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// computeDiff returns a line-by-line diff of old vs new using the classic
// LCS (Longest Common Subsequence) algorithm. The result alternates
// unchanged, added, and removed lines in the order they appear.
//
// For write_file (where old is empty), all lines are marked diffAdded.
// For update_file (where old/new are partial snippets), only the changed
// region is shown.
func computeDiff(old, new string) []diffLine {
	oldLines := splitLines(old)
	newLines := splitLines(new)

	// Compute the LCS table. lcs[i][j] = length of LCS of oldLines[i:] and
	// newLines[j:]. We use uint16 since diffs are capped at maxDiffLines.
	n := len(oldLines)
	m := len(newLines)

	// Cap to prevent O(n*m) explosion on very large inputs.
	if n > maxDiffLines {
		n = maxDiffLines
	}
	if m > maxDiffLines {
		m = maxDiffLines
	}

	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	// Backtrack through the LCS table to produce the diff.
	var result []diffLine
	i, j := 0, 0
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			result = append(result, diffLine{text: oldLines[i], status: diffUnchanged})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			result = append(result, diffLine{text: oldLines[i], status: diffRemoved})
			i++
		} else {
			result = append(result, diffLine{text: newLines[j], status: diffAdded})
			j++
		}
	}
	// Remaining removed lines.
	for i < n {
		result = append(result, diffLine{text: oldLines[i], status: diffRemoved})
		i++
	}
	// Remaining added lines.
	for j < m {
		result = append(result, diffLine{text: newLines[j], status: diffAdded})
		j++
	}

	// Cap the output.
	if len(result) > maxDiffLines {
		result = result[:maxDiffLines]
	}

	return result
}

// renderDiff renders a slice of diffLines as colored text suitable for the
// diff pane. Each line is prefixed with +/-/space and colored:
//   - added lines: green with "+" prefix
//   - removed lines: red with "-" prefix
//   - unchanged lines: gray with " " prefix
//
// width is the available pane width; lines are truncated to fit.
func renderDiff(lines []diffLine, width int) string {
	if width < 10 {
		width = 40
	}
	// Account for the 1-char prefix + 1 space margin.
	contentWidth := width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	var b strings.Builder
	for _, dl := range lines {
		text := dl.text
		if len(text) > contentWidth {
			text = text[:contentWidth-1] + "…"
		}

		switch dl.status {
		case diffAdded:
			b.WriteString(diffAddStyle.Render("+ " + text))
		case diffRemoved:
			b.WriteString(diffDelStyle.Render("- " + text))
		case diffUnchanged:
			b.WriteString(diffContextStyle.Render("  " + text))
		}
		b.WriteString("\n")
	}

	if len(lines) == 0 {
		b.WriteString(diffContextStyle.Render("(no changes)"))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderDiffPane renders the complete right-side diff pane: a header
// (tool name + file path) + the diff content, sized to the given width
// and height.
func renderDiffPane(title, content string, width, height int) string {
	if width < 10 {
		width = 40
	}

	// Header line.
	header := diffHeaderStyle.Render(" " + title)

	// Content area with left padding.
	contentLines := strings.Split(content, "\n")
	if len(contentLines) > height-1 {
		contentLines = contentLines[:height-1]
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(diffBorderVDark.Render(strings.Repeat("─", width-1)))
	b.WriteString("\n")
	for _, line := range contentLines {
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Pad to fill the pane height.
	rendered := b.String()
	lineCount := strings.Count(rendered, "\n") + 1
	for lineCount < height {
		b.WriteString("\n")
		lineCount++
	}

	result := b.String()
	// Truncate to exactly height lines.
	lines := strings.Split(result, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// splitLines splits s by newlines, returning a slice without trailing empty
// strings. "a\nb\nc" → ["a", "b", "c"]. "" → [].
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
