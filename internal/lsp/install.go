package lsp

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// InstallTool identifies the exact method used to install a specific
// language server. Each value maps to a concrete, known install path.
type InstallTool string

const (
	InstallNPM      InstallTool = "npm"
	InstallGo       InstallTool = "go"
	InstallGem      InstallTool = "gem"
	InstallBrew     InstallTool = "brew"
	InstallRustup   InstallTool = "rustup"
	InstallApt      InstallTool = "apt"
	InstallChoco    InstallTool = "choco"
	InstallCoursier InstallTool = "coursier"
	InstallBinaryDL InstallTool = "binary" // GitHub release download
	InstallManual   InstallTool = "manual" // SDK-bundled, cannot auto-install
)

// InstallSpec describes exactly how to install one specific language server.
// Every server has its own hand-written spec — no generic fallbacks.
type InstallSpec struct {
	Tool InstallTool

	// The command for the current OS (set by CommandsForOS).
	// For npm/go/gem this is the same on all platforms.
	// For brew, this is macOS-only; Linux/Windows use the alternatives below.
	Command []string

	// OS-specific alternatives for brew-only servers.
	LinuxCmd   []string
	WindowsCmd []string

	// For binary-download servers: GitHub repo to fetch releases from.
	GitHubRepo string

	// Full human-readable instructions shown when auto-install is not possible.
	Instructions string
}

// IsAvailable returns true if the install tool for this spec is on PATH.
func (s InstallSpec) IsAvailable() bool {
	if s.Tool == InstallManual {
		return false
	}
	if s.Tool == InstallBinaryDL {
		// Binary downloads need curl.
		_, err := exec.LookPath("curl")
		return err == nil
	}
	cmd := s.CommandsForOS()
	if len(cmd) == 0 {
		return false
	}
	_, err := exec.LookPath(cmd[0])
	return err == nil
}

// CommandsForOS returns the install command for the current OS.
// Returns nil if no command is available for this platform.
func (s InstallSpec) CommandsForOS() []string {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, brew servers use brew. Everything else uses its native tool.
		if s.Tool == InstallBrew {
			return s.Command
		}
		return s.Command

	case "linux":
		if s.Tool == InstallBrew {
			return s.LinuxCmd // apt/snap/manual
		}
		return s.Command

	case "windows":
		if s.Tool == InstallBrew {
			return s.WindowsCmd // choco/scoop/manual
		}
		return s.Command

	default:
		return s.Command
	}
}

// Run executes the install command. Returns an error if the tool isn't
// available or the install fails. Output goes to stderr/stdout of the
// calling process.
func (s InstallSpec) Run() error {
	cmd := s.CommandsForOS()
	if len(cmd) == 0 {
		return fmt.Errorf("no install command available for %s on %s/%s",
			s.Tool, runtime.GOOS, runtime.GOARCH)
	}

	// Verify the install tool itself is present.
	if _, err := exec.LookPath(cmd[0]); err != nil {
		return fmt.Errorf("install tool %q not found on PATH", cmd[0])
	}

	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin = nil
	c.Stdout = nil // caller can capture if needed
	c.Stderr = nil
	return c.Run()
}

// SpecForLanguage returns the exact install instructions for one language
// server. Every language has a hand-written spec with its real install command.
func SpecForLanguage(lang Language) InstallSpec {
	switch lang {

	case LangPython:
		return InstallSpec{
			Tool:    InstallNPM,
			Command: []string{"npm", "install", "-g", "pyright"},
			Instructions: `pyright-langserver (Python)

  npm install -g pyright

  Requires Node.js — install from https://nodejs.org
  Verify: pyright-langserver --version`,
		}

	case LangGo:
		return InstallSpec{
			Tool:    InstallGo,
			Command: []string{"go", "install", "golang.org/x/tools/gopls@latest"},
			Instructions: `gopls (Go)

  go install golang.org/x/tools/gopls@latest

  Requires Go — install from https://go.dev/dl/
  Verify: gopls version`,
		}

	case LangKotlin:
		return InstallSpec{
			Tool:       InstallBrew,
			Command:    []string{"brew", "install", "kotlin-language-server"},
			LinuxCmd:   nil, // Linux: GitHub binary download, no package manager
			WindowsCmd: nil,
			GitHubRepo: "fwcd/kotlin-language-server",
			Instructions: `kotlin-language-server (Kotlin)

  macOS:
    brew install kotlin-language-server

  Linux/Windows:
    Download from https://github.com/fwcd/kotlin-language-server/releases
    Extract and add the bin/ directory to PATH

  Requires JDK 17+`,
		}

	case LangJava:
		return InstallSpec{
			Tool:       InstallBrew,
			Command:    []string{"brew", "install", "jdtls"},
			GitHubRepo: "eclipse-jdtls/eclipse.jdt.ls",
			Instructions: `jdtls — Eclipse JDT.Language Server (Java)

  macOS:
    brew install jdtls

  Linux:
    Download from https://download.eclipse.org/jdtls/milestones/
    Extract and configure the bin/jdtls launcher script

  Requires Java 17+ JDK`,
		}

	case LangCSharp:
		return InstallSpec{
			Tool:       InstallBinaryDL,
			GitHubRepo: "OmniSharp/omnisharp-roslyn",
			Instructions: `OmniSharp (C#)

  Download from https://github.com/OmniSharp/omnisharp-roslyn/releases
  Pick the platform-specific build:
    macOS arm64:  OmniSharp-osx-arm64-net8.0.zip
    macOS x64:    OmniSharp-osx-x64-net8.0.zip
    Linux x64:    OmniSharp-linux-x64-net8.0.zip
    Windows x64:  OmniSharp-win-x64-net8.0.zip

  Extract and add to PATH. Requires .NET SDK 8.0+`,
		}

	case LangPHP:
		return InstallSpec{
			Tool:    InstallNPM,
			Command: []string{"npm", "install", "-g", "intelephense"},
			Instructions: `intelephense (PHP)

  npm install -g intelephense

  Requires Node.js — install from https://nodejs.org
  Verify: intelephense --version`,
		}

	case LangJS:
		return InstallSpec{
			Tool:    InstallNPM,
			Command: []string{"npm", "install", "-g", "typescript-language-server", "typescript"},
			Instructions: `typescript-language-server (JavaScript/TypeScript)

  npm install -g typescript-language-server typescript

  Requires Node.js — install from https://nodejs.org
  Verify: typescript-language-server --version`,
		}

	case LangCSS:
		return InstallSpec{
			Tool:    InstallNPM,
			Command: []string{"npm", "install", "-g", "vscode-langservers-extracted"},
			Instructions: `vscode-css-language-server (CSS/SCSS)

  npm install -g vscode-langservers-extracted

  This package provides vscode-css-language-server AND vscode-html-language-server.
  Requires Node.js — install from https://nodejs.org`,
		}

	case LangHTML:
		return InstallSpec{
			Tool:    InstallNPM,
			Command: []string{"npm", "install", "-g", "vscode-langservers-extracted"},
			Instructions: `vscode-html-language-server (HTML)

  npm install -g vscode-langservers-extracted

  This package provides vscode-css-language-server AND vscode-html-language-server.
  Requires Node.js — install from https://nodejs.org`,
		}

	case LangRust:
		return InstallSpec{
			Tool:    InstallRustup,
			Command: []string{"rustup", "component", "add", "rust-analyzer"},
			Instructions: `rust-analyzer (Rust)

  rustup component add rust-analyzer

  Alternatives:
    macOS:  brew install rust-analyzer
    Cargo:  cargo install rust-analyzer

  Requires Rust — install via https://rustup.rs
  Verify: rust-analyzer --version`,
		}

	case LangRuby:
		return InstallSpec{
			Tool:    InstallGem,
			Command: []string{"gem", "install", "solargraph"},
			Instructions: `solargraph (Ruby)

  gem install solargraph

  Requires Ruby — install from https://www.ruby-lang.org/en/downloads/
  Verify: solargraph --version`,
		}

	case LangC:
		return InstallSpec{
			Tool:       InstallBrew,
			Command:    []string{"brew", "install", "llvm"},
			LinuxCmd:   []string{"apt-get", "install", "-y", "clangd"},
			WindowsCmd: []string{"choco", "install", "llvm"},
			Instructions: `clangd (C/C++)

  macOS:
    brew install llvm
    (clangd is in the llvm keg, add to PATH: $(brew --prefix llvm)/bin)

  Linux (Ubuntu/Debian):
    sudo apt install clangd

  Linux (Fedora/RHEL):
    sudo dnf install clang-tools-extra

  Windows:
    choco install llvm

  Or install from https://clangd.llvm.org/installation`,
		}

	case LangDart:
		return InstallSpec{
			Tool: InstallManual,
			Instructions: `Dart analysis server (Dart)

  The language server is bundled with the Dart SDK — it cannot be
  installed separately.

  Install Dart SDK: https://dart.dev/get-dart
  Or install Flutter (includes Dart): https://flutter.dev

  Once dart is on PATH, the server runs automatically.`,
		}

	case LangLua:
		return InstallSpec{
			Tool:       InstallBinaryDL,
			GitHubRepo: "LuaLS/lua-language-server",
			Instructions: `lua-language-server (Lua)

  Download from https://github.com/LuaLS/lua-language-server/releases
  Pick the platform-specific archive:
    macOS arm64:  lua-language-server-3.x.x-darwin-arm64.tar.gz
    macOS x64:    lua-language-server-3.x.x-darwin-x64.tar.gz
    Linux x64:    lua-language-server-3.x.x-linux-x64.tar.gz
    Windows x64:  lua-language-server-3.x.x-win32-x64.zip

  Extract and add the bin/ directory to PATH.

  Or build from source:
    git clone https://github.com/LuaLS/lua-language-server
    cd lua-language-server && ./make.sh`,
		}

	case LangElixir:
		return InstallSpec{
			Tool:    InstallBrew,
			Command: []string{"brew", "install", "elixir-ls"},
			Instructions: `elixir-ls (Elixir)

  macOS:
    brew install elixir-ls

  Linux:
    git clone https://github.com/elixir-lsp/elixir-ls
    cd elixir-ls
    mix deps.get
    MIX_ENV=prod mix elixir_ls.release2 -o ./release
    Add ./release to PATH

  Requires Elixir 1.16+ and Erlang/OTP 26+`,
		}

	case LangScala:
		return InstallSpec{
			Tool:    InstallCoursier,
			Command: []string{"cs", "bootstrap", "org.scalameta:metals_2.13:1.4.0", "-o", "metals", "-f"},
			Instructions: `metals (Scala)

  coursier bootstrap org.scalameta:metals_2.13:1.4.0 -o metals -f

  Requires Coursier (cs) — install from https://get-coursier.io
  Requires JDK 17+

  macOS alternative:
    brew install metals`,
		}

	case LangSwift:
		return InstallSpec{
			Tool: InstallManual,
			Instructions: `sourcekit-lsp (Swift)

  sourcekit-lsp is bundled with the Swift toolchain — it cannot be
  installed separately.

  macOS:  Install Xcode — sourcekit-lsp is included
  Linux:  Install from https://swift.org/install/

  Verify: xcrun --find sourcekit-lsp`,
		}

	default:
		return InstallSpec{
			Tool: InstallManual,
			Instructions: `Unknown language — no install spec available.

  Check the language server's documentation for install instructions.`,
		}
	}
}

// CheckInstalled returns a map of language → installed (bool) for all
// known language servers. Uses exec.LookPath to check each binary.
func CheckInstalled() map[Language]bool {
	specs := DefaultSpecs()
	result := make(map[Language]bool, len(specs))
	for lang, spec := range specs {
		_, err := exec.LookPath(spec.Command)
		result[lang] = err == nil
	}
	return result
}

// SpecInfo is one row in the `vla lsp list` output.
type SpecInfo struct {
	Language    Language
	Command     string
	Installed   bool
	InstallTool InstallTool
	InstallCmd  []string
}

// ListSpecs returns info about all known language servers, sorted by
// language name, for display in the `vla lsp list` subcommand.
func ListSpecs() []SpecInfo {
	specs := DefaultSpecs()
	installed := CheckInstalled()
	result := make([]SpecInfo, 0, len(specs))

	// Collect and sort by language name.
	langs := make([]Language, 0, len(specs))
	for lang := range specs {
		langs = append(langs, lang)
	}
	for i := 0; i < len(langs); i++ {
		for j := i + 1; j < len(langs); j++ {
			if string(langs[i]) > string(langs[j]) {
				langs[i], langs[j] = langs[j], langs[i]
			}
		}
	}

	for _, lang := range langs {
		spec := specs[lang]
		installSpec := SpecForLanguage(lang)
		cmd := installSpec.CommandsForOS()
		if cmd == nil {
			cmd = installSpec.Command
		}
		result = append(result, SpecInfo{
			Language:    lang,
			Command:     spec.Command,
			Installed:   installed[lang],
			InstallTool: installSpec.Tool,
			InstallCmd:  cmd,
		})
	}
	return result
}

// FormatInstallCommand returns a human-readable version of the install command.
func (s SpecInfo) FormatInstallCommand() string {
	if len(s.InstallCmd) == 0 {
		return "(" + string(s.InstallTool) + ")"
	}
	return strings.Join(s.InstallCmd, " ")
}
