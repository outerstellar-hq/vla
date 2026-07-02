# VLA — Very Large Agent

A CLI-based agentic coding harness. Named after the Very Large Array: multiple
tools working together to see deep into a codebase.

**Status:** All 6 phases implemented. 114 deterministic tests, stdlib-only Go.

## What makes VLA different

1. **IDE-grade tool space.** The LLM gets tools that mirror what a human developer does in an IDE: file read/write/update/delete, list, search, git, web.
2. **Live background index.** A polling watcher maintains a real-time symbol/call-graph index. Navigation tools query this index — instant results.
3. **Ctrl+click / Ctrl+f navigation.** `go_to_definition` and `find_references` traverse code exactly like a human in an IDE. These are tools backed by the live index, not grep.
4. **Web search + web read.** Built-in, no API key required.
5. **Encapsulated tools.** Every tool is a self-contained Go struct in its own file. Altering a tool's schema = editing one file.

## Quick start

```bash
# Build
go build -o vla .

# Configure (copy and edit with your OpenAI-compatible API key)
cp config.json.example config.json

# Run (new session — indexes the project on startup)
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
(NDJSON format: line 1 is session metadata, subsequent lines are turns). Resume
with `--resume <id>`.

## Built-in tools

| Tool | Description |
|------|-------------|
| `echo` | Returns its input (test tool) |
| `read_file` | Read file contents (capped at 256 KiB) |
| `write_file` | Create or overwrite a file |
| `update_file` | Find-and-replace within a file |
| `delete_file` | Delete a file (refuses directories) |
| `list_files` | List project files (skips noise dirs) |
| `search` | Codebase text search (ripgrep or Go fallback) |
| `git_status` | Show working tree status |
| `git_diff` | Show staged or unstaged changes |
| `git_commit` | Stage all + commit |
| `web_search` | Search the web (DuckDuckGo, no API key) |
| `web_read` | Fetch a URL, strip HTML to text |
| `go_to_definition` | Find where a symbol is defined |
| `find_references` | Find all usages of a symbol |

Adding a tool: implement `tools.Tool` in its own file under `internal/tools/builtin/`, then add one line to `RegisterBuiltins` in `internal/app/app.go`.

## Architecture

```
main.go                  → flags, config, session, indexer, loop wiring
internal/agent/          → core loop + message types
internal/llm/            → OpenAI-compatible streaming client (SSE)
internal/session/        → session lifecycle + NDJSON transcript
internal/indexer/        → live symbol index + polling watcher
internal/tools/          → Tool interface + registry
internal/tools/builtin/  → all 14 built-in tools (one file each)
internal/compaction/     → context-window compaction
internal/config/         → config.json loader
internal/fsutil/         → path confinement (project root safety)
internal/app/            → wiring (config discovery, tool registration)
```

**No external dependencies.** Pure Go stdlib. The `go_to_definition` and
`find_references` tools use a regex-based parser for Python and Go by default;
the `Parser` interface allows swapping in tree-sitter later without changing
the tool surface.

## Testing

```bash
go test ./...        # 114 tests, ~8 seconds
go test -v ./...     # verbose
```

All tests are deterministic — no API keys, no real network, no external services.
LLM interactions are simulated with `httptest.NewServer`; file/git operations
run against temp directories.
