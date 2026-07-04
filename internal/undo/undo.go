// Package undo provides a simple change stack for rollback of file
// modifications made by the agent. Before each write_file, update_file,
// or delete_file, the tool records the file's previous state. /undo
// restores it.
package undo

import (
	"os"
	"path/filepath"
	"sync"
)

// Change records one file modification for potential rollback.
type Change struct {
	Path    string // absolute path
	OldData []byte // previous content (nil if file didn't exist)
	Existed bool   // whether the file existed before the change
}

// Stack is a thread-safe stack of changes. Each session has one.
type Stack struct {
	mu      sync.Mutex
	changes []Change
}

// NewStack creates an empty undo stack.
func NewStack() *Stack {
	return &Stack{}
}

// Push records a file's state before a modification. Reads the current
// file content (if it exists) so it can be restored on undo.
func (s *Stack) Push(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(abs)
	existed := true
	if err != nil {
		if os.IsNotExist(err) {
			existed = false
			data = nil
		} else {
			return err
		}
	}

	s.mu.Lock()
	s.changes = append(s.changes, Change{
		Path:    abs,
		OldData: data,
		Existed: existed,
	})
	s.mu.Unlock()
	return nil
}

// Pop removes and returns the most recent change. Returns false if empty.
func (s *Stack) Pop() (Change, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.changes) == 0 {
		return Change{}, false
	}

	idx := len(s.changes) - 1
	c := s.changes[idx]
	s.changes = s.changes[:idx]
	return c, true
}

// Peek returns the most recent change without removing it. Returns false if empty.
func (s *Stack) Peek() (Change, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.changes) == 0 {
		return Change{}, false
	}
	return s.changes[len(s.changes)-1], true
}

// Len returns the number of changes on the stack.
func (s *Stack) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.changes)
}

// Undo restores the most recent change. Returns the path that was restored,
// or empty string if the stack was empty.
func (s *Stack) Undo() (string, error) {
	c, ok := s.Pop()
	if !ok {
		return "", nil
	}

	if !c.Existed {
		// File was created — delete it.
		return c.Path, os.Remove(c.Path)
	}

	// File was modified or overwritten — restore old content.
	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return c.Path, err
	}
	return c.Path, os.WriteFile(c.Path, c.OldData, 0o644)
}

// UndoAll restores all changes in reverse order.
func (s *Stack) UndoAll() ([]string, error) {
	var restored []string
	for {
		path, err := s.Undo()
		if err != nil {
			return restored, err
		}
		if path == "" {
			break
		}
		restored = append(restored, path)
	}
	return restored, nil
}

// Clear empties the stack without restoring.
func (s *Stack) Clear() {
	s.mu.Lock()
	s.changes = nil
	s.mu.Unlock()
}
