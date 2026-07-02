// Package session manages VLA session lifecycles: creating new sessions
// (each launch), capturing CWD, and managing the NDJSON transcript file.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Session represents one VLA conversation. Each launch creates a new
// Session; --resume reopens an existing transcript file.
type Session struct {
	id   string
	cwd  string
	path string // absolute path to the transcript .json file
}

// Option configures a new Session.
type Option func(*config)

type config struct {
	dir   string // directory to store the transcript file
	model string
}

// WithDir overrides the transcript storage directory (used by tests).
func WithDir(dir string) Option { return func(c *config) { c.dir = dir } }

// WithModel overrides the model recorded in the transcript metadata.
func WithModel(model string) Option { return func(c *config) { c.model = model } }

// New creates a fresh session: a timestamp-based ID, the current CWD,
// and a transcript file with the metadata line already written.
func New(opts ...Option) (*Session, error) {
	cfg := config{dir: SessionsDir()}
	for _, o := range opts {
		o(&cfg)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("session: get cwd: %w", err)
	}

	id := time.Now().UTC().Format("2006-01-02T150405Z")
	path := filepath.Join(cfg.dir, id+".json")

	s := &Session{id: id, cwd: cwd, path: path}
	if err := s.writeMeta(cfg.model); err != nil {
		return nil, err
	}
	return s, nil
}

// Open reopens an existing transcript file by path (used by --resume).
func Open(path string) (*Session, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("session: resolve %s: %w", path, err)
	}
	_, meta, err := readTranscript(abs)
	if err != nil {
		return nil, err
	}
	var id, cwd string
	if v, ok := meta["id"].(string); ok {
		id = v
	}
	if v, ok := meta["cwd"].(string); ok {
		cwd = v
	}
	return &Session{id: id, cwd: cwd, path: abs}, nil
}

// ID returns the session identifier (timestamp string).
func (s *Session) ID() string { return s.id }

// CWD returns the working directory captured at session creation.
func (s *Session) CWD() string { return s.cwd }

// Path returns the absolute path to the transcript file.
func (s *Session) Path() string { return s.path }

// SessionsDir returns the global sessions directory: ~/.vla/sessions.
func SessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vla", "sessions")
}

// writeMeta writes the first line of the transcript (the session metadata).
func (s *Session) writeMeta(model string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("session: create sessions dir: %w", err)
	}
	meta := map[string]any{
		"type":    "session",
		"id":      s.id,
		"cwd":     s.cwd,
		"model":   model,
		"created": time.Now().UTC().Format(time.RFC3339),
	}
	line, err := encodeLine(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(line, '\n'), 0644)
}
