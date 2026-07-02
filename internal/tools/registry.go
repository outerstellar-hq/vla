package tools

import "fmt"

// Registry collects all tools and exposes their schemas to the LLM.
// Adding a tool = implement Tool in its own file + one Register call.
type Registry struct {
	tools map[string]Tool
	order []string // preserves registration order for stable schema output
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Registering a duplicate name
// returns an error — each tool name must be unique.
func (r *Registry) Register(t Tool) error {
	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tools: duplicate registration for %q", name)
	}
	r.tools[name] = t
	r.order = append(r.order, name)
	return nil
}

// Get returns the tool registered under name, plus whether it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Schemas returns the OpenAI function-calling tool definitions for all
// registered tools, in registration order. Each entry is the full tool
// object: {"type": "function", "function": {"name": ..., "parameters": <schema>}}.
func (r *Registry) Schemas() []map[string]any {
	out := make([]map[string]any, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":       t.Name(),
				"parameters": t.Schema(),
			},
		})
	}
	return out
}
