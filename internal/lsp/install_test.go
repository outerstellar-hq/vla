package lsp

import (
	"strings"
	"testing"
)

func TestSpecForLanguage_AllLanguages(t *testing.T) {
	// Every language in DefaultSpecs must have an install spec.
	specs := DefaultSpecs()
	for lang := range specs {
		spec := SpecForLanguage(lang)
		if spec.Instructions == "" {
			t.Errorf("language %q: install spec has empty Instructions", lang)
		}
		// Manual and binary-download servers don't have a simple Command.
		if spec.Tool != InstallManual && spec.Tool != InstallBinaryDL {
			if len(spec.Command) == 0 {
				t.Errorf("language %q: install spec has empty Command", lang)
			}
			// The first element of Command must be the tool name.
			if len(spec.Command) > 0 {
				expectedTool := string(spec.Tool)
				switch spec.Tool {
				case InstallNPM:
					expectedTool = "npm"
				case InstallGo:
					expectedTool = "go"
				case InstallGem:
					expectedTool = "gem"
				case InstallBrew:
					expectedTool = "brew"
				case InstallRustup:
					expectedTool = "rustup"
				case InstallCoursier:
					expectedTool = "cs"
				}
				if spec.Command[0] != expectedTool {
					t.Errorf("language %q: command[0] = %q, expected %q", lang, spec.Command[0], expectedTool)
				}
			}
		}
	}
}

func TestSpecForLanguage_Python(t *testing.T) {
	spec := SpecForLanguage(LangPython)
	if spec.Tool != InstallNPM {
		t.Errorf("expected npm, got %s", spec.Tool)
	}
	if spec.Command[3] != "pyright" {
		t.Errorf("expected 'pyright' in command, got %v", spec.Command)
	}
}

func TestSpecForLanguage_Go(t *testing.T) {
	spec := SpecForLanguage(LangGo)
	if spec.Tool != InstallGo {
		t.Errorf("expected go, got %s", spec.Tool)
	}
	if len(spec.Command) != 3 {
		t.Fatalf("expected 3 args, got %d", len(spec.Command))
	}
	if spec.Command[2] != "golang.org/x/tools/gopls@latest" {
		t.Errorf("unexpected gopls path: %s", spec.Command[2])
	}
}

func TestSpecForLanguage_Rust(t *testing.T) {
	spec := SpecForLanguage(LangRust)
	if spec.Tool != InstallRustup {
		t.Errorf("expected rustup, got %s", spec.Tool)
	}
	// rustup component add rust-analyzer
	if spec.Command[3] != "rust-analyzer" {
		t.Errorf("expected rust-analyzer in command, got %v", spec.Command)
	}
}

func TestSpecForLanguage_Ruby(t *testing.T) {
	spec := SpecForLanguage(LangRuby)
	if spec.Tool != InstallGem {
		t.Errorf("expected gem, got %s", spec.Tool)
	}
	if spec.Command[2] != "solargraph" {
		t.Errorf("expected solargraph, got %v", spec.Command)
	}
}

func TestSpecForLanguage_JS(t *testing.T) {
	spec := SpecForLanguage(LangJS)
	if spec.Tool != InstallNPM {
		t.Errorf("expected npm, got %s", spec.Tool)
	}
	// Should install both typescript-language-server AND typescript.
	cmdStr := strings.Join(spec.Command, " ")
	if !strings.Contains(cmdStr, "typescript") {
		t.Errorf("expected typescript in command: %s", cmdStr)
	}
}

func TestSpecForLanguage_CSS_And_HTML_SamePackage(t *testing.T) {
	cssSpec := SpecForLanguage(LangCSS)
	htmlSpec := SpecForLanguage(LangHTML)
	// Both should install vscode-langservers-extracted.
	cssCmd := strings.Join(cssSpec.Command, " ")
	htmlCmd := strings.Join(htmlSpec.Command, " ")
	if !strings.Contains(cssCmd, "vscode-langservers-extracted") {
		t.Errorf("CSS should install vscode-langservers-extracted: %s", cssCmd)
	}
	if !strings.Contains(htmlCmd, "vscode-langservers-extracted") {
		t.Errorf("HTML should install vscode-langservers-extracted: %s", htmlCmd)
	}
}

func TestSpecForLanguage_Clangd_OS(t *testing.T) {
	spec := SpecForLanguage(LangC)
	if spec.Tool != InstallBrew {
		t.Errorf("expected brew, got %s", spec.Tool)
	}
	if len(spec.LinuxCmd) == 0 || spec.LinuxCmd[0] != "apt-get" {
		t.Errorf("expected apt-get for Linux: %v", spec.LinuxCmd)
	}
	if len(spec.WindowsCmd) == 0 || spec.WindowsCmd[0] != "choco" {
		t.Errorf("expected choco for Windows: %v", spec.WindowsCmd)
	}
}

func TestSpecForLanguage_ManualServers(t *testing.T) {
	for _, lang := range []Language{LangDart, LangSwift} {
		spec := SpecForLanguage(lang)
		if spec.Tool != InstallManual {
			t.Errorf("language %q: expected manual install, got %s", lang, spec.Tool)
		}
		if len(spec.Command) != 0 {
			t.Errorf("language %q: manual spec should have no command", lang)
		}
	}
}

func TestSpecForLanguage_OmniSharp(t *testing.T) {
	spec := SpecForLanguage(LangCSharp)
	if spec.Tool != InstallBinaryDL {
		t.Errorf("expected binary download, got %s", spec.Tool)
	}
	if spec.GitHubRepo != "OmniSharp/omnisharp-roslyn" {
		t.Errorf("unexpected repo: %s", spec.GitHubRepo)
	}
}

func TestSpecForLanguage_Lua(t *testing.T) {
	spec := SpecForLanguage(LangLua)
	if spec.Tool != InstallBinaryDL {
		t.Errorf("expected binary download, got %s", spec.Tool)
	}
	if spec.GitHubRepo != "LuaLS/lua-language-server" {
		t.Errorf("unexpected repo: %s", spec.GitHubRepo)
	}
}

func TestSpecForLanguage_Scala(t *testing.T) {
	spec := SpecForLanguage(LangScala)
	if spec.Tool != InstallCoursier {
		t.Errorf("expected coursier, got %s", spec.Tool)
	}
	if spec.Command[0] != "cs" {
		t.Errorf("expected cs command, got %s", spec.Command[0])
	}
}

func TestSpecForLanguage_Kotlin(t *testing.T) {
	spec := SpecForLanguage(LangKotlin)
	if spec.Tool != InstallBrew {
		t.Errorf("expected brew, got %s", spec.Tool)
	}
	if spec.Command[2] != "kotlin-language-server" {
		t.Errorf("expected kotlin-language-server: %v", spec.Command)
	}
	if spec.GitHubRepo != "fwcd/kotlin-language-server" {
		t.Errorf("unexpected repo: %s", spec.GitHubRepo)
	}
}

func TestSpecForLanguage_Elixir(t *testing.T) {
	spec := SpecForLanguage(LangElixir)
	if spec.Tool != InstallBrew {
		t.Errorf("expected brew, got %s", spec.Tool)
	}
	if spec.Command[2] != "elixir-ls" {
		t.Errorf("expected elixir-ls: %v", spec.Command)
	}
}

func TestInstallSpec_IsAvailable_Manual(t *testing.T) {
	spec := InstallSpec{Tool: InstallManual}
	if spec.IsAvailable() {
		t.Error("manual install should never be 'available'")
	}
}

func TestListSpecs(t *testing.T) {
	specs := ListSpecs()
	if len(specs) < 17 {
		t.Errorf("expected at least 17 specs, got %d", len(specs))
	}

	// Verify sorting (alphabetical by language).
	for i := 1; i < len(specs); i++ {
		if string(specs[i-1].Language) > string(specs[i].Language) {
			t.Errorf("specs not sorted: %q before %q", specs[i-1].Language, specs[i].Language)
		}
	}

	// Verify each spec has required fields.
	for _, s := range specs {
		if s.Command == "" {
			t.Errorf("spec for %q has empty Command", s.Language)
		}
	}
}

func TestFormatInstallCommand(t *testing.T) {
	s := SpecInfo{
		InstallCmd: []string{"npm", "install", "-g", "pyright"},
	}
	got := s.FormatInstallCommand()
	if got != "npm install -g pyright" {
		t.Errorf("got %q", got)
	}

	// Empty command → shows tool name in parens.
	s2 := SpecInfo{
		InstallTool: InstallManual,
		InstallCmd:  nil,
	}
	got2 := s2.FormatInstallCommand()
	if !strings.Contains(got2, "manual") {
		t.Errorf("expected tool name, got %q", got2)
	}
}

func TestCommandsForOS_NPM(t *testing.T) {
	// npm commands are the same on all platforms.
	spec := SpecForLanguage(LangPython)
	cmd := spec.CommandsForOS()
	if cmd[0] != "npm" {
		t.Errorf("expected npm, got %s", cmd[0])
	}
}

func TestCheckInstalled(t *testing.T) {
	installed := CheckInstalled()
	// Should return a result for every language.
	specs := DefaultSpecs()
	if len(installed) != len(specs) {
		t.Errorf("expected %d results, got %d", len(specs), len(installed))
	}
	// At least some should be true or false (not all missing on a dev machine).
	allMissing := true
	for _, ok := range installed {
		if ok {
			allMissing = false
			break
		}
	}
	_ = allMissing // don't fail — CI might have none installed
}
