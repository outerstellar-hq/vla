package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/indexer"
)

func newIndexedDir(t *testing.T) (*indexer.Indexer, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "app/models.py", "class User:\n    def __init__(self):\n        pass\n")
	writeFile(t, root, "app/views.py", "from app.models import User\n\ndef render(u):\n    return User()\n")
	writeFile(t, root, "main.py", "from app.views import render\nu = render()\n")
	ix := indexer.New(root)
	if _, err := ix.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return ix, root
}

func TestGoToDefinition_FindsFunction(t *testing.T) {
	ix, _ := newIndexedDir(t)
	g := GoToDefinition{Index: ix}
	out, err := g.Execute(json.RawMessage(`{"symbol":"render"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "render") || !strings.Contains(out, "app/views.py") {
		t.Errorf("expected render in app/views.py, got %q", out)
	}
}

func TestGoToDefinition_FindsClass(t *testing.T) {
	ix, _ := newIndexedDir(t)
	g := GoToDefinition{Index: ix}
	out, _ := g.Execute(json.RawMessage(`{"symbol":"User"}`))
	if !strings.Contains(out, "class User") || !strings.Contains(out, "app/models.py") {
		t.Errorf("expected class User in models.py, got %q", out)
	}
}

func TestGoToDefinition_NotFound(t *testing.T) {
	ix, _ := newIndexedDir(t)
	g := GoToDefinition{Index: ix}
	out, _ := g.Execute(json.RawMessage(`{"symbol":"nonexistent"}`))
	if !strings.Contains(out, "no definition found") {
		t.Errorf("got %q", out)
	}
}

func TestGoToDefinition_EmptySymbol(t *testing.T) {
	ix, _ := newIndexedDir(t)
	g := GoToDefinition{Index: ix}
	out, _ := g.Execute(json.RawMessage(`{"symbol":""}`))
	if !strings.Contains(out, "symbol is required") {
		t.Errorf("got %q", out)
	}
}

func TestGoToDefinition_NilIndex(t *testing.T) {
	g := GoToDefinition{}
	out, _ := g.Execute(json.RawMessage(`{"symbol":"foo"}`))
	if !strings.Contains(out, "index not available") {
		t.Errorf("got %q", out)
	}
}

func TestFindReferences_ReturnsUsages(t *testing.T) {
	ix, _ := newIndexedDir(t)
	f := FindReferences{Index: ix}
	out, err := f.Execute(json.RawMessage(`{"symbol":"render"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "reference") {
		t.Errorf("expected reference count, got %q", out)
	}
	// render is used in main.py.
	if !strings.Contains(out, "main.py") {
		t.Errorf("expected main.py reference, got %q", out)
	}
}

func TestFindReferences_NoUsages(t *testing.T) {
	ix, root := newIndexedDir(t)
	// Add a function that's never called.
	writeFile(t, root, "unused.py", "def never_called():\n    pass\n")
	_ = ix.ReindexFile(filepath.Join(root, "unused.py"))
	f := FindReferences{Index: ix}
	out, _ := f.Execute(json.RawMessage(`{"symbol":"never_called"}`))
	if !strings.Contains(out, "no references") && !strings.Contains(out, "defined at") {
		t.Errorf("expected no-references message, got %q", out)
	}
}

func TestFindReferences_UnknownSymbol(t *testing.T) {
	ix, _ := newIndexedDir(t)
	f := FindReferences{Index: ix}
	out, _ := f.Execute(json.RawMessage(`{"symbol":"ghost_xyz"}`))
	if !strings.Contains(out, "no definition or references") {
		t.Errorf("got %q", out)
	}
}

func TestFindReferences_EmptySymbol(t *testing.T) {
	ix, _ := newIndexedDir(t)
	f := FindReferences{Index: ix}
	out, _ := f.Execute(json.RawMessage(`{"symbol":""}`))
	if !strings.Contains(out, "symbol is required") {
		t.Errorf("got %q", out)
	}
}

// Verify the navigation tools work with a Go codebase too.
func TestGoToDefinition_GoCodebase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "server.go", "package main\n\ntype Server struct{}\n\nfunc (s *Server) Run() {}\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() {\n    s := Server{}\n    s.Run()\n}\n")
	ix := indexer.New(root)
	_, _ = ix.Build()

	g := GoToDefinition{Index: ix}
	out, _ := g.Execute(json.RawMessage(`{"symbol":"Run"}`))
	if !strings.Contains(out, "server.go") {
		t.Errorf("expected Run in server.go, got %q", out)
	}

	f := FindReferences{Index: ix}
	out2, _ := f.Execute(json.RawMessage(`{"symbol":"Server"}`))
	if !strings.Contains(out2, "main.go") {
		t.Errorf("expected Server reference in main.go, got %q", out2)
	}
}

// silence unused import
var _ = os.Stat
