// Package app holds the wiring logic that ties VLA's packages together:
// config discovery, session open/create, tool registration, and memory/LSP
// setup. Extracted from main so it can be unit-tested deterministically.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/indexer"
	"github.com/abrandt/vla/internal/lsp"
	"github.com/abrandt/vla/internal/memory"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
	"github.com/abrandt/vla/internal/undo"
)

// Deps bundles all the shared dependencies that RegisterBuiltins needs.
// Each is optional — nil deps result in tools that gracefully report
// unavailability rather than crashing.
type Deps struct {
	BaseDir    string
	Indexer    *indexer.Indexer
	LSPManager *lsp.Manager
	MemStore   *memory.Store
	Embedder   *memory.EmbeddingClient
	Project    func() string // resolves current project name from CWD
	UndoStack  *undo.Stack   // optional; enables /undo for file tools
}

// ResolveConfigPath finds config.json in priority order:
//  1. explicit path (if non-empty)
//  2. ./config.json in the current working directory
//  3. ~/.vla/config.json
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

// RegisterBuiltins adds all built-in tools to the registry, wiring each to
// its dependencies. To add a tool: implement tools.Tool in its own file under
// builtin/, then add one line to the slice here.
func RegisterBuiltins(r *tools.Registry, deps Deps) error {
	fsCtx := builtin.Ctx{BaseDir: deps.BaseDir, UndoStack: deps.UndoStack}
	memDeps := builtin.MemoryTools{
		Store:    deps.MemStore,
		Embedder: deps.Embedder,
		Project:  deps.Project,
	}
	builtins := []tools.Tool{
		builtin.Echo{},
		builtin.ReadFile{Ctx: fsCtx},
		builtin.WriteFile{Ctx: fsCtx},
		builtin.UpdateFile{Ctx: fsCtx},
		builtin.DeleteFile{Ctx: fsCtx},
		builtin.ListFiles{Ctx: fsCtx},
		builtin.Search{Ctx: fsCtx},
		builtin.GitStatus{Ctx: fsCtx},
		builtin.GitDiff{Ctx: fsCtx},
		builtin.GitCommit{Ctx: fsCtx},
		builtin.WebSearch{},
		builtin.WebRead{},
		// Memory tools
		builtin.MemorySave{Deps: memDeps},
		builtin.MemorySearch{Deps: memDeps},
		builtin.MemoryList{Deps: memDeps},
		builtin.MemoryDelete{Deps: memDeps},
		// Navigation (LSP-prefer, regex-fallback)
		builtin.GoToDefinition{Index: deps.Indexer, Manager: deps.LSPManager, BaseDir: deps.BaseDir},
		builtin.FindReferences{Index: deps.Indexer, Manager: deps.LSPManager, BaseDir: deps.BaseDir},
		// LSP-only tools (report error if no server available)
		builtin.Hover{Manager: deps.LSPManager, BaseDir: deps.BaseDir},
		builtin.Diagnostics{Manager: deps.LSPManager, BaseDir: deps.BaseDir},
	}
	for _, t := range builtins {
		if err := r.Register(t); err != nil {
			return fmt.Errorf("register %s: %w", t.Name(), err)
		}
	}
	return nil
}
