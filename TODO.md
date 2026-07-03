# VLA — TODO

Open work items, prioritized by impact. Each item includes what to build,
where it goes, and why it matters.

---

## P0 — Trust & Safety (blocks daily use)

### 1. Diff Approval System
**What:** Before `write_file`, `update_file`, or `delete_file` executes, show the user a unified diff and let them approve (y/n), approve all, or reject.

**Where:**
- `internal/agent/loop.go` — add an `Approver` interface injected into `executeToolCall`
- `internal/approval/` — new package with `DiffApprover` (TUI prompt), `AlwaysApprover` (auto), `NeverApprover` (dry-run)
- `internal/tui/model.go` — render diff in a modal/popup, capture y/n keystroke

**Why:** This is the #1 reason people trust Claude Code. Without it, the LLM can modify files with zero human oversight. The path confinement prevents escaping the project, but doesn't prevent destructive changes *within* it (e.g. `rm -rf src/`).

**Design:**
```go
type Approver interface {
    Approve(toolName string, args map[string]any, preview string) (bool, error)
}
```
- `DiffApprover` renders the diff and waits for y/n in the TUI or readline.
- `AlwaysApprover` auto-approves (for `--yes` flag or piped input).
- Configurable per-tool: some tools (read_file, search) never need approval; others (delete_file, git_commit) always do.

---

### 2. Permission System
**What:** Allow/deny rules per tool, configurable via `.vla/permissions.json` or CLI flags. Examples: "deny git_push", "allow read_file everywhere", "ask before write_file in /docs".

**Where:**
- `internal/permissions/` — new package
- `.vla/permissions.json` — config file
- `internal/agent/loop.go` — check permissions before `executeToolCall`

**Why:** Without permissions, every tool call runs unconditionally. A user may want the LLM to read and analyze but not modify. Or restrict git operations to non-destructive ones (status/diff but not commit/push).

**Config format:**
```json
{
  "rules": [
    {"tool": "git_commit", "action": "deny"},
    {"tool": "write_file", "action": "ask"},
    {"tool": "delete_file", "action": "deny"},
    {"tool": "read_file", "action": "allow"}
  ],
  "default": "ask"
}
```

---

### 3. Sandbox (OS-level file access control)
**What:** Run tool operations inside an OS-level sandbox that restricts filesystem access beyond what `fsutil.Confine` provides. On macOS use `sandbox-exec`; on Linux use `bwrap` or `landlock`; on Windows use job objects.

**Where:**
- `internal/sandbox/` — new package with per-OS implementations
- `main.go` — optionally launch VLA inside a sandbox

**Why:** `fsutil.Confine` is lexical — it prevents `../` escapes in paths but doesn't prevent symlinks from pointing outside the project. An OS-level sandbox is defense-in-depth. Claude Code does this on macOS.

**Note:** Lower priority than diff approval + permissions because path confinement already covers the common case. This is for users who want guarantees.

---

## P1 — Functionality (matches competitor features)

### 4. Better Compaction (token-aware)
**What:** Replace the char-based threshold with real token counting. Use strategic inclusion/exclusion of prior tool results (e.g. summarize a 50KB file read instead of keeping it verbatim). Stitch conversation fragments intelligently.

**Where:**
- `internal/compaction/compaction.go` — replace `totalChars` with token counting
- `internal/tokenizer/` — new package (tiktoken Go port or API-based counting)

**Why:** The current compaction triggers at 100K chars (~25K tokens estimated). This is inaccurate for models with 128K-2M context windows. A GPT-4o model with 128K context should compact at ~96K tokens, not ~25K. models.dev gives us the real context limit — we should use it.

**Specifics:**
- Count tokens using the model's tokenizer (tiktoken for OpenAI, API `usage` response for others)
- When compacting, summarize large tool results individually rather than all old turns en masse
- Keep the most recent tool result verbatim (it's likely relevant to the current task)
- Track token count per message so we don't re-count on every turn

---

### 5. Plan / Build Modes
**What:** Separate planning from execution. In "plan" mode the LLM investigates and proposes a plan without making changes. In "build" mode it executes the plan with tool calls. User approves the plan before build starts.

**Where:**
- `main.go` — `--plan` flag or `vla plan` subcommand
- `internal/agent/loop.go` — in plan mode, restrict tools to read-only (read_file, search, list_files, go_to_definition, find_references)
- `internal/plan/` — new package for plan data model + storage

**Why:** OpenCode and Claude Code both have this. It prevents the LLM from making changes while it's still understanding the problem. The plan becomes a checkpoint the user can review.

---

### 6. Multi-Language Support (PHP, JS, HTML, CSS, SCSS)
**What:** Extend the indexer parser and LSP defaults to handle the languages listed in the design doc.

**Where:**
- `internal/indexer/parser.go` — add `phpParser`, `jsParser`, `cssParser`
- `internal/lsp/manager.go` — add server specs for PHP (intelephense), JS/TS (typescript-language-server)

**Why:** The design doc specifies: "Only these languages (eventually): Python, PHP, JavaScript, HTML, CSS, SCSS." Currently only Python and Go are supported.

**Per-language needs:**
- PHP: intelephense LSP, regex parser for `function`, `class`, `interface`
- JS/TS: typescript-language-server, regex for `function`, `const`, `class`, `export`
- CSS/SCSS: regex for selectors, no LSP needed (static analysis suffices)
- HTML: regex for element IDs, classes; integration with CSS selector index

---

## P2 — Polish (improves UX)

### 7. Image / Multimodal Support
**What:** Allow the user to paste or reference images in messages. The LLM client sends them as base64-encoded content parts in the message array.

**Where:**
- `internal/agent/message.go` — add `ImageURL` or `ImageBase64` field
- `internal/llm/client.go` — include image content parts in the request body
- `internal/tui/model.go` — accept image paths or paste handlers
- models.dev `attachment` field tells us which models support images

**Why:** Vision models (GPT-4o, Gemini, Claude) can analyze screenshots, diagrams, UI mockups. Without image support, VLA can't handle "look at this error screenshot" or "implement this design."

---

### 8. Cost Tracking
**What:** Track token usage and cost per session. Display running total in the status bar. Store per-session cost in the transcript metadata.

**Where:**
- `internal/llm/client.go` — parse `usage` from the API response (prompt_tokens, completion_tokens)
- `internal/cost/` — new package that maps model → pricing (from models.dev) and accumulates
- `internal/tui/model.go` — show "$0.04 | 12K tokens" in status bar

**Why:** Users need to know how much a session costs. models.dev gives us pricing data. The API returns token counts. Connecting them is straightforward.

---

### 9. TUI Polish
**What:** Improve the bubbletea TUI with features users expect from Claude Code / OpenCode.

**Where:** `internal/tui/model.go`

**Specifics:**
- Tool call indicators with expand/collapse (show/hide full tool output)
- Markdown rendering for assistant messages (code blocks with syntax highlighting)
- Split-pane mode: conversation + live diff preview side by side
- `/help`, `/tools`, `/memory` slash commands within the TUI
- Auto-scroll lock (stop scrolling when user scrolls up to read history)
- Session switcher (`/sessions` to list and switch)

---

### 10. Slash Commands
**What:** In-app commands prefixed with `/` that invoke tools or change settings without going through the LLM.

**Where:**
- `internal/agent/loop.go` — intercept messages starting with `/`
- `internal/commands/` — new package

**Examples:**
- `/tools` — list registered tools
- `/memory search <query>` — search memories directly
- `/model <name>` — switch model mid-session
- `/compact` — manually trigger compaction
- `/save <description>` — save current state as a memory
- `/undo` — undo the last file change (needs a change journal)
- `/help` — show available commands

---

## P3 — Future (nice to have)

### 11. Multi-Agent / Parallel Execution
**What:** Run multiple agent loops in parallel (e.g. one investigates the bug, another writes tests, a third updates docs).

**Where:**
- `internal/agent/` — add a `MultiLoop` coordinator
- `main.go` — `--parallel` flag

**Why:** OpenCode does this (~3x speedup for independent tasks). Complex.

---

### 12. Plugin System
**What:** Let users write custom tools as Go plugins (compiled .so files) or scripts that VLA loads at startup.

**Where:**
- `internal/plugins/` — new package
- `.vla/plugins/` — directory for user plugins

---

### 13. Hooks
**What:** User-defined scripts that run before/after specific events (before tool call, after file write, on session start).

**Where:**
- `internal/hooks/` — new package
- `.vla/hooks.json` — config

**Examples:**
- Run linter after every `write_file`
- Run tests after every `update_file`
- Notify on `git_commit`

---

### 14. Session Index
**What:** A `~/.vla/sessions/index.json` mapping session IDs to project paths, timestamps, and summaries. Enables cross-project session browsing.

**Where:**
- `internal/session/` — add index management

**Why:** Currently sessions are scattered files. An index enables `vla sessions --project /path` to list relevant sessions.

---

### 15. .gitignore Awareness
**What:** `list_files` and `search` should respect the project's `.gitignore` instead of the hardcoded ignore list.

**Where:**
- `internal/tools/builtin/` — parse `.gitignore` in `list_files.go` and `search.go`

**Why:** Currently `dist/`, `build/` etc. are skipped via a hardcoded list. A project might gitignore `coverage/`, `tmp/`, or custom directories that VLA would still scan.

---

### 16. Concurrency in Indexer
**What:** Parse files in parallel during the initial build using a worker pool.

**Where:**
- `internal/indexer/indexer.go` — use `errgroup` or goroutine pool in `Build()`

**Why:** Sequential parsing is fine for small projects but slow for large ones (10K+ files). Parallel parsing with a 4-8 worker pool would cut build time significantly.

---

## Completed

- [x] Core agent loop (streaming, tool calls, compaction)
- [x] File tools (read, write, update, delete, list)
- [x] Git tools (status, diff, commit)
- [x] Search tool (ripgrep + native fallback)
- [x] Background indexer (regex-based, polling watcher)
- [x] Navigation tools (go-to-def, find-references, hover, diagnostics)
- [x] Web tools (search, read)
- [x] Persistent memory (embeddings, hybrid search, auto-injection)
- [x] LSP integration (gopls, pyright, warm pool, crash recovery)
- [x] MCP support (Model Context Protocol, .vla/mcp.json)
- [x] models.dev integration (150+ providers, `vla use`)
- [x] Full-screen TUI (bubbletea)
- [x] Readline fallback (history, Ctrl+C)
- [x] Session transcripts (NDJSON, persist, resume)
- [x] Path confinement (fsutil.Confine)
- [x] Max-turns protection
- [x] HTTP timeout on LLM client
- [x] Git command timeout
- [x] Signal handling (Ctrl+C clean shutdown)
- [x] System prompt (new sessions + resume)
- [x] Context-limit-aware compaction threshold
- [x] CI pipeline (GitHub Actions, golangci-lint, race detector)
- [x] Dependabot (grouped weekly PRs)
- [x] 262 tests (253 unit + 9 integration), all deterministic
