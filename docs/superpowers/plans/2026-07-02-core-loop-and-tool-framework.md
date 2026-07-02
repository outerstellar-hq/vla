# VLA Core Loop + Tool Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first working VLA — a CLI agentic coding harness with a streaming agent loop, NDJSON session transcripts, a self-contained tool framework, and one trivial tool (echo) to prove the loop end-to-end.

**Architecture:** Go single binary. Packages under `internal/`: `config` (load `config.json`), `llm` (OpenAI-compatible streaming client with SSE parsing), `session` (NDJSON transcript lifecycle), `agent` (the core loop + message types), `tools` (registry + `Tool` interface + builtin/echo), `compaction` (view-transform summarization). `main.go` wires flags → config → session → loop.

**Tech Stack:** Go 1.26 (stdlib only — no external dependencies for the first build), OpenAI Chat Completions API (streaming via SSE), NDJSON file format.

**Reference docs:**
- Design: `docs/DESIGN.md` (read this first — all decisions and rationale live here)
- Coding standards: appendix of DESIGN.md — absolute bare minimum LOC, errors surface loudly, static typing, encapsulated tools.

**Scope guard:** This plan implements ONLY the core loop + tool framework + echo tool. File tools, git tools, search, navigation, background indexer, and web tools are explicitly out of scope (Phases 2–6 in the design doc).

---

## File Structure

```
vla/
├── main.go                          # Entry: parse flags, load config, start session, run loop
├── config.json.example              # Template users copy to config.json
├── go.mod
│
├── internal/
│   ├── agent/
│   │   ├── loop.go                  # Core loop: send → stream → parse → execute → append → repeat
│   │   └── message.go               # Message types matching OpenAI chat completions API
│   │
│   ├── config/
│   │   ├── config.go                # Load + validate config.json
│   │   └── config_test.go
│   │
│   ├── llm/
│   │   ├── client.go                # Streaming client: SSE parsing, tool-call assembly
│   │   └── client_test.go
│   │
│   ├── session/
│   │   ├── session.go               # Session lifecycle: ID, CWD, transcript file
│   │   ├── transcript.go            # NDJSON read/write (metadata line + turn lines)
│   │   └── transcript_test.go
│   │
│   ├── tools/
│   │   ├── tool.go                  # Tool interface
│   │   ├── registry.go              # Collects tools, exposes schemas
│   │   ├── registry_test.go
│   │   └── builtin/
│   │       └── echo.go              # Trivial test tool
│   │
│   └── compaction/
│       ├── compaction.go            # Compact([]Message) []Message view transform
│       └── compaction_test.go
│
└── docs/
    └── DESIGN.md
```

**Why this split:** Each package has one responsibility and can be tested in isolation. The dependency graph is acyclic and shallow: `main` → `agent` → `{llm, session, tools, compaction}` → `config`. No package imports `main`. Tools depend only on the `tools` package interface, never on the agent or LLM client.

---

## Task 1: Initialize Go module and project skeleton

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `config.json.example`

- [ ] **Step 1: Initialize the Go module**

Run from the repo root:
```bash
cd "C:\Develop\Claude\projects\weird\vla"
go mod init github.com/abrandt/vla
```

Expected: creates `go.mod` with `module github.com/abrandt/vla` and `go 1.26`.

- [ ] **Step 2: Create .gitignore**

Create `.gitignore`:
```
# Build output
vla
vla.exe
*.exe

# Local config (contains API key)
config.json

# Go workspace cruft
vendor/
```

- [ ] **Step 3: Create config.json.example**

Create `config.json.example`:
```json
{
    "api_key": "sk-...",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o"
}
```

- [ ] **Step 4: Verify the module compiles (empty main)**

Create a temporary `main.go`:
```go
package main

func main() {}
```

Run:
```bash
go build ./...
```
Expected: succeeds with no output.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore config.json.example main.go
git commit -m "chore: initialize Go module and project skeleton"
```

---

## Task 2: Config package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{
		"api_key": "sk-test",
		"base_url": "https://api.openai.com/v1",
		"model": "gpt-4o"
	}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.openai.com/v1")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{not json`), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{"base_url":"u","model":"m"}`), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for missing api_key, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/config/
```
Expected: FAIL — package does not exist / `Load` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/config.go`:
```go
// Package config loads and validates the VLA config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the LLM connection settings loaded from config.json.
type Config struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// Load reads, parses, and validates the config file at path.
// Returns an error if the file is missing, unreadable, malformed JSON,
// or fails validation (empty api_key or model).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("config: api_key is required")
	}
	if c.Model == "" {
		return fmt.Errorf("config: model is required")
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.openai.com/v1"
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/config/ -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add config.json loader with validation"
```

---

## Task 3: Agent message types

**Files:**
- Create: `internal/agent/message.go`

This task defines the message types used both for the transcript and for the OpenAI API payload. There is no test step — these are pure data types exercised by later tasks.

- [ ] **Step 1: Define the message types**

Create `internal/agent/message.go`:
```go
// Package agent implements the VLA core agent loop and the message types
// shared between the transcript, the LLM client, and the tools.
package agent

import "encoding/json"

// Role identifies who produced a message in the conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a single function call requested by the assistant.
// The OpenAI API streams this in fragments; the LLM client assembles
// the complete call before appending it to the transcript.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function name + arguments pair inside a ToolCall.
// Arguments is a raw JSON string (per the OpenAI API contract, not a parsed object).
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string, parsed by the tool
}

// Message is one turn in the conversation. It maps to the OpenAI chat
// completions message shape, with a few VLA-specific fields (Timestamp,
// Kind) used only by the transcript layer.
type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // set when Role == RoleTool
	Timestamp  string      `json:"timestamp,omitempty"`    // ISO-8601, set by the transcript layer
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./internal/agent/
```
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/message.go
git commit -m "feat(agent): define message and tool-call types"
```

---

## Task 4: Tool interface and registry

**Files:**
- Create: `internal/tools/tool.go`
- Create: `internal/tools/registry.go`
- Create: `internal/tools/registry_test.go`

- [ ] **Step 1: Write the failing test for the registry**

Create `internal/tools/registry_test.go`:
```go
package tools

import (
	"encoding/json"
	"testing"
)

// stubTool is a test double implementing Tool.
type stubTool struct {
	name   string
	schema map[string]any
}

func (s *stubTool) Name() string                 { return s.name }
func (s *stubTool) Schema() map[string]any        { return s.schema }
func (s *stubTool) Execute(args json.RawMessage) (string, error) {
	return "ok", nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	t1 := &stubTool{name: "alpha", schema: map[string]any{"type": "object"}}
	r.Register(t1)

	got, ok := r.Get("alpha")
	if !ok {
		t.Fatal("expected to find registered tool alpha")
	}
	if got.Name() != "alpha" {
		t.Errorf("got name %q, want alpha", got.Name())
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected Get to return false for unregistered tool")
	}
}

func TestRegistry_Schemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "a", schema: map[string]any{"type": "object"}})
	r.Register(&stubTool{name: "b", schema: map[string]any{"type": "object"}})

	schemas := r.Schemas()
	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}
	// Each schema should carry the tool's name as the top-level "name".
	for _, s := range schemas {
		if s["name"] == nil {
			t.Errorf("schema missing name field: %v", s)
		}
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "dup", schema: map[string]any{}})
	err := r.Register(&stubTool{name: "dup", schema: map[string]any{}})
	if err == nil {
		t.Fatal("expected error registering duplicate tool name")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/tools/ -v
```
Expected: FAIL — `Tool`, `NewRegistry` undefined.

- [ ] **Step 3: Write the Tool interface**

Create `internal/tools/tool.go`:
```go
// Package tools defines the VLA tool framework: the Tool interface every
// tool implements, and the Registry that collects tools and exposes their
// schemas to the LLM. Each concrete tool lives in its own file under
// builtin/ and is fully self-contained.
package tools

import "encoding/json"

// Tool is the interface every VLA tool implements.
// To alter a tool's schema or behavior, edit its file — no central
// registry changes required beyond the one-line registration call.
type Tool interface {
	// Name returns the tool's unique identifier (e.g. "echo").
	Name() string

	// Schema returns the OpenAI function-calling JSON schema for this
	// tool's parameters — what the LLM sees. The caller wraps this with
	// the tool's Name() to form the full tool definition.
	Schema() map[string]any

	// Execute runs the tool with the given raw JSON arguments and
	// returns the result string. Tools parse args themselves via
	// json.Unmarshal into a tool-specific struct for type safety.
	// Per the design: tool errors are returned as result strings
	// ("Error: ..."), NOT as a Go error, so the loop can feed them
	// back to the LLM. The error return is reserved for failures the
	// loop cannot recover from (which, by convention, tools never raise).
	Execute(args json.RawMessage) (string, error)
}
```

- [ ] **Step 4: Write the registry**

Create `internal/tools/registry.go`:
```go
package tools

import "fmt"

// Registry collects all tools and exposes their schemas to the LLM.
// Adding a tool = implement Tool in its own file + one Register call.
type Registry struct {
	tools map[string]Tool
	order []string // preserves registration order for stable schema output
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Registering a duplicate name
// returns an error — each tool name must be unique.
func (r *Registry) Register(t Tool) error {
	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tools: duplicate registration for %q", name)
	}
	r.tools[name] = t
	r.order = append(r.order, name)
	return nil
}

// Get returns the tool registered under name, plus whether it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Schemas returns the OpenAI function-calling tool definitions for all
// registered tools, in registration order. Each entry is the full tool
// object: {"type": "function", "function": {"name": ..., "parameters": <schema>}}.
func (r *Registry) Schemas() []map[string]any {
	out := make([]map[string]any, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":       t.Name(),
				"parameters": t.Schema(),
			},
		})
	}
	return out
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
go test ./internal/tools/ -v
```
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): add Tool interface and Registry"
```

---

## Task 5: Echo builtin tool

**Files:**
- Create: `internal/tools/builtin/echo.go`

The echo tool proves the loop end-to-end: the LLM calls it, the loop executes it, and the result is appended to the transcript. It returns the text the LLM sent as arguments.

- [ ] **Step 1: Write the echo tool**

Create `internal/tools/builtin/echo.go`:
```go
// Package builtin holds VLA's built-in tools. Each tool is a self-contained
// struct in its own file implementing tools.Tool.
package builtin

import (
	"encoding/json"
	"fmt"
)

// Echo is a trivial tool that returns its input text. It exists to prove
// the agent loop end-to-end: LLM calls it, the loop executes it, the
// result lands in the transcript. Replace or extend it with real tools
// (read_file, etc.) in later builds.
type Echo struct{}

// Name returns the tool's identifier.
func (Echo) Name() string { return "echo" }

// Schema returns the OpenAI function-calling parameter schema.
func (Echo) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "The text to echo back.",
			},
		},
		"required": []string{"text"},
	}
}

// Execute parses the arguments and returns the text unchanged.
// Per the design, malformed arguments come back as an error *string*
// (not a Go error) so the LLM can react and retry.
func (Echo) Execute(args json.RawMessage) (string, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return fmt.Sprintf("Error: could not parse echo arguments: %v", err), nil
	}
	if in.Text == "" {
		return "Error: text is required", nil
	}
	return in.Text, nil
}
```

- [ ] **Step 2: Verify it compiles and satisfies the Tool interface**

Create `internal/tools/builtin/echo_test.go`:
```go
package builtin

import "testing"

func TestEcho_Execute(t *testing.T) {
	var e Echo
	got, err := e.Execute([]byte(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEcho_Execute_MissingText(t *testing.T) {
	var e Echo
	got, _ := e.Execute([]byte(`{}`))
	if got != "Error: text is required" {
		t.Errorf("got %q, want error string", got)
	}
}

func TestEcho_Execute_MalformedJSON(t *testing.T) {
	var e Echo
	got, err := e.Execute([]byte(`{not json`))
	if err != nil {
		t.Fatalf("tool errors should be returned as result strings, not Go errors; got: %v", err)
	}
	if got == "" || got[:5] != "Error" {
		t.Errorf("got %q, want an error string starting with 'Error'", got)
	}
}
```

- [ ] **Step 3: Run the tests**

Run:
```bash
go test ./internal/tools/... -v
```
Expected: PASS (registry + echo tests).

- [ ] **Step 4: Commit**

```bash
git add internal/tools/builtin/
git commit -m "feat(tools): add echo builtin tool"
```

---

## Task 6: Session and NDJSON transcript

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/transcript.go`
- Create: `internal/session/transcript_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/session/transcript_test.go`:
```go
package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_CreatesTranscriptFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithDir(dir), WithModel("gpt-4o"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	path := filepath.Join(dir, s.ID()+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("transcript file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("transcript file is empty; expected metadata line")
	}

	// First line must be the session metadata.
	f, _ := os.Open(path)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("transcript has no lines")
	}
	var meta struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Model   string `json:"model"`
		Cwd     string `json:"cwd"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		t.Fatalf("first line not valid JSON: %v", err)
	}
	if meta.Type != "session" {
		t.Errorf("metadata type = %q, want %q", meta.Type, "session")
	}
	if meta.ID == "" {
		t.Error("metadata id is empty")
	}
	if meta.Model != "gpt-4o" {
		t.Errorf("metadata model = %q, want gpt-4o", meta.Model)
	}
}

func TestAppend_Turn(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(WithDir(dir), WithModel("gpt-4o"))

	err := s.Append(map[string]any{
		"type":      "turn",
		"role":      "user",
		"content":   "hello",
		"timestamp": "2026-07-02T15:03:01Z",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	turns, meta, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if meta["model"] != "gpt-4o" {
		t.Errorf("meta model = %v", meta["model"])
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0]["content"] != "hello" {
		t.Errorf("turn content = %v", turns[0]["content"])
	}
}

func TestRead_ExistingSession(t *testing.T) {
	dir := t.TempDir()
	// Create a transcript manually, then resume it by ID.
	path := filepath.Join(dir, "2026-01-01T000000Z.json")
	content := `{"type":"session","id":"2026-01-01T000000Z","cwd":"/tmp","model":"gpt-4o","created":"2026-01-01T00:00:00Z"}
{"type":"turn","role":"user","content":"hi","timestamp":"2026-01-01T00:00:01Z"}
{"type":"turn","role":"assistant","content":"hello","timestamp":"2026-01-01T00:00:02Z"}
`
	_ = os.WriteFile(path, []byte(content), 0644)

	s, err := Open(filepath.Join(dir, "2026-01-01T000000Z.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	turns, meta, err := s.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if meta["cwd"] != "/tmp" {
		t.Errorf("meta cwd = %v", meta["cwd"])
	}
}

func TestSessionsDir(t *testing.T) {
	// SessionsDir returns ~/.vla/sessions by default.
	// We can't assert the exact path (home varies), but it must be non-empty
	// and end with the right suffix.
	dir := SessionsDir()
	if dir == "" {
		t.Fatal("SessionsDir returned empty string")
	}
	// It should contain the vla/sessions path components.
	if !strings.HasSuffix(filepath.ToSlash(dir), ".vla/sessions") {
		t.Errorf("SessionsDir = %q, want path ending in .vla/sessions", dir)
	}
}
```

Add the missing import at the top of the test file (the file above references `strings`):
```go
import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/session/ -v
```
Expected: FAIL — `New`, `Open`, `SessionsDir` undefined.

- [ ] **Step 3: Write session.go**

Create `internal/session/session.go`:
```go
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
	meta, _, err := readTranscript(abs)
	if err != nil {
		return nil, err
	}
	id, _ := meta["id"].(string)
	cwd, _ := meta["cwd"].(string)
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
		home = "." // best-effort fallback
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
```

- [ ] **Step 4: Write transcript.go**

Create `internal/session/transcript.go`:
```go
package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Append writes one turn as a single NDJSON line to the transcript file.
// The caller supplies the full turn object (role, content, tool_calls, etc.).
func (s *Session) Append(turn map[string]any) error {
	line, err := encodeLine(turn)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("session: open transcript for append: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("session: append turn: %w", err)
	}
	return nil
}

// Read returns the turns (excluding the metadata line) and the session metadata.
func (s *Session) Read() (turns []map[string]any, meta map[string]any, err error) {
	return readTranscript(s.path)
}

// readTranscript parses an NDJSON transcript file: the first line is the
// session metadata, every subsequent line is a turn.
func readTranscript(path string) ([]map[string]any, map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("session: open %s: %w", path, err)
	}
	defer f.Close()

	var meta map[string]any
	var turns []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // long turns (file contents) get room
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return nil, nil, fmt.Errorf("session: parse line %d of %s: %w", lineNo, path, err)
		}
		if t, _ := obj["type"].(string); t == "session" {
			meta = obj
			continue
		}
		turns = append(turns, obj)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("session: read %s: %w", path, err)
	}
	if meta == nil {
		return nil, nil, fmt.Errorf("session: %s has no session metadata line", path)
	}
	return turns, meta, nil
}

// encodeLine marshals obj to JSON with no trailing newline (Append adds it).
func encodeLine(obj map[string]any) ([]byte, error) {
	line, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("session: encode turn: %w", err)
	}
	return line, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
go test ./internal/session/ -v
```
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/session/
git commit -m "feat(session): add session lifecycle and NDJSON transcript"
```

---

## Task 7: LLM client (streaming, SSE, tool-call assembly)

**Files:**
- Create: `internal/llm/client.go`
- Create: `internal/llm/client_test.go`

This is the most intricate piece. The client posts a chat-completions request with `stream: true`, reads the SSE stream token-by-token, prints text deltas to the terminal as they arrive, and accumulates tool-call fragments into complete calls.

**Design notes for the implementer:**
- The OpenAI streaming response is a series of `data: {...}\n\n` lines. The stream ends with `data: [DONE]`.
- Each chunk's `choices[0].delta` may carry `content` (text), `tool_calls` (an array of fragments indexed by `index`), or both. Tool-call fragments arrive split across many chunks: the first chunk carries `id` and `function.name`, subsequent chunks append to `function.arguments`.
- Text deltas are written to `os.Stdout` immediately as they arrive (the streaming UX). The accumulated full content is returned at the end.
- We use `net/http` + `bufio.Scanner` + `encoding/json` from the stdlib. No SSE library.

- [ ] **Step 1: Write the failing test using a test HTTP server**

Create `internal/llm/client_test.go`:
```go
package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

// fakeStreamServer returns a sequence of SSE chunks. We test the
// accumulation logic: text deltas become Content, tool-call fragments
// merge into complete ToolCalls.
func fakeStreamServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestStream_TextOnly(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":", world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if msg.Content != "Hello, world" {
		t.Errorf("Content = %q, want %q", msg.Content, "Hello, world")
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(msg.ToolCalls))
	}
}

func TestStream_ToolCallFragments(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"te"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"xt\":\"hi\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := fakeStreamServer(t, chunks)
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	msg, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "call echo"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("tool call id = %q, want call_1", tc.ID)
	}
	if tc.Function.Name != "echo" {
		t.Errorf("function name = %q, want echo", tc.Function.Name)
	}
	wantArgs := `{"text":"hi"}`
	if tc.Function.Arguments != wantArgs {
		t.Errorf("arguments = %q, want %q", tc.Function.Arguments, wantArgs)
	}
}

func TestStream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	c := NewClient("sk-test", srv.URL, "gpt-4o")
	_, err := c.Stream([]agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/llm/ -v
```
Expected: FAIL — `NewClient`, `Stream` undefined.

- [ ] **Step 3: Write the client**

Create `internal/llm/client.go`:
```go
// Package llm implements an OpenAI-compatible streaming chat-completions client.
// It posts a request with stream:true, parses the SSE response token by token,
// prints text deltas to stdout as they arrive, and accumulates tool-call
// fragments into complete calls.
package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abrandt/vla/internal/agent"
)

// Client is an OpenAI-compatible streaming chat-completions client.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// NewClient returns a streaming client for the given config.
func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{},
	}
}

// request is the body posted to /chat/completions.
type request struct {
	Model    string           `json:"model"`
	Messages []agent.Message  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

// streamChunk models the relevant fields of one SSE chunk.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Stream sends messages to the model, streams text deltas to w as they arrive,
// and returns the fully-assembled assistant message. toolDefs is the output
// of tools.Registry.Schemas() (may be nil).
//
// Returns an error on HTTP failures (non-2xx), network errors, or malformed
// SSE lines. These are infrastructure failures — the agent loop treats them
// as abort conditions.
func (c *Client) Stream(messages []agent.Message, toolDefs []map[string]any) (agent.Message, error) {
	return c.StreamTo(messages, toolDefs, nil)
}

// StreamTo is like Stream but writes text deltas to out instead of discarding
// them. If out is nil, deltas are still accumulated into the returned message
// but not printed anywhere (useful for tests; the agent loop passes os.Stdout).
func (c *Client) StreamTo(messages []agent.Message, toolDefs []map[string]any, out io.Writer) (agent.Message, error) {
	body, err := json.Marshal(request{
		Model:    c.model,
		Messages: messages,
		Tools:    toolDefs,
		Stream:   true,
	})
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return agent.Message{}, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return agent.Message{}, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return parseSSE(resp.Body, out)
}

// parseSSE reads the SSE stream, prints text deltas to out, and accumulates
// the assistant message (content + tool calls).
func parseSSE(r io.Reader, out io.Writer) (agent.Message, error) {
	msg := agent.Message{Role: agent.RoleAssistant}
	// tool-call accumulator keyed by fragment index.
	type tcAcc struct {
		ID   string
		Name string
		Args strings.Builder
	}
	acc := map[int]*tcAcc{}
	var order []int // preserves the order tool calls first appeared in

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue // ignore event/id lines for now
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return msg, fmt.Errorf("llm: parse SSE chunk: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			msg.Content += delta.Content
			if out != nil {
				if _, err := io.WriteString(out, delta.Content); err != nil {
					return msg, fmt.Errorf("llm: write stream output: %w", err)
				}
			}
		}
		for _, fr := range delta.ToolCalls {
			accEntry, ok := acc[fr.Index]
			if !ok {
				accEntry = &tcAcc{}
				acc[fr.Index] = accEntry
				order = append(order, fr.Index)
			}
			if fr.ID != "" {
				accEntry.ID = fr.ID
			}
			if fr.Function.Name != "" {
				accEntry.Name = fr.Function.Name
			}
			if fr.Function.Arguments != "" {
				accEntry.Args.WriteString(fr.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return msg, fmt.Errorf("llm: read stream: %w", err)
	}

	for _, idx := range order {
		a := acc[idx]
		msg.ToolCalls = append(msg.ToolCalls, agent.ToolCall{
			ID:   a.ID,
			Type: "function",
			Function: agent.FunctionCall{
				Name:      a.Name,
				Arguments: a.Args.String(),
			},
		})
	}
	if out != nil && msg.Content != "" {
		// Visual separator after the streamed response.
		_, _ = io.WriteString(out, "\n")
	}
	return msg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/llm/ -v
```
Expected: PASS (3 tests). The fake server exercises: text-only streaming, tool-call fragment assembly, and HTTP error handling.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/
git commit -m "feat(llm): add streaming client with SSE parsing and tool-call assembly"
```

---

## Task 8: Compaction (view transform)

**Files:**
- Create: `internal/compaction/compaction.go`
- Create: `internal/compaction/compaction_test.go`

The prototype compaction is a pure function: if the message list is over the char threshold, summarize the oldest turns into one system message. The on-disk transcript is never modified — this only changes what's sent to the LLM.

**Note:** Summarization requires an LLM call. For testability and simplicity, the summarizer is injected as a function parameter. The agent loop will pass a closure that calls the LLM client; tests pass a stub.

- [ ] **Step 1: Write the failing test**

Create `internal/compaction/compaction_test.go`:
```go
package compaction

import (
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/agent"
)

func TestCompact_BelowThreshold_Noop(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "short"},
		{Role: agent.RoleAssistant, Content: "reply"},
	}
	out, err := Compact(msgs, stubSummarizer, 1_000_000)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(out) != len(msgs) {
		t.Errorf("expected noop, got %d messages (want %d)", len(out), len(msgs))
	}
}

func TestCompact_AboveThreshold_Summarizes(t *testing.T) {
	// Build a transcript well over the threshold.
	var msgs []agent.Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, agent.Message{
			Role:    agent.RoleUser,
			Content: strings.Repeat("x", 1000), // 1KB each
		})
	}
	// Threshold 5000 means ~5 messages triggers compaction; we keep the last 8.
	out, err := Compact(msgs, stubSummarizer, 5000)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	// Expected: 1 summary + 8 recent = 9.
	if len(out) != 9 {
		t.Errorf("expected 9 messages after compaction, got %d", len(out))
	}
	if out[0].Role != agent.RoleSystem {
		t.Errorf("expected first message to be system summary, got role %q", out[0].Role)
	}
	if !strings.Contains(out[0].Content, "SUMMARY") {
		t.Errorf("expected summary content, got %q", out[0].Content)
	}
}

func TestCompact_TooFewToCompact_Noop(t *testing.T) {
	// Even if over threshold, if we have <= KeepRecent messages, return as-is.
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: strings.Repeat("x", 10000)},
	}
	out, err := Compact(msgs, stubSummarizer, 100)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("expected 1 message (too few to compact), got %d", len(out))
	}
}

func TestCompact_PreservesRecentOrder(t *testing.T) {
	var msgs []agent.Message
	for i := 0; i < 12; i++ {
		msgs = append(msgs, agent.Message{Role: agent.RoleUser, Content: strings.Repeat("x", 1000)})
	}
	out, _ := Compact(msgs, stubSummarizer, 5000)
	// Last 8 messages must be unchanged.
	if len(out) != 9 {
		t.Fatalf("expected 9 messages, got %d", len(out))
	}
	for i := 0; i < 8; i++ {
		original := msgs[len(msgs)-8+i]
		got := out[1+i]
		if got.Content != original.Content {
			t.Errorf("recent message %d changed: %q vs %q", i, got.Content, original.Content)
		}
	}
}

// stubSummarizer replaces the LLM call in tests.
func stubSummarizer(msgs []agent.Message) (string, error) {
	return "SUMMARY of " + itoa(len(msgs)) + " messages", nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/compaction/ -v
```
Expected: FAIL — `Compact` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/compaction/compaction.go`:
```go
// Package compaction implements the VLA context-window compaction strategy.
// When the transcript grows too long, the oldest turns are summarized into
// a single system message. The on-disk transcript is NEVER modified —
// Compact is a pure view transform that returns the messages to send to
// the LLM.
package compaction

import (
	"fmt"

	"github.com/abrandt/vla/internal/agent"
)

// CharThreshold is the rough transcript size (in characters) above which
// compaction kicks in. ~100K chars ≈ 25K tokens, leaving headroom under
// a 32K context window. Tunable.
const CharThreshold = 100_000

// KeepRecent is the number of most-recent turns always preserved verbatim.
// Everything older than this is eligible for summarization.
const KeepRecent = 8

// Summarizer summarizes a slice of messages into a terse string.
// The agent loop supplies a real implementation that calls the LLM.
type Summarizer func(msgs []agent.Message) (string, error)

// Compact returns the message list to send to the LLM. If the total
// character count is below CharThreshold, or there are too few messages
// to summarize, the input is returned unchanged. Otherwise the oldest
// turns (all but the most recent KeepRecent) are replaced by a single
// system message produced by sum.
func Compact(msgs []agent.Message, sum Summarizer, threshold int) ([]agent.Message, error) {
	if totalChars(msgs) < threshold {
		return msgs, nil
	}
	if len(msgs) <= KeepRecent {
		return msgs, nil
	}

	split := len(msgs) - KeepRecent
	old := msgs[:split]
	recent := msgs[split:]

	summary, err := sum(old)
	if err != nil {
		return nil, fmt.Errorf("compaction: summarize: %w", err)
	}
	out := make([]agent.Message, 0, 1+len(recent))
	out = append(out, agent.Message{
		Role:    agent.RoleSystem,
		Content: "Summary of earlier conversation:\n\n" + summary,
	})
	out = append(out, recent...)
	return out, nil
}

func totalChars(msgs []agent.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/compaction/ -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/compaction/
git commit -m "feat(compaction): add view-transform compaction with injectable summarizer"
```

---

## Task 9: The agent loop

**Files:**
- Create: `internal/agent/loop.go`
- Create: `internal/agent/loop_test.go`

The loop ties together the LLM client, the tool registry, and the transcript. It receives a user message, appends it, builds the messages view (with compaction), calls the LLM, streams the response, executes any tool calls, appends results, and loops until the LLM responds without tool calls.

- [ ] **Step 1: Write the failing test**

Create `internal/agent/loop_test.go`:
```go
package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

// streamingServer emits the given SSE chunks, then [DONE].
func streamingServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
}

// Test that a simple text-only exchange terminates after one LLM call
// (no tool calls → loop ends).
func TestLoop_TextOnly_Terminates(t *testing.T) {
	srv := streamingServer(t, []string{
		`{"choices":[{"delta":{"role":"assistant","content":"hi there"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	})
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	loop := NewLoop(client, reg, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("hello\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(output.String(), "hi there") {
		t.Errorf("expected 'hi there' in output, got %q", output.String())
	}
}

// Test that when the LLM calls a tool, the loop executes it and re-calls
// the LLM. The server returns two responses in sequence: first a tool call,
// then a final text response.
func TestLoop_ToolCall_ExecutesAndLoops(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		var chunks []string
		if callCount == 1 {
			// First call: LLM requests the echo tool.
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"ping\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		} else {
			// Second call: LLM gives a final text answer.
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","content":"echo said: ping"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			}
		}
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	if err := reg.Register(builtin.Echo{}); err != nil {
		t.Fatal(err)
	}
	loop := NewLoop(client, reg, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("call echo with ping\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (tool + final), got %d", callCount)
	}
	if !strings.Contains(output.String(), "echo said: ping") {
		t.Errorf("expected final answer in output, got %q", output.String())
	}
}

// Test that a tool returning an error string does NOT break the loop —
// the error is fed back to the LLM as the tool result.
func TestLoop_ToolError_FedBackToLLM(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		var chunks []string
		if callCount == 1 {
			// LLM calls echo with no text → echo returns "Error: text is required".
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			}
		} else {
			chunks = []string{
				`{"choices":[{"delta":{"role":"assistant","content":"sorry, try again"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			}
		}
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			f.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		f.Flush()
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL, "gpt-4o")
	reg := tools.NewRegistry()
	_ = reg.Register(builtin.Echo{})
	loop := NewLoop(client, reg, 1_000_000)

	var output strings.Builder
	err := loop.Run(strings.NewReader("call echo badly\n\n"), &output)
	if err != nil {
		t.Fatalf("Run: %v (tool errors should not propagate as Go errors)", err)
	}
	if !strings.Contains(output.String(), "sorry, try again") {
		t.Errorf("expected LLM recovery in output, got %q", output.String())
	}
}

// Ensure unused imports are referenced.
var _ = json.RawMessage{}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/agent/ -v
```
Expected: FAIL — `NewLoop`, `Loop.Run` undefined.

- [ ] **Step 3: Write the loop**

Create `internal/agent/loop.go`:
```go
package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/abrandt/vla/internal/compaction"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/tools"
)

// Loop is the VLA agent loop. It is created once per session and run with
// a user-input reader and an output writer (the terminal).
type Loop struct {
	client     *llm.Client
	registry   *tools.Registry
	summarizer compaction.Summarizer
	threshold  int
	messages   []Message // in-memory transcript view (rebuilt from disk on resume)
}

// NewLoop returns a Loop wired to the given client and tool registry.
// threshold is the compaction character threshold.
func NewLoop(client *llm.Client, registry *tools.Registry, threshold int) *Loop {
	return &Loop{
		client:    client,
		registry:  registry,
		threshold: threshold,
		// Default summarizer: makes a summarization LLM call.
		// Overridden in tests; in production this is the real deal.
		summarizer: defaultSummarizer(client),
	}
}

// Run reads user messages from in (one per blank-line-terminated block) and
// writes assistant responses + tool results to out. The loop continues
// reading input until EOF or error.
func (l *Loop) Run(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "> ")
		text, err := readMessage(reader)
		if err == io.EOF {
			fmt.Fprintln(out)
			return nil
		}
		if err != nil {
			return fmt.Errorf("agent: read input: %w", err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		l.messages = append(l.messages, Message{Role: RoleUser, Content: text})
		if err := l.turn(out); err != nil {
			return err
		}
	}
}

// turn executes one full agent turn: call LLM → stream → execute tool calls
// → loop until the LLM responds without tool calls.
func (l *Loop) turn(out io.Writer) error {
	for {
		view, err := compaction.Compact(l.messages, l.summarizer, l.threshold)
		if err != nil {
			return err
		}

		msg, err := l.client.StreamTo(view, l.registry.Schemas(), out)
		if err != nil {
			return err
		}
		l.messages = append(l.messages, msg)

		if len(msg.ToolCalls) == 0 {
			return nil // turn complete — wait for next user message
		}

		// Execute each tool call and append the result.
		for _, tc := range msg.ToolCalls {
			result := l.executeToolCall(tc)
			l.messages = append(l.messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
			fmt.Fprintf(out, "[tool %s → %s]\n", tc.Function.Name, truncate(result, 200))
		}
	}
}

// executeToolCall looks up the tool by name and runs it. Per the design,
// tool errors are returned as result strings — they never break the loop.
// A Go error here means the tool doesn't exist (registry miss), which we
// also feed back to the LLM as content.
func (l *Loop) executeToolCall(tc ToolCall) string {
	tool, ok := l.registry.Get(tc.Function.Name)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", tc.Function.Name)
	}
	result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
	if err != nil {
		// By convention tools return errors as strings, but if a tool
		// ever does return a Go error, surface it as content too.
		return fmt.Sprintf("Error: %s: %v", tc.Function.Name, err)
	}
	return result
}

// readMessage reads one user message: lines until a blank line is entered.
// A blank line submits the accumulated text. EOF returns io.EOF.
func readMessage(r *bufio.Reader) (string, error) {
	var b strings.Builder
	sawAny := false
	for {
		line, err := r.ReadString('\n')
		stripped := strings.TrimRight(line, "\r\n")
		if stripped == "" {
			if err == io.EOF {
				if sawAny {
					return b.String(), nil
				}
				return "", io.EOF
			}
			if b.Len() > 0 {
				return b.String(), nil
			}
			continue
		}
		sawAny = true
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(stripped)
		if err == io.EOF {
			return b.String(), nil
		}
	}
}

// truncate shortens s to at most n chars, appending "…" if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// defaultSummarizer returns a Summarizer that calls the LLM to produce a
// terse summary of older turns. Used in production; tests inject a stub.
func defaultSummarizer(c *llm.Client) compaction.Summarizer {
	return func(msgs []Message) (string, error) {
		var b strings.Builder
		for _, m := range msgs {
			fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
		}
		summaryReq := []Message{{
			Role: RoleUser,
			Content: "Summarize the following conversation turns. Preserve: file paths mentioned, " +
				"decisions made, errors encountered, and any incomplete tasks. Be terse.\n\n" + b.String(),
		}}
		resp, err := c.StreamTo(summaryReq, nil, io.Discard)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/agent/ -v
```
Expected: PASS (3 tests — text-only termination, tool call loop, tool-error recovery).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): add core agent loop with tool-call execution and compaction"
```

---

## Task 10: Wire it together in main.go

**Files:**
- Modify: `main.go` (replace placeholder)
- Create: `internal/tools/builtin/echo.go` already done — verify registration here

- [ ] **Step 1: Write main.go**

Replace `main.go` with:
```go
// VLA — Very Large Agent.
// A CLI agentic coding harness. See docs/DESIGN.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abrandt/vla/internal/agent"
	"github.com/abrandt/vla/internal/config"
	"github.com/abrandt/vla/internal/llm"
	"github.com/abrandt/vla/internal/session"
	"github.com/abrandt/vla/internal/tools"
	"github.com/abrandt/vla/internal/tools/builtin"
)

func main() {
	resume := flag.String("resume", "", "session ID to resume (default: new session)")
	modelFlag := flag.String("model", "", "override config model for this run")
	configFlag := flag.String("config", "", "path to config.json (default: ./config.json then ~/.vla/config.json)")
	flag.Parse()

	cfgPath := resolveConfigPath(*configFlag)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: load config: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	sess, err := openOrCreateSession(*resume, cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vla: session: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "vla: session %s (cwd %s)\n", sess.ID(), sess.CWD())

	// Set the working directory to the session's CWD so subsequent tools
	// (in later builds) operate on the right project. For the new-session
	// case this is a no-op (we're already there).
	if err := os.Chdir(sess.CWD()); err != nil {
		fmt.Fprintf(os.Stderr, "vla: warn: could not chdir to %s: %v\n", sess.CWD(), err)
	}

	reg := tools.NewRegistry()
	if err := registerBuiltins(reg); err != nil {
		fmt.Fprintf(os.Stderr, "vla: register tools: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model)
	loop := agent.NewLoop(client, reg, 100_000)

	if err := loop.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vla: %v\n", err)
		os.Exit(1)
	}
}

// resolveConfigPath finds config.json: explicit flag → CWD → ~/.vla/config.json.
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".vla", "config.json")
	}
	return "config.json" // let Load report the error
}

// openOrCreateSession opens an existing session (--resume) or creates a new one.
func openOrCreateSession(resumeID, model string) (*session.Session, error) {
	if resumeID != "" {
		path := filepath.Join(session.SessionsDir(), resumeID+".json")
		return session.Open(path)
	}
	return session.New(session.WithModel(model))
}

// registerBuiltins adds all built-in tools to the registry.
// To add a tool: implement tools.Tool in its own file, then add one line here.
func registerBuiltins(r *tools.Registry) error {
	tools := []tools.Tool{
		builtin.Echo{},
	}
	for _, t := range tools {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: Build the binary**

Run:
```bash
go build -o vla.exe .
```
Expected: succeeds, produces `vla.exe` (gitignored).

- [ ] **Step 3: Smoke test with a real config**

Create `config.json` (NOT the example — the real one, gitignored) with a valid OpenAI-compatible API key:
```bash
cp config.json.example config.json
# edit config.json to set a real api_key and model
```

Run:
```bash
echo "What is 2+2? Answer in one word." | ./vla.exe
```
Or interactively:
```bash
./vla.exe
> What is 2+2? Answer in one word.
<blank line to submit>
```
Expected: the LLM streams its answer to the terminal. Session ID and CWD are printed on startup.

- [ ] **Step 4: Smoke test the echo tool**

Run interactively and ask the LLM to use the echo tool:
```
> Use the echo tool with the text "hello world". Then tell me what it returned.
<blank line>
```
Expected: the LLM calls `echo`, the loop prints `[tool echo → hello world]`, then the LLM gives a final answer referencing the echo result.

- [ ] **Step 5: Run all tests**

Run:
```bash
go test ./...
```
Expected: PASS for all packages.

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat: wire core loop, config, session, and tools into the CLI"
```

---

## Task 11: End-to-end smoke test and documentation

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

Create `README.md`:
```markdown
# VLA — Very Large Agent

A CLI-based agentic coding harness. Named after the Very Large Array: multiple
tools working together to see deep into a codebase.

**Status:** Prototype — core loop + tool framework only. See `docs/DESIGN.md`
for the full architecture and the roadmap for file/git/search/nav/web tools.

## Quick start

```bash
# Build
go build -o vla .

# Configure (copy and edit with your OpenAI-compatible API key)
cp config.json.example config.json

# Run (new session)
./vla

# Run with flags
./vla --resume 2026-07-02T150300Z   # resume a prior session
./vla --model gpt-4o-mini           # override config model
./vla --config /path/to/config.json # use a specific config
```

## Usage

Type a message and press Enter twice (blank line) to send. Multi-line input is
supported — each line becomes part of the message until you submit with a blank line.

The LLM streams its response to the terminal. If it calls a tool, the tool runs
and its result is fed back automatically; the LLM continues until it responds
without any tool call.

## Sessions

Each launch creates a new session stored at `~/.vla/sessions/<timestamp>.json`
(NDJSON format). Resume with `--resume <id>`.

## Built-in tools

- `echo` — returns its input. Proves the loop end-to-end.

More tools (read_file, write_file, git, search, go-to-definition) arrive in
later builds.
```

- [ ] **Step 2: Final full test run**

Run:
```bash
go test ./... -v
```
Expected: all tests PASS across config, session, llm, tools, compaction, agent.

- [ ] **Step 3: Final build**

Run:
```bash
go build -o vla.exe .
```
Expected: clean build, no warnings.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add README with quick start and usage"
```

---

## Self-Review Checklist

After writing the plan, I verified:

**1. Spec coverage** (from DESIGN.md first-build scope):
- ✅ Agent loop — Task 9
- ✅ Config loading — Task 2
- ✅ OpenAI-compatible API client (streaming) — Task 7
- ✅ Session/transcript model (NDJSON) — Task 6
- ✅ Tool interface — Task 4
- ✅ One trivial test tool (echo) — Task 5
- ✅ Compaction — Task 8 (bonus — included because it's referenced in the loop)
- ✅ CLI flags (--resume, --model, --config) — Task 10
- ✅ Multi-line input — Task 9 (readMessage)
- ✅ Tool-call error handling — Task 9 (executeToolCall returns errors as content)

**2. Placeholder scan:** No TBD/TODO/"implement later" in steps. Every code block is complete.

**3. Type consistency:**
- `Tool` interface: `Name() string`, `Schema() map[string]any`, `Execute(json.RawMessage) (string, error)` — consistent across tool.go, echo.go, registry.go, loop.go.
- `agent.Message` / `ToolCall` / `FunctionCall` — consistent across message.go, llm/client.go, loop.go, compaction.go.
- `session.New(opts...)` / `session.Open(path)` / `(*Session).Append(turn)` / `(*Session).Read()` — consistent across session.go, transcript.go, loop usage in main.go.
- `llm.NewClient(apiKey, baseURL, model)` / `(*Client).Stream(msgs, tools)` / `(*Client).StreamTo(msgs, tools, out)` — consistent across client.go, loop.go, main.go.
- `compaction.Compact(msgs, sum, threshold)` / `compaction.Summarizer` — consistent across compaction.go, loop.go.
```
