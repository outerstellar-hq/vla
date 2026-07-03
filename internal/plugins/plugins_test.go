package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/tools"
)

func createPlugin(t *testing.T, root, name, scriptContent, manifest string) {
	t.Helper()
	dir := filepath.Join(root, ".vla", "plugins", name)
	os.MkdirAll(dir, 0755)

	if manifest == "" {
		manifest = `{
			"name": "` + name + `",
			"description": "Test plugin",
			"input_schema": {
				"type": "object",
				"properties": {
					"input": {"type": "string"}
				}
			}
		}`
	}
	os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0644)

	// Pick script extension based on OS.
	if runtime.GOOS == "windows" {
		os.WriteFile(filepath.Join(dir, "run.cmd"), []byte(scriptContent), 0644)
	} else {
		os.WriteFile(filepath.Join(dir, "run.sh"), []byte(scriptContent), 0755)
	}
}

func TestLoad_NoPluginsDir(t *testing.T) {
	plugins := Load(t.TempDir())
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoad_EmptyPluginsDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".vla", "plugins"), 0755)
	plugins := Load(dir)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoad_ValidPlugin(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		createPlugin(t, dir, "upper", "@echo off\nset /p INPUT=\necho %INPUT%", "")
	} else {
		createPlugin(t, dir, "upper", "#!/bin/sh\ncat | tr a-z A-Z", "")
	}

	plugins := Load(dir)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Name != "upper" {
		t.Errorf("name = %q", plugins[0].Manifest.Name)
	}
	if plugins[0].Script == "" {
		t.Error("script path is empty")
	}
}

func TestLoad_MissingManifest(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, ".vla", "plugins", "broken")
	os.MkdirAll(pluginDir, 0755)
	// No plugin.json, just a script.
	os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/bin/sh\necho hi"), 0755)

	plugins := Load(dir)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins (no manifest), got %d", len(plugins))
	}
}

func TestLoad_MissingScript(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, ".vla", "plugins", "noScript")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"name":"test"}`), 0644)

	plugins := Load(dir)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins (no script), got %d", len(plugins))
	}
}

func TestPluginTool_Name(t *testing.T) {
	p := Plugin{Manifest: Manifest{Name: "my_tool"}}
	tool := PluginTool{Plugin: p}
	if tool.Name() != "my_tool" {
		t.Errorf("got %q", tool.Name())
	}
}

func TestPluginTool_Schema(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"x": map[string]any{"type": "string"},
	}}
	p := Plugin{Manifest: Manifest{Name: "t", InputSchema: schema}}
	tool := PluginTool{Plugin: p}
	got := tool.Schema()
	if got["type"] != "object" {
		t.Errorf("schema type = %v", got["type"])
	}
}

func TestPluginTool_Schema_Default(t *testing.T) {
	p := Plugin{Manifest: Manifest{Name: "t"}}
	tool := PluginTool{Plugin: p}
	got := tool.Schema()
	if got["type"] != "object" {
		t.Errorf("default schema should be object, got %v", got["type"])
	}
}

func TestRegisterAll(t *testing.T) {
	plugins := []Plugin{
		{Manifest: Manifest{Name: "plugin_a"}, Script: "/dev/null"},
		{Manifest: Manifest{Name: "plugin_b"}, Script: "/dev/null"},
	}
	reg := tools.NewRegistry()
	if err := RegisterAll(reg, plugins); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if _, ok := reg.Get("plugin_a"); !ok {
		t.Error("plugin_a not registered")
	}
	if _, ok := reg.Get("plugin_b"); !ok {
		t.Error("plugin_b not registered")
	}
}

func TestRegisterAll_Empty(t *testing.T) {
	reg := tools.NewRegistry()
	if err := RegisterAll(reg, nil); err != nil {
		t.Fatalf("RegisterAll nil: %v", err)
	}
	if len(reg.Schemas()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(reg.Schemas()))
	}
}

func TestRegisterAll_DuplicateName(t *testing.T) {
	plugins := []Plugin{
		{Manifest: Manifest{Name: "dup"}, Script: "/dev/null"},
		{Manifest: Manifest{Name: "dup"}, Script: "/dev/null"},
	}
	reg := tools.NewRegistry()
	err := RegisterAll(reg, plugins)
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestPluginTool_Execute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell plugin execution test is Unix-only")
	}
	dir := t.TempDir()
	createPlugin(t, dir, "echo_plugin", "#!/bin/sh\nread INPUT\necho \"result: $INPUT\"", "")

	plugins := Load(dir)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	tool := PluginTool{Plugin: plugins[0]}
	result, err := tool.Execute(json.RawMessage(`{"input":"hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected input echoed, got %q", result)
	}
}

func TestFindScript(t *testing.T) {
	dir := t.TempDir()
	// Create run.sh.
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh"), 0755)
	script := findScript(dir)
	if !strings.HasSuffix(script, "run.sh") {
		t.Errorf("expected run.sh, got %q", script)
	}
}

func TestFindScript_NoneFound(t *testing.T) {
	dir := t.TempDir()
	if findScript(dir) != "" {
		t.Error("expected empty when no script found")
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		script  string
		wantCmd string
	}{
		{"run.sh", "sh"},
		{"run.py", "python3"},
		{"run.js", "node"},
	}
	for _, tt := range tests {
		cmd := buildCommand(tt.script, ".")
		if cmd == nil {
			t.Errorf("buildCommand(%q) = nil", tt.script)
			continue
		}
		if cmd.Path == "" && len(cmd.Args) == 0 {
			t.Errorf("buildCommand(%q) returned empty cmd", tt.script)
		}
	}
}
