package indexer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSrc(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestIndex_PythonFunctionsAndClasses(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "app/models.py", `class User:
    def __init__(self):
        self.name = ""

def login(user):
    return True

async def fetch_data():
    pass
`)
	ix := New(root)
	n, err := ix.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 file indexed, got %d", n)
	}

	syms := ix.Index().AllSymbols()
	// Expect: User (class), __init__ (func), login (func), fetch_data (func) = 4.
	if len(syms) != 4 {
		t.Errorf("expected 4 symbols, got %d: %+v", len(syms), syms)
	}

	defs := ix.Index().LookupDefinition("login")
	if len(defs) != 1 {
		t.Fatalf("expected 1 login def, got %d", len(defs))
	}
	if defs[0].Kind != SymbolFunction {
		t.Errorf("login kind = %v", defs[0].Kind)
	}
	if defs[0].Line != 5 {
		t.Errorf("login line = %d, want 5", defs[0].Line)
	}

	classDefs := ix.Index().LookupDefinition("User")
	if len(classDefs) != 1 || classDefs[0].Kind != SymbolClass {
		t.Errorf("User lookup failed: %+v", classDefs)
	}
}

func TestIndex_GoFunctionsAndTypes(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "main.go", `package main

type Server struct {
	addr string
}

func (s *Server) Start() error {
	return nil
}

func main() {
	s := &Server{}
	s.Start()
}
`)
	ix := New(root)
	_, _ = ix.Build()

	defs := ix.Index().LookupDefinition("Server")
	if len(defs) != 1 || defs[0].Kind != SymbolClass {
		t.Errorf("Server: %+v", defs)
	}
	defs = ix.Index().LookupDefinition("Start")
	if len(defs) != 1 || defs[0].Kind != SymbolFunction {
		t.Errorf("Start: %+v", defs)
	}
	defs = ix.Index().LookupDefinition("main")
	if len(defs) != 1 {
		t.Errorf("main: %+v", defs)
	}
}

func TestIndex_References(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "a.py", "def helper():\n    pass\n")
	writeSrc(t, root, "b.py", "from a import helper\nhelper()\n")
	ix := New(root)
	_, _ = ix.Build()

	refs := ix.Index().LookupReferences("helper")
	// At least one reference in b.py (the call or the import line).
	if len(refs) == 0 {
		t.Errorf("expected references to helper, got 0")
	}
	foundInB := false
	for _, r := range refs {
		if r.File == "b.py" {
			foundInB = true
		}
	}
	if !foundInB {
		t.Errorf("no reference to helper in b.py; refs: %+v", refs)
	}
}

func TestIndex_ReindexFile(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "f.py", "def foo():\n    pass\n")
	ix := New(root)
	_, _ = ix.Build()
	if len(ix.Index().LookupDefinition("foo")) != 1 {
		t.Fatal("expected foo before reindex")
	}

	// Rewrite: remove foo, add bar.
	writeSrc(t, root, "f.py", "def bar():\n    pass\n")
	if err := ix.ReindexFile(filepath.Join(root, "f.py")); err != nil {
		t.Fatalf("ReindexFile: %v", err)
	}
	if len(ix.Index().LookupDefinition("foo")) != 0 {
		t.Error("foo should be gone after reindex")
	}
	if len(ix.Index().LookupDefinition("bar")) != 1 {
		t.Error("bar should exist after reindex")
	}
}

func TestIndex_SkipsUnsupportedFiles(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "a.py", "def f():\n    pass\n")
	writeSrc(t, root, "b.txt", "not code\n")
	writeSrc(t, root, "c.js", "function f() {}\n")
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Errorf("expected only 1 file indexed (.py), got %d", n)
	}
}

func TestIndex_SkipsIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "real.py", "def f():\n    pass\n")
	writeSrc(t, root, "node_modules/dep.py", "def g():\n    pass\n")
	writeSrc(t, root, ".git/hook.py", "def h():\n    pass\n")
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Errorf("expected 1 file (ignored dirs skipped), got %d", n)
	}
	if len(ix.Index().LookupDefinition("g")) != 0 {
		t.Error("node_modules symbol leaked")
	}
}

func TestWatcher_DetectsNewFile(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "a.py", "def first():\n    pass\n")
	ix := New(root)
	_, _ = ix.Build()
	w := NewWatcher(ix, 50*time.Millisecond)
	w.Start()
	defer w.Stop()

	// Add a new file after the watcher starts.
	time.Sleep(80 * time.Millisecond) // let initial scan complete
	writeSrc(t, root, "b.py", "def second():\n    pass\n")

	// Wait for at least one poll cycle.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(ix.Index().LookupDefinition("second")) > 0 {
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("watcher did not index new file within 2s")
}

func TestWatcher_DetectsChangedFile(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "f.py", "def original():\n    pass\n")
	ix := New(root)
	_, _ = ix.Build()
	w := NewWatcher(ix, 50*time.Millisecond)
	w.Start()
	defer w.Stop()

	time.Sleep(80 * time.Millisecond)
	// Modify the file.
	writeSrc(t, root, "f.py", "def renamed():\n    pass\n")
	// Ensure mtime differs (some filesystems have low resolution).
	_ = os.Chtimes(filepath.Join(root, "f.py"), time.Now(), time.Now())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(ix.Index().LookupDefinition("renamed")) > 0 &&
			len(ix.Index().LookupDefinition("original")) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("watcher did not detect changed file within 2s")
}

func TestWatcher_Stop(t *testing.T) {
	root := t.TempDir()
	ix := New(root)
	w := NewWatcher(ix, 50*time.Millisecond)
	w.Start()
	w.Stop()
	// Should be able to start a new one after stop.
	w2 := NewWatcher(ix, 50*time.Millisecond)
	w2.Start()
	w2.Stop()
}

func TestContainsWord(t *testing.T) {
	cases := []struct {
		s, name string
		want    bool
	}{
		{"foo()", "foo", true},
		{"foobar()", "foo", false},
		{"barfoo()", "foo", false},
		{"x.foo()", "foo", true},
		{"foo_bar()", "foo", false},
		{"foo bar", "foo", true},
		{"", "foo", false},
		{"foo", "foo", true},
		{"foo.foo", "foo", true},
	}
	for _, c := range cases {
		got := containsWord(c.s, c.name)
		if got != c.want {
			t.Errorf("containsWord(%q, %q) = %v, want %v", c.s, c.name, got, c.want)
		}
	}
}
