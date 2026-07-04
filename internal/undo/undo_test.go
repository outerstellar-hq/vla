package undo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStack_PushAndUndo_Modified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Create initial file.
	os.WriteFile(path, []byte("original"), 0o644)

	s := NewStack()
	s.Push(path)

	// Modify the file.
	os.WriteFile(path, []byte("modified"), 0o644)

	// Undo should restore "original".
	restored, err := s.Undo()
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if restored != path {
		t.Errorf("expected path %s, got %s", path, restored)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Errorf("expected 'original', got %q", string(data))
	}
}

func TestStack_PushAndUndo_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	s := NewStack()

	// Push before file exists (simulating write_file creating a new file).
	// Push reads the file — it doesn't exist, so Existed=false.
	if err := s.Push(path); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Create the file.
	os.WriteFile(path, []byte("created"), 0o644)

	// Undo should delete it.
	restored, err := s.Undo()
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if restored != path {
		t.Errorf("expected path %s, got %s", path, restored)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted after undo")
	}
}

func TestStack_Empty(t *testing.T) {
	s := NewStack()
	if s.Len() != 0 {
		t.Error("new stack should be empty")
	}

	c, ok := s.Pop()
	if ok {
		t.Error("Pop on empty stack should return false")
	}
	if c.Path != "" {
		t.Error("Pop on empty should return zero-value Change")
	}

	path, err := s.Undo()
	if err != nil {
		t.Errorf("Undo on empty should not error: %v", err)
	}
	if path != "" {
		t.Error("Undo on empty should return empty path")
	}
}

func TestStack_MultipleUndos(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")

	os.WriteFile(f1, []byte("a1"), 0o644)
	os.WriteFile(f2, []byte("b1"), 0o644)

	s := NewStack()
	s.Push(f1)
	os.WriteFile(f1, []byte("a2"), 0o644)
	s.Push(f2)
	os.WriteFile(f2, []byte("b2"), 0o644)

	if s.Len() != 2 {
		t.Errorf("expected 2 changes, got %d", s.Len())
	}

	// Undo most recent (f2).
	s.Undo()
	data, _ := os.ReadFile(f2)
	if string(data) != "b1" {
		t.Errorf("f2: expected 'b1', got %q", string(data))
	}

	// Undo f1.
	s.Undo()
	data, _ = os.ReadFile(f1)
	if string(data) != "a1" {
		t.Errorf("f1: expected 'a1', got %q", string(data))
	}
}

func TestStack_UndoAll(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "x.txt")
	f2 := filepath.Join(dir, "y.txt")

	os.WriteFile(f1, []byte("x1"), 0o644)
	os.WriteFile(f2, []byte("y1"), 0o644)

	s := NewStack()
	s.Push(f1)
	os.WriteFile(f1, []byte("x2"), 0o644)
	s.Push(f2)
	os.WriteFile(f2, []byte("y2"), 0o644)

	restored, err := s.UndoAll()
	if err != nil {
		t.Fatalf("UndoAll: %v", err)
	}
	if len(restored) != 2 {
		t.Errorf("expected 2 restored, got %d", len(restored))
	}

	// Both files should have original content.
	data, _ := os.ReadFile(f1)
	if string(data) != "x1" {
		t.Errorf("f1: expected 'x1', got %q", string(data))
	}
	data, _ = os.ReadFile(f2)
	if string(data) != "y1" {
		t.Errorf("f2: expected 'y1', got %q", string(data))
	}
}

func TestStack_Peek(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peek.txt")
	os.WriteFile(path, []byte("data"), 0o644)

	s := NewStack()
	s.Push(path)

	c, ok := s.Peek()
	if !ok {
		t.Fatal("Peek should return true")
	}
	if c.Path != path {
		t.Errorf("expected path %s, got %s", path, c.Path)
	}
	if !c.Existed {
		t.Error("should have existed")
	}

	// Peek should not remove from stack.
	if s.Len() != 1 {
		t.Error("Peek should not change stack length")
	}
}

func TestStack_Clear(t *testing.T) {
	s := NewStack()
	s.changes = append(s.changes, Change{Path: "/tmp/a"})
	s.Clear()

	if s.Len() != 0 {
		t.Error("Clear should empty the stack")
	}
}
