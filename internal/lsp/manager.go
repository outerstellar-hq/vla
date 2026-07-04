package lsp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Language identifies which LSP server to run.
type Language string

const (
	LangPython Language = "python"
	LangGo     Language = "go"
	LangKotlin Language = "kotlin"
	LangJava   Language = "java"
	LangCSharp Language = "csharp"
	LangPHP    Language = "php"
	LangJS     Language = "javascript"
	LangCSS    Language = "css"
	LangHTML   Language = "html"
	LangRust   Language = "rust"
	LangRuby   Language = "ruby"
	LangC      Language = "c"
	LangDart   Language = "dart"
	LangLua    Language = "lua"
	LangElixir Language = "elixir"
	LangScala  Language = "scala"
	LangSwift  Language = "swift"
)

// ServerSpec describes how to launch a language server for one language.
type ServerSpec struct {
	Language Language
	Command  string   // executable name or absolute path
	Args     []string // command-line arguments
}

// DefaultSpecs returns sensible defaults for each supported language.
// These are only used if the server is found on PATH; the manager won't fail
// if they're missing — navigation tools just fall back to the regex indexer.
//
//	Python:   pyright-langserver --stdio
//	Go:       gopls serve
//	Kotlin:   fwcd/kotlin-language-server --stdio
//	Java:     Eclipse JDT.LS (jdtls --stdio)
//	C#:       OmniSharp-roslyn (-lsp)
//	PHP:      intelephense --stdio
//	JS/TS:    typescript-language-server --stdio
//	CSS/SCSS: vscode-css-language-server --stdio
//	HTML:     vscode-html-language-server --stdio
//	Rust:     rust-analyzer
//	Ruby:     solargraph stdio
//	C/C++:    clangd
//	Dart:     dart language-server
//	Lua:      lua-language-server
//	Elixir:   elixir-ls
//	Scala:    metals
//	Swift:    sourcekit-lsp
func DefaultSpecs() map[Language]ServerSpec {
	return map[Language]ServerSpec{
		LangPython: {Language: LangPython, Command: "pyright-langserver", Args: []string{"--stdio"}},
		LangGo:     {Language: LangGo, Command: "gopls", Args: []string{"serve"}},
		LangKotlin: {Language: LangKotlin, Command: "kotlin-language-server", Args: []string{"--stdio"}},
		LangJava:   {Language: LangJava, Command: "jdtls", Args: []string{"--stdio"}},
		LangCSharp: {Language: LangCSharp, Command: "OmniSharp", Args: []string{"-lsp"}},
		LangPHP:    {Language: LangPHP, Command: "intelephense", Args: []string{"--stdio"}},
		LangJS:     {Language: LangJS, Command: "typescript-language-server", Args: []string{"--stdio"}},
		LangCSS:    {Language: LangCSS, Command: "vscode-css-language-server", Args: []string{"--stdio"}},
		LangHTML:   {Language: LangHTML, Command: "vscode-html-language-server", Args: []string{"--stdio"}},
		LangRust:   {Language: LangRust, Command: "rust-analyzer", Args: nil},
		LangRuby:   {Language: LangRuby, Command: "solargraph", Args: []string{"stdio"}},
		LangC:      {Language: LangC, Command: "clangd", Args: nil},
		LangDart:   {Language: LangDart, Command: "dart", Args: []string{"language-server", "--protocol=lsp"}},
		LangLua:    {Language: LangLua, Command: "lua-language-server", Args: nil},
		LangElixir: {Language: LangElixir, Command: "elixir-ls", Args: []string{"--stdio"}},
		LangScala:  {Language: LangScala, Command: "metals", Args: []string{"--stdio"}},
		LangSwift:  {Language: LangSwift, Command: "sourcekit-lsp", Args: nil},
	}
}

// Manager owns a pool of warm LSP processes, one per (language, workspace).
// It auto-starts servers on first use and restarts crashed ones.
type Manager struct {
	specs       map[Language]ServerSpec
	mu          sync.Mutex
	clients     map[string]*clientHandle // key = "<lang>::<workspace>"
	autoInstall bool                     // if true, try installing missing servers
}

type clientHandle struct {
	client *Client
	cmd    *exec.Cmd
}

// NewManager creates a Manager with the given server specs (use DefaultSpecs()
// if you don't have custom ones).
func NewManager(specs map[Language]ServerSpec) *Manager {
	return &Manager{
		specs:   specs,
		clients: make(map[string]*clientHandle),
	}
}

// SetAutoInstall enables or disables automatic installation of missing
// language servers. When enabled, the manager will attempt to install
// a server using its InstallSpec when exec.LookPath fails.
func (m *Manager) SetAutoInstall(enabled bool) {
	m.mu.Lock()
	m.autoInstall = enabled
	m.mu.Unlock()
}

// Get returns a Client for the given language + workspace, starting the server
// if necessary. Returns an error if the server isn't available (not on PATH,
// failed to start, etc.) — callers should fall back to the regex indexer.
func (m *Manager) Get(lang Language, workspace string) (*Client, error) {
	key := fmt.Sprintf("%s::%s", lang, workspace)

	m.mu.Lock()
	defer m.mu.Unlock()

	if handle, ok := m.clients[key]; ok {
		// Check the process is still alive.
		if handle.cmd.ProcessState == nil || handle.cmd.ProcessState.Exited() {
			// Dead — remove and restart.
			delete(m.clients, key)
		} else {
			return handle.client, nil
		}
	}

	return m.startLocked(lang, workspace, key)
}

func (m *Manager) startLocked(lang Language, workspace, key string) (*Client, error) {
	spec, ok := m.specs[lang]
	if !ok {
		return nil, fmt.Errorf("lsp: no server spec for language %q", lang)
	}

	// Verify the executable is on PATH.
	path, err := exec.LookPath(spec.Command)
	if err != nil {
		// If auto-install is enabled, try installing the server.
		if m.autoInstall {
			installSpec := SpecForLanguage(lang)
			if installSpec.IsAvailable() {
				if installErr := installSpec.Run(); installErr == nil {
					// Re-check after install.
					path, err = exec.LookPath(spec.Command)
				}
			}
		}
		if err != nil {
			// Return error with the server's install instructions.
			installSpec := SpecForLanguage(lang)
			return nil, fmt.Errorf("lsp: %s not found on PATH\n\n%s", spec.Command, installSpec.Instructions)
		}
	}

	rootURI := pathToURI(workspace)
	args := append([]string{}, spec.Args...)
	cmd := exec.Command(path, args...)
	cmd.Dir = workspace

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %s: %w", spec.Command, err)
	}

	client := NewClient(stdout, stdin)
	client.Start()

	// Drain stderr on a daemon goroutine to prevent pipe deadlock.
	go func() {
		buf := make([]byte, 4096)
		for {
			if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
				return
			}
			_, err := stderr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Send initialize request.
	initParams := map[string]any{
		"processId": nil, // we can't easily get our PID portably; nil is fine
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"didChangeWatchedFiles": map[string]any{
					"dynamicRegistration": false,
				},
			},
			"textDocument": map[string]any{
				"definition": map[string]any{"linkSupport": false},
				"references": map[string]any{},
				"hover":      map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
		},
	}

	// Wait up to 30 seconds for initialization.
	done := make(chan error, 1)
	go func() {
		_, err := client.Request("initialize", initParams)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("lsp: initialize %s: %w", spec.Command, err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("lsp: %s initialization timed out (30s)", spec.Command)
	}

	// Send initialized notification.
	_ = client.Notify("initialized", map[string]any{})

	m.clients[key] = &clientHandle{client: client, cmd: cmd}
	return client, nil
}

// Close shuts down all LSP servers. Called on VLA exit.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, handle := range m.clients {
		_ = handle.client.Notify("shutdown", nil)
		_ = handle.client.Notify("exit", nil)
		_ = handle.cmd.Process.Kill()
		handle.client.Close()
	}
	m.clients = make(map[string]*clientHandle)
}

// InferLanguage guesses the primary language from project marker files.
// Returns empty string if it can't determine.
func InferLanguage(workspace string) Language {
	// Rust: Cargo.toml
	if fileExists(filepath.Join(workspace, "Cargo.toml")) {
		return LangRust
	}
	// Go: go.mod
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return LangGo
	}
	// Kotlin: build.gradle.kts or settings.gradle.kts
	if fileExists(filepath.Join(workspace, "build.gradle.kts")) ||
		fileExists(filepath.Join(workspace, "settings.gradle.kts")) {
		return LangKotlin
	}
	// Scala: build.sbt
	if fileExists(filepath.Join(workspace, "build.sbt")) {
		return LangScala
	}
	// Java: pom.xml or build.gradle (non-kts)
	if fileExists(filepath.Join(workspace, "pom.xml")) ||
		fileExists(filepath.Join(workspace, "build.gradle")) {
		return LangJava
	}
	// C#: .csproj or .sln
	matches, _ := filepath.Glob(filepath.Join(workspace, "*.csproj"))
	if len(matches) > 0 {
		return LangCSharp
	}
	matches, _ = filepath.Glob(filepath.Join(workspace, "*.sln"))
	if len(matches) > 0 {
		return LangCSharp
	}
	// Swift: Package.swift or *.xcodeproj
	if fileExists(filepath.Join(workspace, "Package.swift")) {
		return LangSwift
	}
	matches, _ = filepath.Glob(filepath.Join(workspace, "*.xcodeproj"))
	if len(matches) > 0 {
		return LangSwift
	}
	// PHP: composer.json
	if fileExists(filepath.Join(workspace, "composer.json")) {
		return LangPHP
	}
	// Ruby: Gemfile or .rb files with no other markers
	if fileExists(filepath.Join(workspace, "Gemfile")) ||
		fileExists(filepath.Join(workspace, "Rakefile")) {
		return LangRuby
	}
	// Dart/Flutter: pubspec.yaml
	if fileExists(filepath.Join(workspace, "pubspec.yaml")) {
		return LangDart
	}
	// Elixir: mix.exs
	if fileExists(filepath.Join(workspace, "mix.exs")) {
		return LangElixir
	}
	// C/C++: CMakeLists.txt, Makefile with .c/.h files
	if fileExists(filepath.Join(workspace, "CMakeLists.txt")) {
		return LangC
	}
	// Lua: .luarc.json or lua/ directory with .lua files
	if fileExists(filepath.Join(workspace, ".luarc.json")) {
		return LangLua
	}
	// JS/TS: package.json
	if fileExists(filepath.Join(workspace, "package.json")) {
		return LangJS
	}
	// Python: requirements.txt, setup.py, pyproject.toml
	for _, f := range []string{"requirements.txt", "setup.py", "pyproject.toml"} {
		if fileExists(filepath.Join(workspace, f)) {
			return LangPython
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// pathToURI converts a filesystem path to an LSP file:// URI.
func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// On Windows, paths use backslashes; URIs use forward slashes.
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}
