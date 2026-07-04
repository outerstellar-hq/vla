# VLA Architecture

This document describes the internal architecture of VLA as of v0.1.0. For
the original design rationale (including rejected alternatives), see
[DESIGN.md](DESIGN.md) — note that DESIGN.md reflects the prototype state
and many features described there as "future" are now implemented.

## Overview

VLA is a CLI agentic coding harness. The user sends a message, VLA streams
it to an LLM, the LLM can respond with tool calls (read files, search code,
write files, run git commands, etc.), VLA executes them and feeds the
results back to the LLM, and the cycle repeats until the LLM responds with
plain text (no tool calls).

```
User → Input (TUI/Readline) → Agent Loop → LLM (streaming)
                                      ↓
                                 Tool Call?
                              ↙           ↘
                           Yes             No
                            ↓               ↓
                     Execute Tool      Stream Response
                     (approval,         → User
                      permissions,
                      hooks)
                            ↓
                     Append Result
                     to Messages
                            ↓
                     Loop Back to LLM
```

## Package Layout

```
main.go              Entry point + subcommand routing
demo.go              `vla demo` — screenshot/GIF generation
bug_report.go        `vla bug` — creates GitHub issues directly
tui_runner.go        Full-screen TUI launcher (bubbletea)
input.go             Readline fallback wrapper
adapters.go          Approval/permission bridge types
hooks_adapter.go     Hook runner bridge type

internal/
  agent/             Core agent loop, events, multi-agent coordinator
    loop.go          Loop: streaming → tool calls → compaction → persist
    events.go        Typed Event system (tool start/result, usage, turn)
    multi.go         Coordinator: parallel sub-agents
    memory_inject.go ContextInjector: auto-inject memories before LLM calls
    message.go       Message, ToolCall, ContentPart (OpenAI-compatible)
    loop.go          Interfaces: Streamer, Compactor, ToolApprover, etc.

  llm/               OpenAI-compatible streaming chat-completions client
    client.go        SSE parsing, tool-call fragment assembly, usage tracking

  tui/               Full-screen terminal interface (bubbletea)
    model.go         Bubbletea Model: viewport, textarea, key handling
    blocks.go        Block-based rendering (user, assistant, tool, system)
    diff.go          LCS diff engine for split-pane diff preview
    sessions.go      Session picker overlay (Ctrl+S)
    approval.go      TUI-native approver (fixes ReadlineApprover deadlock)
    bridge.go        ChannelInput/ChannelWriter for loop ↔ TUI
    markdown.go      Glamour-based markdown renderer
    demo.go          Off-screen frame renderer for screenshots/GIFs

  tools/             Tool framework
    tool.go          Tool interface: Name(), Schema(), Execute()
    registry.go      Registry: register, get, schemas
    builtin/         20+ built-in tools (each in its own file)

  session/           NDJSON transcript persistence
    session.go       Session: create, open, metadata
    transcript.go    Append/Read NDJSON turns
    index.go         Cross-project session index (~/.vla/sessions/index.json)

  indexer/           Regex-based symbol indexer
    indexer.go       Parallel build (4-goroutine worker pool)
    parser.go        Parser interface + language detection
    *_parser.go      9 language parsers (Go, Python, Kotlin, Java, C#, PHP, JS/TS, CSS, HTML)
    watcher.go       Polling watcher (stdlib, no fsnotify)

  lsp/               LSP client (JSON-RPC over stdio)
    client.go        Content-Length framing, request/response/notify
    manager.go       Warm pool: pre-started servers, crash recovery
    specs.go         Language specs (gopls, pyright, etc.)

  mcp/               Model Context Protocol client
    client.go        Newline-delimited JSON-RPC over stdio
    manager.go       Server lifecycle management
    config.go        .vla/mcp.json parsing
    adapter.go       MCP tool → VLA Tool interface adapter

  memory/            Persistent memory with hybrid search
    store.go         JSON store per project (~/.vla/memory/)
    embedding.go     Embedding client (OpenAI-compatible)
    search.go        Hybrid search: keyword + cosine similarity

  compaction/        Token-aware context window compaction
    compaction.go    Compact: summarize old messages, keep recent window

  approval/          Human-in-the-loop tool approval
    approval.go      Approver interface
    readline_approver.go  Interactive y/n/a prompt (readline mode)

  permissions/       Tool permission rules
    permissions.go   Load .vla/permissions.json, allow/deny/ask per tool

  commands/          Slash commands
    commands.go      /help, /tools, /memory, /compact, /session, /cost, /model

  hooks/             User-defined scripts on tool events
    hooks.go         before_tool, after_tool, on_write events

  plugins/           Script-based plugin system
    plugins.go       .vla/plugins/<name>/plugin.json + run.{sh,py,js,cmd}

  gitignore/         .gitignore pattern matching
    gitignore.go     Exact, wildcard, dir, negation patterns

  modelsdev/         models.dev catalog integration
    modelsdev.go     Provider/model catalog client (150+ providers)
    commands.go      `vla models` and `vla use` subcommands

  config/            Config loader
    config.go        JSON config: api_key, base_url, model, context_limit

  sandbox/           OS-level process sandbox
    sandbox.go       Mode detection (sandbox-exec, bwrap, none)
    sandbox_darwin.go  macOS Seatbelt profile
    sandbox_linux.go   Linux bubblewrap arguments
    sandbox_windows.go Stub (not supported)

  fsutil/            Path confinement
    fsutil.go        Confine: lexical + symlink-safe path checking

  app/               Application glue
    app.go           System prompt, session helpers, tool registration
    resume.go        Load transcript messages for --resume
```

## Agent Loop Lifecycle

The agent loop (`agent.Loop.Run()`) is the heart of VLA. One "turn" is:

1. **Read input** — from the TUI's channel, readline, or piped stdin.
2. **Slash command?** — if input starts with `/`, handle locally (no LLM call).
3. **Compaction** — if the message list exceeds the token threshold, summarize old messages and keep only the recent window.
4. **Context injection** — inject relevant memories (embedding-based search on the last user message).
5. **Stream to LLM** — POST to `/chat/completions` with `stream: true`. Parse SSE chunks, write text deltas to the output writer (TUI sees them stream in real-time).
6. **Tool calls?** — if the LLM response includes `tool_calls`:
   - Emit `EventToolStart` (TUI shows running spinner + tool block).
   - Check permissions (deny = blocked entirely).
   - Check approval (destructive tools → y/n/a prompt).
   - Run before_tool hooks (can block).
   - Execute the tool.
   - Run after_tool/on_write hooks.
   - Emit `EventToolResult`.
   - Append the result as a `RoleTool` message.
   - Loop back to step 3.
7. **No tool calls?** — the turn is done. Emit `EventTurnEnd`. Go back to step 1.

### Max Turns Protection

The inner loop (steps 3-6) is capped at 50 iterations (`MaxTurns`). If the LLM keeps requesting tool calls forever, VLA aborts and returns control to the user.

## Event System

The loop emits typed events via a non-blocking channel (`SetEventChan`). The TUI uses these for:
- `EventTurnStart` / `EventTurnEnd` — spinner on/off, streaming state
- `EventToolStart` / `EventToolResult` — tool blocks with expand/collapse
- `EventUsage` — live token count in the status bar

Events are best-effort: if the channel is full, events are dropped (the loop never blocks on UI updates).

## Threading Model

```
Main goroutine          Bubbletea goroutine       Agent loop goroutine
│                       │                         │
├─ runTUI()             ├─ p.Run() (tea.Program)  ├─ loop.Run()
│  └─ for {             │  └─ Model.Update()      │     └─ turn()
│       select {        │     └─ handle events    │         └─ StreamTo()
│       ←loopDone       │        from channels    │         └─ executeToolCall()
│       ←switchCh       │                         │
│       }               │                         │
│     }                 │                         │
```

- **Main goroutine**: creates the TUI + loop, watches for completion or session switch.
- **Bubbletea goroutine**: renders the UI, handles key events, polls channels.
- **Agent loop goroutine**: blocks on `loop.Run()`, streams tokens to the `ChannelWriter`.

Communication is via channels:
- `inputReady chan string` — TUI → loop (user messages)
- `streamWriter.Chan()` — loop → TUI (streaming text tokens via `io.Writer`)
- `eventCh chan agent.Event` — loop → TUI (typed events)
- `switchCh chan string` — TUI → runner (session switch requests)

## Security Model

VLA has four layers of defense against the LLM doing unintended things:

### Layer 1: Path Confinement (`fsutil.Confine`)

Every filesystem tool calls `fsutil.Confine(baseDir, path)` before operating. This:
- Lexically checks the path is within `baseDir` (catches `../` escapes).
- Resolves symlinks via `filepath.EvalSymlinks` and re-checks (catches symlink escapes).
- For non-existent paths (new files), resolves the parent directory.

### Layer 2: OS-Level Sandbox (`--sandbox` flag)

Optional process-level isolation. When enabled, VLA re-executes itself inside:
- **macOS**: `sandbox-exec` (Seatbelt sandbox profile restricting FS to project dir)
- **Linux**: `bwrap` (bubblewrap user namespaces, read-only system mounts)
- **Windows**: not supported (relies on Layer 1 + user account restrictions)

### Layer 3: Permission Rules (`.vla/permissions.json`)

Per-tool allow/deny/ask rules checked before tool execution. Deny = tool never runs.

### Layer 4: Diff Approval (`ToolApprover`)

Destructive tools (`write_file`, `update_file`, `delete_file`, `git_commit`) require human approval before executing. In TUI mode, this is an inline y/n/a prompt. With `--yes`, approval is skipped.

## Multi-Agent Coordinator

The `agent.Coordinator` can spawn N sub-agents in parallel goroutines. Each sub-agent:
- Has its own message history (independent conversation with the LLM).
- Shares the tool registry (can call any tool).
- Executes tool calls autonomously (no approval prompts — sub-agents are trusted).
- Returns a result string collected by the coordinator.

Results are gathered in order and formatted as a combined summary. Failures in one sub-agent don't block others.

## Session Persistence

Sessions are stored as NDJSON files in `~/.vla/sessions/<id>.json`. Each line is a JSON object:
- First line: session metadata (`type: "session"`, `id`, `cwd`, `model`, `created`).
- Subsequent lines: turns (`type: "turn"`, `role`, `content`, `tool_calls`, `tool_call_id`, `timestamp`).

A cross-project index at `~/.vla/sessions/index.json` maps session IDs to project paths, models, and timestamps for the session switcher and `vla sessions` command.

## Compaction

When the message list exceeds the token threshold (default: 75% of the model's context limit), old messages are summarized:
1. Split messages into "recent window" (kept as-is) and "old" (summarized).
2. The summarizer calls the LLM with a "summarize this conversation" prompt.
3. The summary replaces the old messages as a single `RoleSystem` message.
4. The recent window is preserved verbatim.

This prevents context window overflow while maintaining conversation continuity.
