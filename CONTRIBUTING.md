# Contributing to VLA

Thanks for your interest in contributing! VLA is a CLI agentic coding harness built in Go.

## Quick Start

```bash
git clone https://github.com/outerstellar-hq/vla.git
cd vla
go build -o vla .
go test ./...
```

## Project Structure

```
main.go              Entry point + subcommand routing
demo.go              `vla demo` — screenshot/GIF generation
tui_runner.go        Full-screen TUI launcher (bubbletea)
input.go             Readline fallback wrapper
adapters.go          Approval/permission bridge types
internal/
  agent/             Core agent loop, events, multi-agent coordinator
  llm/               OpenAI-compatible streaming client
  tui/               Full-screen terminal interface (bubbletea)
  tools/             Tool interface + registry
    builtin/         20+ built-in tools (file, search, git, web, memory, LSP)
  session/           NDJSON transcript persistence + session index
  indexer/           Regex symbol indexer (9 languages, polling watcher)
  lsp/               LSP client (JSON-RPC over stdio, warm pool)
  mcp/               Model Context Protocol client
  memory/            Persistent memory with embedding-based hybrid search
  compaction/        Token-aware context window compaction
  approval/          Human-in-the-loop tool approval
  permissions/       Allow/deny/ask rules from .vla/permissions.json
  commands/          Slash commands (/help, /tools, /memory, /cost, /session)
  hooks/             User-defined scripts on tool events
  plugins/           Script-based plugin system
  gitignore/         .gitignore pattern matching
  modelsdev/         models.dev catalog integration (150+ providers)
  config/            Config loader
  fsutil/            Path confinement (security)
  app/               System prompt, session helpers
```

## Development Guidelines

### Code Style

- Match the surrounding code: same naming, comment density, idioms.
- `gofmt` everything — CI runs `golangci-lint`.
- Keep functions focused; prefer composition over inheritance.
- Comments explain *why*, not *what*.

### Testing

**Tests must be deterministic.** No API keys, no network calls, no LLM interactions.

- Use fake/stub implementations for interfaces (see `fakeStreamer` in `internal/agent/events_test.go`).
- Use `bytes.Buffer` for I/O, `json.RawMessage` for tool args.
- Run tests: `go test ./... -count=1`
- Race detector: `go test -race ./...`

### Adding a New Tool

1. Create `internal/tools/builtin/<your_tool>.go` — implement the `tools.Tool` interface (`Name()`, `Schema()`, `Execute()`).
2. Register it in `internal/app/tools.go` (`RegisterBuiltins`).
3. Write tests in `internal/tools/builtin/<your_tool>_test.go`.

### Adding a New Language (Indexer)

1. Create `internal/indexer/<lang>_parser.go` — implement the `Parser` interface.
2. Register it in `internal/indexer/parser.go`.
3. Add LSP spec in `internal/lsp/specs.go` if a language server exists.
4. Write parser tests with sample code.

### Adding a Slash Command

1. Add the command to `internal/commands/commands.go`.
2. Add it to `knownSlashCommands` in `internal/tui/model.go` for autocomplete.
3. Test the command handler.

### TUI Changes

If you change TUI rendering, the screenshot pipeline auto-regenerates:
- Static PNGs: `assets/demo-*.png`
- Animated GIF: `assets/demo.gif`

Verify your changes render correctly: `./vla demo --out=/tmp/` then inspect the `.ansi` files.

## Pull Requests

1. Branch from `main`: `git checkout -b feature/my-feature`.
2. Keep PRs focused — one feature or fix per PR.
3. Ensure `go build ./...`, `go vet ./...`, and `go test ./...` all pass.
4. Write a clear commit message following the existing style (lowercase prefix: `tui:`, `agent:`, `tools:`, etc.).

## Reporting Issues

The fastest way to report a bug:

```bash
vla bug --title "Short description" --body "What happened, what you expected"
```

This creates a GitHub issue directly with the `bug` label and your environment info attached. Requires the [GitHub CLI](https://cli.github.com) or a `GITHUB_TOKEN` environment variable.

Alternatively, open an issue manually at [github.com/outerstellar-hq/vla/issues](https://github.com/outerstellar-hq/vla/issues).

- **Bugs:** include steps to reproduce, expected vs actual behavior.
- **Features:** explain the use case and expected behavior.

## License

By contributing, you agree that your contributions are licensed under the MIT License.
