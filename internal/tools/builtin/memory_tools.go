package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abrandt/vla/internal/memory"
)

// MemoryTools holds the shared dependencies for all memory tools: the store
// and an optional embedding client. Each tool is a separate struct (per
// VLA's one-tool-per-file convention) but they share this context.
type MemoryTools struct {
	Store    *memory.Store
	Embedder *memory.EmbeddingClient // nil = keyword-only search, no embeddings on save
	Project  func() string           // resolves the current project name (CWD-based)
}

// MemorySave stores a memory for later retrieval. If an embedding client is
// configured, the content is embedded for semantic search.
type MemorySave struct{ Deps MemoryTools }

func (MemorySave) Name() string { return "memory_save" }

func (MemorySave) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The knowledge or fact to remember.",
			},
			"tags": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional tags to categorize this memory.",
			},
		},
		"required": []string{"content"},
	}
}

func (m MemorySave) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Content == "" {
		return "Error: content is required", nil
	}
	project := m.Deps.Project()

	mem := &memory.Memory{
		Project: project,
		Content: in.Content,
		Tags:    in.Tags,
	}
	// Generate embedding if available.
	if m.Deps.Embedder != nil {
		vec, err := m.Deps.Embedder.Embed(in.Content)
		if err == nil {
			mem.Embedding = vec
		}
		// Embedding failure is non-fatal — save without vector.
	}
	if err := m.Deps.Store.Save(mem); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	return fmt.Sprintf("saved memory %s (project: %s)", mem.ID, project), nil
}

// MemorySearch searches stored memories by keyword and (if embeddings are
// available) semantic similarity.
type MemorySearch struct{ Deps MemoryTools }

func (MemorySearch) Name() string { return "memory_search" }

func (MemorySearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "What to search for in memories.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results. Default 5.",
			},
		},
		"required": []string{"query"},
	}
}

func (m MemorySearch) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.Query == "" {
		return "Error: query is required", nil
	}
	if in.Limit <= 0 {
		in.Limit = 5
	}
	project := m.Deps.Project()

	var queryVec []float32
	if m.Deps.Embedder != nil {
		queryVec, _ = m.Deps.Embedder.Embed(in.Query) // non-fatal if fails
	}

	results, err := m.Deps.Store.Search(project, in.Query, queryVec, in.Limit, 0.7, 0.3)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if len(results) == 0 {
		return "no memories found", nil
	}
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. [score %.2f] %s\n", i+1, r.Score, truncateStr(r.Memory.Content, 200))
		if len(r.Memory.Tags) > 0 {
			fmt.Fprintf(&b, "   tags: %s\n", strings.Join(r.Memory.Tags, ", "))
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// MemoryList lists all stored memories for the current project.
type MemoryList struct{ Deps MemoryTools }

func (MemoryList) Name() string { return "memory_list" }

func (MemoryList) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max memories to return. Default 20.",
			},
		},
	}
}

func (m MemoryList) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	if in.Limit <= 0 {
		in.Limit = 20
	}
	project := m.Deps.Project()

	memories, err := m.Deps.Store.List(project)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if len(memories) == 0 {
		return "no memories stored", nil
	}
	if len(memories) > in.Limit {
		memories = memories[:in.Limit]
	}
	var b strings.Builder
	for i, mem := range memories {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, mem.ID, truncateStr(mem.Content, 100))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// MemoryDelete removes a stored memory by ID.
type MemoryDelete struct{ Deps MemoryTools }

func (MemoryDelete) Name() string { return "memory_delete" }

func (MemoryDelete) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The memory ID to delete.",
			},
		},
		"required": []string{"id"},
	}
}

func (m MemoryDelete) Execute(args json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: parse arguments: %v", err), nil
	}
	if in.ID == "" {
		return "Error: id is required", nil
	}
	project := m.Deps.Project()
	if err := m.Deps.Store.Delete(project, in.ID); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	return fmt.Sprintf("deleted memory %s", in.ID), nil
}
