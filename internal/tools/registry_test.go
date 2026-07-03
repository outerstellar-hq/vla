package tools

import (
	"encoding/json"
	"testing"
)

// stubTool is a test double implementing Tool.
type stubTool struct {
	name   string
	schema map[string]any
}

func (s *stubTool) Name() string           { return s.name }
func (s *stubTool) Schema() map[string]any { return s.schema }
func (s *stubTool) Execute(args json.RawMessage) (string, error) {
	return "ok", nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	t1 := &stubTool{name: "alpha", schema: map[string]any{"type": "object"}}
	r.Register(t1)

	got, ok := r.Get("alpha")
	if !ok {
		t.Fatal("expected to find registered tool alpha")
	}
	if got.Name() != "alpha" {
		t.Errorf("got name %q, want alpha", got.Name())
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected Get to return false for unregistered tool")
	}
}

func TestRegistry_Schemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "a", schema: map[string]any{"type": "object"}})
	r.Register(&stubTool{name: "b", schema: map[string]any{"type": "object"}})

	schemas := r.Schemas()
	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}
	for _, s := range schemas {
		fn, ok := s["function"].(map[string]any)
		if !ok || fn["name"] == nil {
			t.Errorf("schema missing function.name field: %v", s)
		}
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "dup", schema: map[string]any{}})
	err := r.Register(&stubTool{name: "dup", schema: map[string]any{}})
	if err == nil {
		t.Fatal("expected error registering duplicate tool name")
	}
}
