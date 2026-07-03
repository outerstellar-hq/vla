package builtin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/memory"
)

func memoryDeps(t *testing.T) (MemoryTools, *memory.Store) {
	t.Helper()
	store := memory.NewStore(t.TempDir())
	return MemoryTools{
		Store:   store,
		Project: func() string { return "testproj" },
	}, store
}

func TestMemorySave_Success(t *testing.T) {
	deps, _ := memoryDeps(t)
	m := MemorySave{Deps: deps}
	out, err := m.Execute(json.RawMessage(`{"content":"The API uses REST","tags":["api","rest"]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "saved memory") {
		t.Errorf("got %q", out)
	}
}

func TestMemorySave_EmptyContent(t *testing.T) {
	deps, _ := memoryDeps(t)
	m := MemorySave{Deps: deps}
	out, _ := m.Execute(json.RawMessage(`{"content":""}`))
	if !strings.Contains(out, "content is required") {
		t.Errorf("got %q", out)
	}
}

func TestMemorySearch_FindsSaved(t *testing.T) {
	deps, _ := memoryDeps(t)
	save := MemorySave{Deps: deps}
	_, _ = save.Execute(json.RawMessage(`{"content":"PostgreSQL connection string","tags":["db"]}`))

	search := MemorySearch{Deps: deps}
	out, _ := search.Execute(json.RawMessage(`{"query":"PostgreSQL"}`))
	if !strings.Contains(out, "PostgreSQL") {
		t.Errorf("expected to find saved memory, got %q", out)
	}
}

func TestMemorySearch_NoMemories(t *testing.T) {
	deps, _ := memoryDeps(t)
	search := MemorySearch{Deps: deps}
	out, _ := search.Execute(json.RawMessage(`{"query":"anything"}`))
	if out != "no memories found" {
		t.Errorf("got %q", out)
	}
}

func TestMemoryList_ShowsMemories(t *testing.T) {
	deps, _ := memoryDeps(t)
	save := MemorySave{Deps: deps}
	_, _ = save.Execute(json.RawMessage(`{"content":"alpha memory"}`))
	_, _ = save.Execute(json.RawMessage(`{"content":"beta memory"}`))

	list := MemoryList{Deps: deps}
	out, _ := list.Execute(json.RawMessage(`{}`))
	// Both memories should appear regardless of sort order.
	if !strings.Contains(out, "alpha memory") {
		t.Errorf("missing alpha memory: %q", out)
	}
	if !strings.Contains(out, "beta memory") {
		t.Errorf("missing beta memory: %q", out)
	}
}

func TestMemoryList_Empty(t *testing.T) {
	deps, _ := memoryDeps(t)
	list := MemoryList{Deps: deps}
	out, _ := list.Execute(json.RawMessage(`{}`))
	if out != "no memories stored" {
		t.Errorf("got %q", out)
	}
}

func TestMemoryDelete_Removes(t *testing.T) {
	deps, _ := memoryDeps(t)
	save := MemorySave{Deps: deps}
	out, _ := save.Execute(json.RawMessage(`{"content":"to be deleted"}`))
	// Extract the ID from the save response.
	idStart := strings.Index(out, "memory ") + 7
	idEnd := strings.Index(out[idStart:], " ") + idStart
	id := out[idStart:idEnd]

	del := MemoryDelete{Deps: deps}
	out2, _ := del.Execute(json.RawMessage(`{"id":"` + id + `"}`))
	if !strings.Contains(out2, "deleted") {
		t.Errorf("got %q", out2)
	}

	// Verify it's gone.
	list := MemoryList{Deps: deps}
	out3, _ := list.Execute(json.RawMessage(`{}`))
	if out3 != "no memories stored" {
		t.Errorf("expected empty after delete, got %q", out3)
	}
}
