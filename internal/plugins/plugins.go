// Package plugins implements VLA's plugin system. Plugins are user-defined
// tools that live as scripts in .vla/plugins/. Each plugin has a manifest
// (plugin.json) describing the tool schema and an executable script that
// runs when the tool is called.
//
// This is a script-based plugin system (not compiled Go plugins) — it's
// cross-platform, works with any language (shell, Python, Node), and needs
// no CGo. The script receives arguments as JSON on stdin and returns its
// result on stdout.
//
// Directory structure:
//
//	.vla/plugins/
//	  my_tool/
//	    plugin.json    # manifest: name, description, input_schema
//	    run.sh         # the executable (run.sh, run.py, run.js, etc.)
//
// The manifest format (plugin.json):
//
//	{
//	  "name": "my_tool",
//	  "description": "Does something useful",
//	  "input_schema": {
//	    "type": "object",
//	    "properties": {
//	      "input": {"type": "string", "description": "The input value"}
//	    },
//	    "required": ["input"]
//	  }
//	}
//
// The script receives the tool call arguments as a JSON object on stdin
// and must print its result to stdout.
package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abrandt/vla/internal/tools"
)

// Manifest describes a plugin tool. It's the content of plugin.json.
type Manifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Plugin is one loaded plugin: its manifest + the path to its script.
type Plugin struct {
	Manifest Manifest
	Script   string // absolute path to the executable script
	Dir      string // plugin directory
}

// Load discovers all plugins in .vla/plugins/. Each subdirectory with a
// plugin.json and an executable script is loaded. Returns the list of
// valid plugins.
func Load(root string) []Plugin {
	pluginsDir := filepath.Join(root, ".vla", "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil // no plugins directory = no plugins
	}

	var plugins []Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, entry.Name())
		plugin, err := loadPlugin(dir)
		if err != nil {
			continue // skip invalid plugins silently
		}
		plugins = append(plugins, plugin)
	}
	return plugins
}

// loadPlugin loads one plugin from its directory.
func loadPlugin(dir string) (Plugin, error) {
	// Read manifest.
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Plugin{}, fmt.Errorf("read plugin.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Plugin{}, fmt.Errorf("parse plugin.json: %w", err)
	}
	if manifest.Name == "" {
		return Plugin{}, fmt.Errorf("plugin name is required")
	}

	// Find the executable script.
	script := findScript(dir)
	if script == "" {
		return Plugin{}, fmt.Errorf("no executable script found (run.sh, run.py, run.js, etc.)")
	}

	return Plugin{
		Manifest: manifest,
		Script:   script,
		Dir:      dir,
	}, nil
}

// findScript looks for run.sh, run.py, run.js, run.cmd, or run.bat in the
// plugin directory. Returns the first match, or empty if none found.
func findScript(dir string) string {
	candidates := []string{"run.sh", "run.py", "run.js", "run.ts", "run.rb", "run.pl"}
	if runtime.GOOS == "windows" {
		candidates = append([]string{"run.cmd", "run.bat", "run.ps1"}, candidates...)
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// PluginTool adapts a Plugin to VLA's tools.Tool interface. When executed,
// it passes the arguments as JSON on stdin to the script and returns stdout.
type PluginTool struct {
	Plugin Plugin
}

// Name returns the plugin's tool name.
func (p PluginTool) Name() string { return p.Plugin.Manifest.Name }

// Schema returns the plugin's input schema.
func (p PluginTool) Schema() map[string]any {
	if p.Plugin.Manifest.InputSchema != nil {
		return p.Plugin.Manifest.InputSchema
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// Execute runs the plugin script with the given arguments as JSON on stdin.
// Returns the script's stdout as the tool result. Errors are returned as
// result strings (per VLA convention).
func (p PluginTool) Execute(args json.RawMessage) (string, error) {
	// Determine how to run the script based on its extension.
	cmd := buildCommand(p.Plugin.Script, p.Plugin.Dir)
	if cmd == nil {
		return fmt.Sprintf("Error: could not determine how to run %s", filepath.Base(p.Plugin.Script)), nil
	}

	// Pass arguments as JSON on stdin.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Sprintf("Error: stdin pipe: %v", err), nil
	}
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, string(args))
	}()

	// Capture stdout and stderr.
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Sprintf("Error: %s: %s", p.Plugin.Manifest.Name, errMsg), nil
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "(plugin returned no output)", nil
	}
	return result, nil
}

// buildCommand creates the exec.Cmd for the script based on its extension.
func buildCommand(script, dir string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(script))
	switch ext {
	case ".sh":
		return exec.Command("sh", script)
	case ".py":
		return exec.Command("python3", script)
	case ".js":
		return exec.Command("node", script)
	case ".ts":
		return exec.Command("npx", "ts-node", script)
	case ".cmd", ".bat":
		return exec.Command("cmd", "/c", script)
	case ".ps1":
		return exec.Command("powershell", "-File", script)
	default:
		// Try making it directly executable.
		return exec.Command(script)
	}
}

// RegisterAll wraps every plugin as a PluginTool and registers it with
// the VLA tool registry.
func RegisterAll(registry *tools.Registry, plugins []Plugin) error {
	for _, p := range plugins {
		tool := PluginTool{Plugin: p}
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("plugins: register %s: %w", tool.Name(), err)
		}
	}
	return nil
}

// Compile-time check.
var _ tools.Tool = PluginTool{}
