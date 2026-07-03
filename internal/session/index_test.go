package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndex_LoadEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	idx := LoadIndex()
	if len(idx.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(idx.Entries))
	}
}

func TestIndex_RecordAndList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	// Pre-create the sessions dir so SessionsDir works.
	os.MkdirAll(filepath.Join(dir, ".vla", "sessions"), 0755)

	idx := LoadIndex()
	idx.Record("sess-1", "/project/a", "gpt-4o")
	idx.Record("sess-2", "/project/b", "claude-sonnet-4-5")
	idx.Record("sess-3", "/project/a", "gpt-4o")

	if len(idx.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(idx.Entries))
	}

	// Reload to verify persistence.
	idx2 := LoadIndex()
	if len(idx2.Entries) != 3 {
		t.Errorf("expected 3 entries after reload, got %d", len(idx2.Entries))
	}
	if idx2.Entries["sess-1"].Project != "/project/a" {
		t.Errorf("sess-1 project = %q", idx2.Entries["sess-1"].Project)
	}
}

func TestIndex_ListByProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	os.MkdirAll(filepath.Join(dir, ".vla", "sessions"), 0755)

	idx := LoadIndex()
	idx.Record("sess-1", "/project/a", "gpt-4o")
	idx.Record("sess-2", "/project/b", "gpt-4o")
	idx.Record("sess-3", "/project/a", "gpt-4o")

	projA := idx.ListByProject("/project/a")
	if len(projA) != 2 {
		t.Errorf("expected 2 sessions for project/a, got %d", len(projA))
	}
	projB := idx.ListByProject("/project/b")
	if len(projB) != 1 {
		t.Errorf("expected 1 session for project/b, got %d", len(projB))
	}
}

func TestIndex_Remove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	os.MkdirAll(filepath.Join(dir, ".vla", "sessions"), 0755)

	idx := LoadIndex()
	idx.Record("sess-1", "/p", "m")
	idx.Record("sess-2", "/p", "m")

	idx.Remove("sess-1")
	if _, ok := idx.Entries["sess-1"]; ok {
		t.Error("sess-1 should be removed")
	}
	if len(idx.Entries) != 1 {
		t.Errorf("expected 1 entry after remove, got %d", len(idx.Entries))
	}
}

func TestIndex_ListSortedByLastActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	os.MkdirAll(filepath.Join(dir, ".vla", "sessions"), 0755)

	idx := LoadIndex()
	idx.Record("old", "/p", "m")
	idx.Record("newest", "/p", "m")

	list := idx.List()
	if list[0].ID != "newest" {
		t.Errorf("expected newest first, got %q", list[0].ID)
	}
}
