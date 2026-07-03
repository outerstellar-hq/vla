# VLA — Very Large Agent

A CLI-based agentic coding harness with persistent memory and LSP-backed code intelligence. Named after the Very Large Array: multiple tools working together to see deep into a codebase.

**142 deterministic tests. Zero external dependencies. Pure Go stdlib.**

## What makes VLA different

1. **IDE-grade tool space.** 20 built-in tools: file read/write/update/delete, list, search, git, web, memory, navigation.
2. **Persistent memory.** The agent remembers across sessions. Memories are stored per-project with embedding-based semantic search, auto-injected into context before each LLM call. Inspired by Memwizard.
3. **LSP-backed navigation.** When a language server (gopls, pyright) is available, go-to-definition, find-references, hover, and diagnostics use real LSP — the same engine that powers VS Code. Falls back to a regex-based indexer when no server is installed.
4. **Live background index.** A polling watcher maintains a real-time symbol/call-graph index even without LSP.
5. **Ctrl+click / Ctrl+f navigation.** `go_to_definition` and `find_references` like an IDE.
6. **Web search + web read.** Built-in, no API key required.
7. **Encapsulated tools.** Every tool is a self-contained Go struct in its own file.

## Quick start

```bash
go build -o vla .

# Option A: Configure with models.dev (auto-discovers 150+ providers)
export OPENAI_API_KEY=sk-...
./vla use openai/gpt-4o       # writes config.json automatically

# Option B: Manual config
cp config.json.example config.json  # edit with your OpenAI-compatible API key
./vla

# Browse available models
./vla models                    # list all providers
./vla models openai             # list OpenAI models
./vla models anthropic claude   # filter by name

# Run the agent
./vla
```

## Built-in tools

| Tool | Description |
|------|-------------|
| **File** | |
| `read_file` | Read file contents (capped at 256 KiB) |
| `write_file` | Create or overwrite a file |
| `update_file` | Find-and-replace within a file |
| `delete_file` | Delete a file |
| `list_files` | List project files |
| `search` | Codebase text search (ripgrep or Go fallback) |
| **Git** | |
| `git_status` | Show working tree status |
| `git_diff` | Show staged or unstaged changes |
| `git_commit` | Stage all + commit |
| **Navigation** | |
| `go_to_definition` | Find where a symbol is defined (LSP or regex) |
| `find_references` | Find all usages of a symbol (LSP or regex) |
| `hover` | Type/signature/docs at a position (LSP only) |
| `diagnostics` | Lint/type errors for a file (LSP only) |
| **Memory** | |
| `memory_save` | Store a fact or knowledge for later |
| `memory_search` | Search stored memories (keyword + semantic) |
| `memory_list` | List all memories for the current project |
| `memory_delete` | Delete a memory by ID |
| **Web** | |
| `web_search` | Search the web (DuckDuckGo, no API key) |
| `web_read` | Fetch a URL, strip HTML to text |

## Architecture

```
main.go                  → flags, config, session, indexer, LSP, memory, loop
internal/agent/          → core loop + message types + context injection
internal/llm/            → OpenAI-compatible streaming client (SSE)
internal/lsp/            → LSP client (JSON-RPC) + process manager
internal/memory/         → persistent memory store + embeddings + hybrid search
internal/session/        → session lifecycle + NDJSON transcript
internal/indexer/        → regex symbol index + polling watcher
internal/tools/builtin/  → all 20 tools (one file each)
internal/compaction/     → context-window compaction
internal/fsutil/         → path confinement
internal/app/            → wiring (config discovery, tool registration)
internal/config/         → config.json loader
```

## Memory system

Memories are stored as JSON files under `~/.vla/memory/<project>/`. Each memory carries content, tags, and an embedding vector (via the OpenAI embeddings API). Search is hybrid: keyword (substring on content/tags) fused with vector cosine similarity, min-max normalized and weighted (0.7 vector / 0.3 keyword).

Before each LLM call, the agent searches memories relevant to the current user message and injects them as a system message — so the agent has context from all previous sessions without being told.

## LSP integration

When a language server is installed (gopls for Go, pyright for Python), VLA uses it for:
- `go_to_definition` — position-aware, not just name matching
- `find_references` — precise, not heuristic
- `hover` — type signatures, documentation
- `diagnostics` — compile/lint errors with severity

If no server is available, navigation falls back to the regex-based indexer. The LSP client speaks the base protocol (Content-Length framing) over stdio, with a warm process pool (one server per language + workspace).

## Testing

```bash
go test ./...        # 142 tests, ~10 seconds
```

All tests are deterministic — no API keys, no real network, no LSP servers required. LLM interactions use `httptest.NewServer`; LSP client tests use `net.Pipe` to simulate a server in-process.
