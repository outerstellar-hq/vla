package builtin

import (
	"testing"

	"github.com/abrandt/vla/internal/tools"
)

// TestAllToolSchemasValid verifies that every built-in tool returns a valid
// Name() and Schema(). This covers the Name()/Schema() methods which are
// trivial but were at 0% coverage. More importantly, it catches schema
// regressions early (malformed JSON schema = the LLM can't use the tool).
func TestAllToolSchemasValid(t *testing.T) {
	ctx := Ctx{BaseDir: t.TempDir()}
	memDeps := MemoryTools{Store: nil, Project: func() string { return "test" }}

	allTools := []tools.Tool{
		Echo{},
		ReadFile{Ctx: ctx},
		WriteFile{Ctx: ctx},
		UpdateFile{Ctx: ctx},
		DeleteFile{Ctx: ctx},
		ListFiles{Ctx: ctx},
		Search{Ctx: ctx},
		GitStatus{Ctx: ctx},
		GitDiff{Ctx: ctx},
		GitCommit{Ctx: ctx},
		WebSearch{},
		WebRead{},
		MemorySave{Deps: memDeps},
		MemorySearch{Deps: memDeps},
		MemoryList{Deps: memDeps},
		MemoryDelete{Deps: memDeps},
		GoToDefinition{Index: nil, BaseDir: ctx.BaseDir},
		FindReferences{Index: nil, BaseDir: ctx.BaseDir},
		Hover{BaseDir: ctx.BaseDir},
		Diagnostics{BaseDir: ctx.BaseDir},
	}

	for _, tool := range allTools {
		t.Run(tool.Name(), func(t *testing.T) {
			name := tool.Name()
			if name == "" {
				t.Error("Name() returned empty string")
			}
			schema := tool.Schema()
			if schema == nil {
				t.Fatal("Schema() returned nil")
			}
			if schema["type"] == nil {
				t.Error("Schema missing 'type' field")
			}
			if schema["type"] != "object" {
				t.Errorf("Schema type = %v, want 'object'", schema["type"])
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("Schema missing 'properties' map")
			}
			if len(props) == 0 {
				t.Error("Schema has no properties")
			}
		})
	}
}

// TestRegistryIntegration_RegisterAllAndSchemasShape verifies the full set
// of tools registers cleanly and the registry wraps schemas correctly for
// the OpenAI API.
func TestRegistryIntegration_RegisterAllAndSchemasShape(t *testing.T) {
	r := tools.NewRegistry()
	ctx := Ctx{BaseDir: t.TempDir()}
	memDeps := MemoryTools{Project: func() string { return "t" }}

	allTools := []tools.Tool{
		Echo{}, ReadFile{Ctx: ctx}, WriteFile{Ctx: ctx}, UpdateFile{Ctx: ctx},
		DeleteFile{Ctx: ctx}, ListFiles{Ctx: ctx}, Search{Ctx: ctx},
		GitStatus{Ctx: ctx}, GitDiff{Ctx: ctx}, GitCommit{Ctx: ctx},
		WebSearch{}, WebRead{},
		MemorySave{Deps: memDeps}, MemorySearch{Deps: memDeps},
		MemoryList{Deps: memDeps}, MemoryDelete{Deps: memDeps},
		GoToDefinition{BaseDir: ctx.BaseDir}, FindReferences{BaseDir: ctx.BaseDir},
		Hover{BaseDir: ctx.BaseDir}, Diagnostics{BaseDir: ctx.BaseDir},
	}

	for _, tool := range allTools {
		if err := r.Register(tool); err != nil {
			t.Fatalf("Register(%s): %v", tool.Name(), err)
		}
	}

	schemas := r.Schemas()
	if len(schemas) != len(allTools) {
		t.Fatalf("expected %d schemas, got %d", len(allTools), len(schemas))
	}
	for _, s := range schemas {
		if s["type"] != "function" {
			t.Errorf("schema type = %v, want 'function'", s["type"])
		}
		fn, ok := s["function"].(map[string]any)
		if !ok {
			t.Fatal("missing function wrapper")
		}
		if fn["name"] == nil {
			t.Error("function.name is nil")
		}
		if fn["parameters"] == nil {
			t.Error("function.parameters is nil")
		}
	}
}
