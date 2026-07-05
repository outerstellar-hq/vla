package builtin

import (
	"testing"
)

func TestCheckErrorHandling_Go_DiscardError(t *testing.T) {
	content := `package main

func doSomething() {
	_ = riskyFunc()
}
`
	warnings := CheckErrorHandling("main.go", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Severity != "critical" {
		t.Errorf("expected critical severity, got %s", warnings[0].Severity)
	}
	if warnings[0].Line != 4 {
		t.Errorf("expected line 4, got %d", warnings[0].Line)
	}
}

func TestCheckErrorHandling_Go_EmptyErrorHandler(t *testing.T) {
	content := `package main

func doSomething() error {
	if err != nil {
	}
	return nil
}
`
	warnings := CheckErrorHandling("main.go", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Pattern != "empty error handler (if err != nil {})" {
		t.Errorf("unexpected pattern: %s", warnings[0].Pattern)
	}
}

func TestCheckErrorHandling_Go_DeferClose(t *testing.T) {
	content := `package main

func doSomething() {
	file, _ := os.Open("foo.txt")
	defer file.Close()
}
`
	warnings := CheckErrorHandling("main.go", content)
	// Should detect: _ = os.Open (discard) AND defer Close.
	if len(warnings) < 1 {
		t.Fatalf("expected at least 1 warning, got %d", len(warnings))
	}

	// Find the defer Close warning.
	found := false
	for _, w := range warnings {
		if w.Pattern == "unchecked defer Close() — errors from Close() are ignored" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected defer Close warning")
	}
}

func TestCheckErrorHandling_Go_ReturnNilOnError(t *testing.T) {
	content := `package main

func doSomething() error {
	if err != nil { return nil }
	return nil
}
`
	warnings := CheckErrorHandling("main.go", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Severity != "warning" {
		t.Errorf("expected warning, got %s", warnings[0].Severity)
	}
}

func TestCheckErrorHandling_Go_Clean(t *testing.T) {
	content := `package main

func doSomething() error {
	if err := riskyFunc(); err != nil {
		return fmt.Errorf("failed: %w", err)
	}
	return nil
}
`
	warnings := CheckErrorHandling("main.go", content)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for clean code, got %d: %+v", len(warnings), warnings)
	}
}

func TestCheckErrorHandling_Python_EmptyExcept(t *testing.T) {
	content := `def do_something():
    try:
        risky()
    except Exception:
        pass
`
	warnings := CheckErrorHandling("auth.py", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Severity != "critical" {
		t.Errorf("expected critical, got %s", warnings[0].Severity)
	}
}

func TestCheckErrorHandling_Python_Clean(t *testing.T) {
	content := `def do_something():
    try:
        risky()
    except Exception as e:
        logger.error("failed: %s", e)
        raise
`
	warnings := CheckErrorHandling("auth.py", content)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for clean code, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_JS_EmptyCatch(t *testing.T) {
	content := `function doSomething() {
	try {
		risky();
	} catch (e) {}
}
`
	warnings := CheckErrorHandling("app.js", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_JS_EmptyCatchArrow(t *testing.T) {
	content := `async function doSomething() {
	await risky().catch(() => {});
}
`
	warnings := CheckErrorHandling("app.js", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_JS_Clean(t *testing.T) {
	content := `async function doSomething() {
	try {
		await risky();
	} catch (e) {
		console.error(e);
		throw e;
	}
}
`
	warnings := CheckErrorHandling("app.js", content)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_Java_EmptyCatch(t *testing.T) {
	content := `public void doSomething() {
	try {
		risky();
	} catch (Exception e) {}
}
`
	warnings := CheckErrorHandling("Main.java", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_CSharp_EmptyCatch(t *testing.T) {
	content := `public void DoSomething() {
	try {
		Risky();
	} catch (Exception e) {}
}
`
	warnings := CheckErrorHandling("Program.cs", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_PHP_SuppressOperator(t *testing.T) {
	content := `<?php
$data = @file_get_contents("config.json");
?>
`
	warnings := CheckErrorHandling("config.php", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Severity != "warning" {
		t.Errorf("expected warning severity, got %s", warnings[0].Severity)
	}
}

func TestCheckErrorHandling_PHP_EmptyCatch(t *testing.T) {
	content := `<?php
try {
    risky();
} catch (Exception $e) {}
?>
`
	warnings := CheckErrorHandling("config.php", content)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_UnsupportedExtension(t *testing.T) {
	content := "This is a text file with no code."
	warnings := CheckErrorHandling("readme.md", content)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for .md, got %d", len(warnings))
	}
}

func TestCheckErrorHandling_NoFalsePositive_GoodGo(t *testing.T) {
	content := `package main

import "fmt"

func main() {
	if err := run(); err != nil {
		fmt.Println("error:", err)
	}
}

func run() error {
	return nil
}
`
	warnings := CheckErrorHandling("main.go", content)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for well-written Go, got %d: %+v", len(warnings), warnings)
	}
}

func TestFormatWarnings_Empty(t *testing.T) {
	result := FormatWarnings(nil)
	if result != "" {
		t.Errorf("expected empty string for nil warnings, got %q", result)
	}
}

func TestFormatWarnings_WithWarnings(t *testing.T) {
	warnings := []Warning{
		{Line: 5, Code: "  _ = riskyFunc()", Severity: "critical", Pattern: "explicit error discard"},
		{Line: 10, Code: "  defer f.Close()", Severity: "warning", Pattern: "unchecked defer Close()"},
	}
	result := FormatWarnings(warnings)

	if !contains(result, "Line 5") {
		t.Errorf("expected 'Line 5' in output: %s", result)
	}
	if !contains(result, "Line 10") {
		t.Errorf("expected 'Line 10' in output: %s", result)
	}
	if !contains(result, "Swallowed error warnings (2)") {
		t.Errorf("expected count in output: %s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
