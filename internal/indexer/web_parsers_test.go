package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abrandt/vla/internal/lsp"
)

func TestIndexer_JavaScriptParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "app.js", `export function greet(name) {
    return "Hello, " + name;
}

class User {
    constructor(name) {
        this.name = name;
    }
}

const handler = (event) => {
    greet("world");
};

const API_URL = "https://api.example.com";
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	for _, name := range []string{"greet", "User", "handler", "API_URL"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected definition for %q", name)
		}
		if defs[0].Language != "javascript" {
			t.Errorf("%q language = %q, want javascript", name, defs[0].Language)
		}
	}
	// greet is called in handler → cross-file ref.
	refs := ix.Index().LookupReferences("greet")
	if len(refs) == 0 {
		t.Error("expected references to greet")
	}
}

func TestIndexer_TypeScriptParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "service.ts", `interface Config {
    host: string;
}

type Status = "active" | "inactive";

export class Service {
    private config: Config;

    constructor(cfg: Config) {
        this.config = cfg;
    }
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	for _, name := range []string{"Config", "Status", "Service"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected definition for %q", name)
		}
		if defs[0].Language != "javascript" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
}

func TestIndexer_CSSParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "styles.css", `.button {
    color: red;
}

#header {
    background: blue;
}

.content-area {
    padding: 10px;
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	for _, name := range []string{"button", "header", "content-area"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected CSS selector %q", name)
			continue
		}
		if defs[0].Language != "css" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
}

func TestIndexer_SCSSParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "theme.scss", `$primary: #333;

@mixin rounded {
    border-radius: 8px;
}

.card {
    @include rounded;
    color: $primary;
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	// Mixixin should be indexed.
	defs := ix.Index().LookupDefinition("rounded")
	if len(defs) == 0 {
		t.Error("expected mixin 'rounded' definition")
	}
	// .card selector.
	defs = ix.Index().LookupDefinition("card")
	if len(defs) == 0 {
		t.Error("expected .card selector")
	}
	// @include should create a reference to the mixin.
	refs := ix.Index().LookupReferences("rounded")
	if len(refs) == 0 {
		t.Error("expected reference to 'rounded' from @include")
	}
}

func TestIndexer_HTMLParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "index.html", `<!DOCTYPE html>
<html>
<body>
    <div id="app" class="container main">
        <h1 class="title">Hello</h1>
        <button id="submit-btn">Click</button>
    </div>
</body>
</html>
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	// IDs are prefixed with "id:".
	for _, name := range []string{"id:app", "id:submit-btn"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected HTML id %q", name)
		}
		if defs[0].Language != "html" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
	// Classes are prefixed with "class:".
	for _, name := range []string{"class:container", "class:main", "class:title"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected HTML class %q", name)
		}
	}
}

func TestInferLanguage_JS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644)
	lang := lsp.InferLanguage(dir)
	if lang != lsp.LangJS {
		t.Errorf("expected javascript, got %q", lang)
	}
}

func TestDefaultSpecs_HasJS(t *testing.T) {
	specs := lsp.DefaultSpecs()
	spec, ok := specs[lsp.LangJS]
	if !ok {
		t.Fatal("missing JS LSP spec")
	}
	if spec.Command != "typescript-language-server" {
		t.Errorf("JS command = %q", spec.Command)
	}
}
