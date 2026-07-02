# VLA — Very Large Agent

A CLI-based agentic coding harness. Named after the Very Large Array: multiple
tools working together to see deep into a codebase.

**Status:** Prototype — core loop + tool framework only. See [`docs/DESIGN.md`](docs/DESIGN.md)
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
(NDJSON format: line 1 is session metadata, subsequent lines are turns). Resume
with `--resume <id>`.

## Built-in tools

- `echo` — returns its input. Proves the loop end-to-end.

More tools (read_file, write_file, git, search, go-to-definition) arrive in
later builds. Each tool is a self-contained Go struct in its own file under
`internal/tools/builtin/`; adding one is one file + one registration line in
`main.go`.

## Architecture

```
main.go                  → flags, config discovery, session, loop wiring
internal/agent/          → core loop + message types (the heart)
internal/llm/            → OpenAI-compatible streaming client (SSE parsing)
internal/session/        → session lifecycle + NDJSON transcript I/O
internal/tools/          → Tool interface + registry + builtin/echo
internal/compaction/     → context-window compaction (view transform)
internal/config/         → config.json loader
```

Stdlib only — no external Go dependencies.
