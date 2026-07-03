package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abrandt/vla/internal/lsp"
)

func TestIndexer_KotlinParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "Main.kt", `package com.example

class User(val name: String) {
    fun greet(): String {
        return "Hello, $name"
    }
}

fun main() {
    val u = User("World")
    println(u.greet())
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	for _, name := range []string{"User", "greet", "main"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected definition for %q", name)
		}
		if defs[0].Language != "kotlin" {
			t.Errorf("%q language = %q, want kotlin", name, defs[0].Language)
		}
	}
	// Verify cross-file reference for greet.
	refs := ix.Index().LookupReferences("greet")
	if len(refs) == 0 {
		t.Error("expected references to greet (the call in main)")
	}
}

func TestIndexer_JavaParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "Service.java", `package com.example;

public class Service {
    private String name;

    public String getName() {
        return name;
    }

    public void process() {
        String result = getName();
    }
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	defs := ix.Index().LookupDefinition("Service")
	if len(defs) != 1 || defs[0].Kind != SymbolClass {
		t.Errorf("Service class: %+v", defs)
	}
	for _, name := range []string{"getName", "process"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected method %q", name)
		}
		if defs[0].Language != "java" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
	// getName is called in process — should have a reference.
	refs := ix.Index().LookupReferences("getName")
	if len(refs) == 0 {
		t.Error("expected references to getName")
	}
}

func TestIndexer_CSharpParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "Controller.cs", `using System;

namespace MyApp {
    public class Controller {
        private int count = 0;

        public string HandleRequest() {
            count++;
            return "ok";
        }
    }
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	defs := ix.Index().LookupDefinition("Controller")
	if len(defs) != 1 || defs[0].Kind != SymbolClass {
		t.Errorf("Controller class: %+v", defs)
	}
	for _, name := range []string{"HandleRequest"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected method %q", name)
		}
		if defs[0].Language != "csharp" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
}

func TestIndexer_PHPParser(t *testing.T) {
	root := t.TempDir()
	writeSrc(t, root, "User.php", `<?php

namespace App\Models;

class User {
    private $name;

    public function getName(): string {
        return $this->name;
    }
}

function helper(): void {
    $u = new User();
    $u->getName();
}
`)
	ix := New(root)
	n, _ := ix.Build()
	if n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
	defs := ix.Index().LookupDefinition("User")
	if len(defs) != 1 || defs[0].Kind != SymbolClass {
		t.Errorf("User class: %+v", defs)
	}
	for _, name := range []string{"getName", "helper"} {
		defs := ix.Index().LookupDefinition(name)
		if len(defs) == 0 {
			t.Errorf("expected function %q", name)
		}
		if defs[0].Language != "php" {
			t.Errorf("%q language = %q", name, defs[0].Language)
		}
	}
	// getName is called in helper — should have a reference.
	refs := ix.Index().LookupReferences("getName")
	if len(refs) == 0 {
		t.Error("expected references to getName")
	}
}

func TestInferLanguage_Kotlin(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte("plugins {}\n"), 0644)
	lang := lsp.InferLanguage(dir)
	if lang != lsp.LangKotlin {
		t.Errorf("expected kotlin, got %q", lang)
	}
}

func TestInferLanguage_Java(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project></project>\n"), 0644)
	lang := lsp.InferLanguage(dir)
	if lang != lsp.LangJava {
		t.Errorf("expected java, got %q", lang)
	}
}

func TestInferLanguage_CSharp(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "App.csproj"), []byte("<Project></Project>\n"), 0644)
	lang := lsp.InferLanguage(dir)
	if lang != lsp.LangCSharp {
		t.Errorf("expected csharp, got %q", lang)
	}
}

func TestInferLanguage_PHP(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}\n"), 0644)
	lang := lsp.InferLanguage(dir)
	if lang != lsp.LangPHP {
		t.Errorf("expected php, got %q", lang)
	}
}

func TestDefaultSpecs_HasAllLanguages(t *testing.T) {
	specs := lsp.DefaultSpecs()
	for _, lang := range []lsp.Language{lsp.LangPython, lsp.LangGo, lsp.LangKotlin, lsp.LangJava, lsp.LangCSharp, lsp.LangPHP} {
		spec, ok := specs[lang]
		if !ok {
			t.Errorf("missing spec for %q", lang)
		}
		if spec.Command == "" {
			t.Errorf("empty command for %q", lang)
		}
	}
}
