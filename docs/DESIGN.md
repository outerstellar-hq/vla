# VLA — Very Large Agent

## Design Document & Session Handoff

**Date:** 2026-07-02
**Status:** Design phase (brainstorming in progress, not yet implemented)
**Repository:** `C:\Develop\Claude\projects\weird\vla`
**Language:** Go
**Authors:** Alexander Brandt + friend

---

## What is VLA?

A CLI-based agentic coding harness — an agent loop where the user sends a message, the LLM performs tool calls, and the harness executes them. Named after the **Very Large Array** (VLA), the radio astronomy observatory where 27 dish antennas work in unison to see deep into space. The metaphor: multiple tools working together to see deep into a codebase.

**The differentiators that make VLA unlike other harnesses:**

1. **IDE-grade tool space.** The LLM gets tools that mirror what a human developer does in an IDE: search, replace (one and many), git, create/update/delete files, list files.
2. **Live background index.** A linter runs in the background maintaining a real-time symbol/call-graph index. Search and navigation tools query this index instead of scanning the filesystem — instant results, no grep waiting.
3. **"Ctrl+click / Ctrl+f" navigation.** The LLM traverses code exactly like a human: go-to-definition (ctrl+click), find-references (ctrl+click on a reference), exact/fuzzy text search (ctrl+f). These are tools, not raw grep/glob.
4. **Web search + web read.** Built-in tools for searching the web and reading URLs (borrowed from the Chalie project).
5. **Encapsulated, alterable tool schemas.** Every tool lives in its own dedicated file (Go struct), fully self-contained. Altering a tool's schema or behavior means editing one file — no central registry to maintain.

---

## Design decisions (confirmed)

### Language: Go

Chosen over Rust, Java+GraalVM, and Kotlin+GraalVM. Full rationale:

| Requirement | Go | Rust | Java+GraalVM | Kotlin+GraalVM |
|---|---|---|---|---|
| Single binary | ✅ native | ✅ native | ⚠️ via GraalVM (painful) | ⚠️ via GraalVM |
| Minimum LOC | ✅ best | ⚠️ verbose | ❌ ceremony | ⚠️ medium |
| Static typing | ✅ | ✅ best | ✅ | ✅ |
| Background indexer | ✅ goroutines | ⚠️ async complexity | ✅ virtual threads | ✅ coroutines |
| Prototype speed | ✅ fast compile | ❌ borrow checker | ⚠️ AOT slow | ⚠️ AOT slow |
| Tree-sitter | ✅ bindings | ✅ native (best) | ✅ bindings | ✅ bindings |
| HTTP/JSON (OpenAI API) | ✅ stdlib | ⚠️ crates needed | ✅ mature | ✅ mature |

**Key reasons:** Single static binary by default (`go build`). Goroutines for the background linter — simplest concurrency model, no async/await coloration. Stdlib covers HTTP, JSON, file I/O, process spawning without dependencies. Fast compilation for prototype iteration. Go is learnable in a weekend.

**The trade-off accepted:** Go is new to the author (Java expert). The learning curve is a weekend; the architectural simplicity is permanent.

### No MCP for the prototype

MCP adds a transport layer, server process, JSON-RPC framing, and capability negotiation — all unnecessary for a CLI where tools are in-process function calls. MCP exists for *external* tools consumed by *any* agent. VLA's tools are built into the binary.

**Decision:** Tools are plain Go structs with a `Schema()` method (returns OpenAI function-calling JSON) and an `Execute()` method. The agent loop calls them directly. Zero transport overhead. Can be wrapped as MCP later if external consumers are needed.

### "Never needs grep" → corrected to "live index"

The original framing ("the LLM never needs grep/glob because the linter runs in the background") was contradictory with also providing a ctrl+f search tool. Corrected to:

> "The harness maintains a live index via a background linter, and the search/navigation tools query that index instead of scanning the filesystem."

The agent still calls a search tool — it just doesn't wait for a filesystem scan. The index is the differentiator.

### Streaming responses (confirmed)

LLM responses stream to the terminal via SSE as tokens arrive. Tool calls are parsed from the stream when complete. Better UX — the user sees the response being typed in real-time rather than staring at a blank terminal.

### First build scope: Core loop + tool framework (confirmed)

The first build delivers ONLY:
- Agent loop (message → LLM → tool calls → results → repeat)
- Config loading (`config.json`)
- OpenAI-compatible API client (streaming)
- Session/transcript model (new session per launch, YAML frontmatter JSON)
- Tool interface (how tools register, declare schemas, execute)
- One trivial test tool to prove the loop works

**NOT in the first build:** File tools, git tools, search, navigation, background indexer, web tools. These come in subsequent builds.

### Languages supported (eventually)

Python, PHP, JavaScript, HTML, CSS, SCSS. For the prototype, pick ONE language for the indexer/navigation (Python recommended — most likely target, best tree-sitter support). The architecture must be language-pluggable but the prototype ships with one.

---

## Architecture (core loop + tool framework)

### Package layout

```
vla/
├── main.go                  # Entry point: parse flags, load config, start session
├── config.json              # API key, model, base URL
├── go.mod
│
├── internal/
│   ├── agent/
│   │   ├── loop.go          # The core agent loop: send → stream → parse tool calls → execute → append → repeat
│   │   └── message.go       # Message types (user, assistant, tool) for the OpenAI chat completions API
│   │
│   ├── config/
│   │   └── config.go        # Load + validate config.json
│   │
│   ├── llm/
│   │   └── client.go        # OpenAI-compatible streaming client (SSE parsing, tool-call extraction)
│   │
│   ├── session/
│   │   ├── session.go       # Session lifecycle: new session, CWD capture, transcript file management
│   │   └── transcript.go    # YAML frontmatter JSON read/write for turns
│   │
│   ├── tools/
│   │   ├── registry.go      # Tool registry: collects all tools, exposes schemas to the LLM
│   │   ├── tool.go          # Tool interface: Schema() + Execute() 
│   │   └── builtin/
│   │       └── echo.go      # Trivial test tool (returns its input) to prove the loop
│   │
│   └── compaction/
│       └── compaction.go    # Borrowed from Chalie: summarize old turns when context gets too long
│
└── docs/
    └── DESIGN.md            # This file
```

### The Tool interface

Every tool is a Go struct implementing this interface:

```go
package tools

// Tool is the interface every VLA tool implements.
// Each tool lives in its own file, fully self-contained.
type Tool interface {
    // Name returns the tool's unique identifier (e.g. "read_file").
    Name() string

    // Schema returns the OpenAI function-calling JSON schema for this tool's parameters.
    // This is what the LLM sees. Alter the schema here to change what the LLM can do.
    Schema() map[string]any

    // Execute runs the tool with the given JSON arguments and returns the result string.
    Execute(args json.RawMessage) (string, error)
}
```

**Why this design:**
- Each tool is fully encapsulated — one file, one struct, one schema.
- Altering a tool's schema means editing its `Schema()` method. No central registry to update.
- The registry just collects all tools and exposes their schemas to the LLM. Adding a tool = adding a file + one line in the registry.
- `json.RawMessage` lets each tool parse its own args with full type safety via `json.Unmarshal` into a tool-specific struct.

### The agent loop

```
┌──────────────────────────────────────────────┐
│  User types message                           │
│         │                                     │
│         ▼                                     │
│  Append user message to transcript            │
│         │                                     │
│         ▼                                     │
│  Build messages[] from transcript             │
│  (apply compaction if needed)                 │
│         │                                     │
│         ▼                                     │
│  Call LLM (streaming) with messages + tools   │
│         │                                     │
│         ├── tokens stream to terminal ──▶ user sees response live
│         │                                     │
│         ▼                                     │
│  Parse complete response                      │
│  (may contain text + tool_calls)              │
│         │                                     │
│         ▼                                     │
│  Append assistant message to transcript       │
│         │                                     │
│         ├── has tool_calls?                   │
│         │     │                               │
│         │     ├── YES ──▶ for each tool_call: │
│         │     │              execute tool      │
│         │     │              append result to  │
│         │     │              transcript        │
│         │     │              │                 │
│         │     │              ▼                 │
│         │     │         loop back to LLM call  │
│         │     │                               │
│         │     └── NO ──▶ done, wait for user   │
│         │                                     │
│         ▼                                     │
│  Prompt user for next message                 │
└──────────────────────────────────────────────┘
```

The loop continues automatically when the LLM requests tool calls — the user only intervenes when the LLM finishes responding without tool calls.

### Config format (`config.json`)

```json
{
    "api_key": "sk-...",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o"
}
```

Deliberately minimal. `base_url` makes it OpenAI-compatible (works with any provider that implements the OpenAI chat completions API: OpenAI, OpenRouter, Together, local vLLM, etc.).

### Session & transcript model

**On launch:**
1. Create a new session (UUID or timestamp-based ID).
2. Capture CWD automatically (`os.Getwd()`).
3. Create a transcript file: `~/.vla/sessions/<session-id>.json` (or in the project dir — TBD).
4. Start the agent loop with an empty transcript.

**Transcript format:** YAML frontmatter + JSON body per turn.

```yaml
---
session: 2026-07-02T150300Z
cwd: /home/user/myproject
model: gpt-4o
created: 2026-07-02T15:03:00Z
---
```

Followed by JSON turns (one per line or array):

```json
{"role": "user", "content": "Fix the login bug in auth.py", "timestamp": "2026-07-02T15:03:01Z"}
{"role": "assistant", "content": "I'll investigate the auth module...", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"auth.py\"}"}}], "timestamp": "2026-07-02T15:03:02Z"}
{"role": "tool", "tool_call_id": "call_1", "content": "...file contents...", "timestamp": "2026-07-02T15:03:02Z"}
```

Each turn is stored as it happens — the transcript is the source of truth for the conversation. On the next LLM call, the full transcript (or compacted version) is sent as `messages[]`.

### Compaction (borrowed from Chalie)

When the transcript grows too long for the LLM's context window, old turns are summarized into a compact representation. The exact algorithm is borrowed from the Chalie project — likely: identify the oldest N turns, summarize them into a single "system/context" message, replace them in the transcript view sent to the LLM.

**For the prototype:** A simple threshold (e.g. when transcript exceeds 50K chars) triggers compaction. The implementation should be a separate function that transforms `[]Message` → `[]Message`, so it's easy to test and swap algorithms.

---

## What comes AFTER the core loop (future builds)

These are documented for context but NOT part of the first build:

### Phase 2: File + Git tools
- `read_file`, `write_file`, `update_file` (diff-based), `delete_file`, `list_files`
- `git_status`, `git_diff`, `git_commit`
- Each in its own file under `internal/tools/builtin/`

### Phase 3: Search tool (ctrl+f)
- Exact match + fuzzy search over the codebase.
- For v1: wrap ripgrep (`rg`) in a tool. Fast enough.
- For v2: query the live index maintained by the background linter.

### Phase 4: Background indexer (the differentiator)
- A goroutine that runs a linter/parser (tree-sitter) on the codebase.
- Maintains an in-memory index of: symbols, definitions, references, call graph.
- Invalidates and re-indexes on file change (fsnotify or polling).
- The search and navigation tools query this index for instant results.

### Phase 5: Navigation tools (ctrl+click)
- `go_to_definition` — given a file:line:col, find where the symbol is defined.
- `find_references` — given a file:line:col, find all places that reference it.
- Backed by tree-sitter for the prototype (no LSP server dependency), or by the background index from Phase 4.

### Phase 6: Web tools
- `web_search` — search the web, return results.
- `web_read` — fetch and parse a URL, return text content.
- Borrowed from the Chalie project.

---

## Coding standards (from the original brief)

- **Absolute bare minimum LOC.** Every line is a liability. Solve more with less.
- **All tools live in dedicated files (classes/structs)** which are fully encapsulated and where the schema can be altered easily.
- **Static typing only** (Go satisfies this).
- **Errors surface loudly** — no silent swallowing, no sentinel returns (aligned with Chalie's principles).
- **Only OpenAI-compatible LLM APIs.**
- **Only these languages (eventually):** Python, PHP, JavaScript, HTML, CSS, SCSS.

---

## Session handoff notes

### Where we are in the design process

The brainstorming was in progress when this document was written. The following design decisions are **confirmed**:
- Language: Go
- No MCP for prototype
- Streaming responses
- Core loop + tool framework as first scope
- Tool interface as Go struct with `Name()`, `Schema()`, `Execute()`
- Config format: minimal JSON
- Transcript: YAML frontmatter + JSON turns
- New session per launch, auto-CWD

### What's NOT yet decided

These questions were not yet reached in brainstorming:
1. **Transcript storage location:** `~/.vla/sessions/` or in-project `.vla/` directory?
2. **Compaction algorithm details:** exact threshold, summarization prompt, how compacted turns are represented.
3. **Tool-call error handling:** when a tool fails, does the loop retry, abort, or let the LLM decide?
4. **Token counting:** do we count tokens ourselves (tiktoken Go port) or estimate from chars?
5. **CLI flags:** any beyond the implicit "launch = new session"? (e.g. `--resume <session-id>`, `--model`, `--config`)
6. **Multi-line input handling:** how does the user enter multi-line messages in the terminal?

### Next steps for the new session

1. Resolve the open questions above.
2. Finalize the design doc and get approval.
3. Write the implementation plan (writing-plans skill).
4. Execute (subagent-driven development).

### Key references

- **Chalie project:** `C:\Develop\Claude\projects\weird\chalie\` — source of compaction logic, web tools, coding standards philosophy.
- **Memwizard LSP work:** `C:\Develop\Claude\projects\weird\wizardmem\` — reference for how to build a `LanguageServer` SPI, tool registration patterns, and the LSP navigation concepts that inspired VLA's ctrl+click metaphor.
- **Contributing-to-chalie skill:** `C:\Users\Alexander\.agents\skills\contributing-to-chalie\SKILL.md` — coding standards, lean code principles, test discipline.

---

## Appendix: Original brief (verbatim)

> You are designing an agentic coding harness that is no other like any other harness.
>
> It will be a CLI tool
> It will be a simple agent loop where the user sends a message and the LLM can perform tool calls
> It will only support openai-compatible LLM APIs
> The LLM API connection info & model selection will be a simple config.json file
> Every turn is stored into a simple yaml frontmatter json file where we store each user input, assistant response and tool call results (attached to the assistant response)
> We borrow the compaction logic from Chalie
> Each time we open the cli tool it should a new session with a new transcript
> The tool automatically captures the CWD based on where it was launched from
>
> The differentiators:
> 1. The LLM tool space should expose basic tools you would find in an IDE: search, replace one, replace many, git, create file, list files, delete file, update file
> 2. This harness will run a full linter in the background the same way a typical IDE would so the LLM should never need to reach out for glob or grep
> 3. This harness will only support the languages; python, php, js, html, css, scss
> 4. The LLM inside this harness can traverse the code the exact same way a human would traverse the codebase; "ctrl + clicking" tells us the callers and callees, "ctrl + f" lets us make an exact match or fuzzy search for a term, etc... the "ctrl + <action>" for the LLM is to be a tool
> 5. The harness has a built-in web search + web read tool (take from Chalie)
>
> Coding standards:
> - The harness needs to be designed so it's the absolute bare minimum LOC
> - All tools the harness will have must live in dedicated files (classes) which are fully encapsulated and where I can alter the schema of the tool easily
