// error_check.go — post-edit swallowed-error detection. Scans written code
// for common anti-patterns where errors are silently ignored, and returns
// warnings that are appended to the write_file/update_file tool result.
package builtin

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Warning is one swallowed-error detection.
type Warning struct {
	Line     int    // 1-based line number
	Code     string // the offending line (trimmed)
	Pattern  string // what anti-pattern was detected
	Severity string // "warning" or "critical"
}

// CheckErrorHandling scans source code content for common error-swallowing
// anti-patterns. The filename is used to determine the language and apply
// the right pattern set. Returns a list of warnings.
//
// This is a heuristic line-by-line scanner — it doesn't do full AST parsing.
// It catches the most common and obvious patterns. False positives are
// possible but preferable to silently allowing swallowed errors.
func CheckErrorHandling(filename, content string) []Warning {
	ext := strings.ToLower(filepath.Ext(filename))
	var warnings []Warning

	switch ext {
	case ".go":
		warnings = checkGo(content)
	case ".py":
		warnings = checkPython(content)
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		warnings = checkJavaScript(content)
	case ".java", ".kt", ".kts":
		warnings = checkJavaKotlin(content)
	case ".cs":
		warnings = checkCSharp(content)
	case ".php":
		warnings = checkPHP(content)
	}

	return warnings
}

// FormatWarnings renders warnings as a human-readable string for appending
// to a tool result. Returns empty string if no warnings.
func FormatWarnings(warnings []Warning) string {
	if len(warnings) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n⚠ Swallowed error warnings (")
	b.WriteString(strconv.Itoa(len(warnings)))
	b.WriteString("):\n")
	for _, w := range warnings {
		icon := "⚠"
		if w.Severity == "critical" {
			icon = "🔴"
		}
		b.WriteString("  ")
		b.WriteString(icon)
		b.WriteString(" Line ")
		b.WriteString(strconv.Itoa(w.Line))
		b.WriteString(": ")
		b.WriteString(w.Pattern)
		b.WriteString(" — ")
		b.WriteString(strings.TrimSpace(w.Code))
		b.WriteString("\n")
	}
	return b.String()
}

// --- Language-specific checkers ---

func checkGo(content string) []Warning {
	var warnings []Warning
	lines := strings.Split(content, "\n")

	// Pattern: _ = someFunc() — explicit error discard.
	reDiscard := regexp.MustCompile(`^\s*_\s*=\s*\w+`)

	// Pattern: if err != nil { } — empty error handler.
	// Matches "if err != nil {" followed by just "}".
	reEmptyErr := regexp.MustCompile(`^\s*if\s+err\s*!=\s*nil\s*\{`)

	// Pattern: if err != nil { return nil } — swallowing error by returning nil.
	reReturnNil := regexp.MustCompile(`^\s*if\s+err\s*!=\s*nil\s*\{.*return\s+nil`)

	// Pattern: defer file.Close() — unchecked deferred close.
	reDeferClose := regexp.MustCompile(`^\s*defer\s+\w+\.Close\(\)`)

	// Pattern: fmt.Println("error:", err) — printing instead of handling.
	// (This is a code smell, not always wrong, so severity=warning.)

	for i, line := range lines {
		lineNum := i + 1

		if reDiscard.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "critical",
				Pattern: "explicit error discard (_ = ...)",
			})
		}

		if reEmptyErr.MatchString(line) {
			// Check if the next non-empty line is just "}".
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "}" {
				warnings = append(warnings, Warning{
					Line: lineNum, Code: line, Severity: "critical",
					Pattern: "empty error handler (if err != nil {})",
				})
			}
		}

		if reReturnNil.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "warning",
				Pattern: "error swallowed with 'return nil'",
			})
		}

		if reDeferClose.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "warning",
				Pattern: "unchecked defer Close() — errors from Close() are ignored",
			})
		}
	}

	return warnings
}

func checkPython(content string) []Warning {
	var warnings []Warning
	lines := strings.Split(content, "\n")

	reExcept := regexp.MustCompile(`^\s*except(\s+\w+)?:`)
	rePass := regexp.MustCompile(`^\s*pass\s*$`)

	for i, line := range lines {
		lineNum := i + 1

		if reExcept.MatchString(line) {
			// Check if the body is just "pass" or a comment.
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if rePass.MatchString(next) || strings.HasPrefix(next, "#") {
					warnings = append(warnings, Warning{
						Line: lineNum, Code: line, Severity: "critical",
						Pattern: "empty except block (except: pass)",
					})
				}
			}
		}

		// @ function call — error suppression operator in PHP, but in Python
		// @ is a decorator. Check for common suppress patterns instead.
		if strings.Contains(line, "except:") && !strings.Contains(line, "as ") {
			// Bare except without 'as' — swallows everything silently.
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if rePass.MatchString(next) || next == "" {
					warnings = append(warnings, Warning{
						Line: lineNum, Code: line, Severity: "warning",
						Pattern: "bare except without capturing the exception",
					})
				}
			}
		}
	}

	return warnings
}

func checkJavaScript(content string) []Warning {
	var warnings []Warning
	lines := strings.Split(content, "\n")

	reEmptyCatch := regexp.MustCompile(`catch\s*\([^)]*\)\s*\{\s*\}`)
	reCatchArrow := regexp.MustCompile(`\.catch\(\s*\(\s*\)\s*=>\s*\{?\s*\}?\s*\)`)
	reCatchArrowEmpty := regexp.MustCompile(`\.catch\(\s*\(\s*\)\s*=>\s*\{\s*\}\s*\)`)

	for i, line := range lines {
		lineNum := i + 1

		if reEmptyCatch.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "critical",
				Pattern: "empty catch block",
			})
		}

		if reCatchArrowEmpty.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "critical",
				Pattern: "empty .catch() — promise rejection swallowed",
			})
		}

		if reCatchArrow.MatchString(line) && !reCatchArrowEmpty.MatchString(line) {
			// .catch(() => ) without body — also swallows.
			if !strings.Contains(line, "console.") && !strings.Contains(line, "throw") {
				warnings = append(warnings, Warning{
					Line: lineNum, Code: line, Severity: "warning",
					Pattern: ".catch() with no logging or rethrow",
				})
			}
		}
	}

	return warnings
}

func checkJavaKotlin(content string) []Warning {
	var warnings []Warning
	lines := strings.Split(content, "\n")

	reEmptyCatch := regexp.MustCompile(`catch\s*\([^)]*\)\s*\{\s*\}`)
	reCatchReturn := regexp.MustCompile(`catch\s*\([^)]*\)\s*\{[^}]*return\s*;`)

	for i, line := range lines {
		lineNum := i + 1

		if reEmptyCatch.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "critical",
				Pattern: "empty catch block",
			})
		}

		if reCatchReturn.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "warning",
				Pattern: "catch block swallows error and returns",
			})
		}
	}

	return warnings
}

func checkCSharp(content string) []Warning {
	// C# catch patterns are the same as Java/Kotlin.
	return checkJavaKotlin(content)
}

func checkPHP(content string) []Warning {
	var warnings []Warning
	lines := strings.Split(content, "\n")

	// @ operator suppresses errors — anywhere in the line before a function call.
	reSuppress := regexp.MustCompile(`@\s*\w+\s*\(`)
	reEmptyCatch := regexp.MustCompile(`catch\s*\([^)]*\)\s*\{\s*\}`)

	for i, line := range lines {
		lineNum := i + 1

		// @file_get_contents(), @fopen(), etc.
		if reSuppress.MatchString(line) && !strings.Contains(line, "@import") && !strings.Contains(line, "@media") {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "warning",
				Pattern: "@ error suppression operator — errors are silenced",
			})
		}

		if reEmptyCatch.MatchString(line) {
			warnings = append(warnings, Warning{
				Line: lineNum, Code: line, Severity: "critical",
				Pattern: "empty catch block",
			})
		}
	}

	return warnings
}

// isSourceCode returns true if the file extension is a supported language.
func isSourceCode(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs",
		".java", ".kt", ".kts", ".cs", ".php":
		return true
	}
	return false
}
