// Package app holds the wiring logic that ties VLA's packages together:
// config discovery, session open/create, and tool registration. Extracted
// from main so it can be unit-tested deterministically.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

// ResolveConfigPath finds config.json in priority order:
//  1. explicit path (if non-empty)
//  2. ./config.json in the current working directory
//  3. ~/.vla/config.json
//
// It never returns an empty string; if nothing exists it returns the
// fallback path so config.Load can report the missing-file error.
func ResolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vla", "config.json")
	}
	return "config.json"
}

// OpenOrCreateSession opens an existing session by ID (--resume) or creates
// a new one. model is recorded in the transcript metadata for new sessions.
func OpenOrCreateSession(resumeID, model string) (*session.Session, error) {
	if resumeID != "" {
		path := filepath.Join(session.SessionsDir(), resumeID+".json")
		return session.Open(path)
	}
	return session.New(session.WithModel(model))
}

// RegisterBuiltins adds all built-in tools to the registry, wiring the
// filesystem/git tools to baseDir (the project root they operate in) and
// the navigation tools to the background indexer. To add a tool: implement
// tools.Tool in its own file under builtin/, then add one line to the slice.
func RegisterBuiltins(r *tools.Registry, baseDir string, ix *indexer.Indexer) error {
	ctx := builtin.Ctx{BaseDir: baseDir}
	builtins := []tools.Tool{
		builtin.Echo{},
		builtin.ReadFile{Ctx: ctx},
		builtin.WriteFile{Ctx: ctx},
		builtin.UpdateFile{Ctx: ctx},
		builtin.DeleteFile{Ctx: ctx},
		builtin.ListFiles{Ctx: ctx},
		builtin.Search{Ctx: ctx},
		builtin.GitStatus{Ctx: ctx},
		builtin.GitDiff{Ctx: ctx},
		builtin.GitCommit{Ctx: ctx},
		builtin.WebSearch{},
		builtin.WebRead{},
		builtin.GoToDefinition{Index: ix},
		builtin.FindReferences{Index: ix},
	}
	for _, t := range builtins {
		if err := r.Register(t); err != nil {
			return fmt.Errorf("register %s: %w", t.Name(), err)
		}
	}
	return nil
}
