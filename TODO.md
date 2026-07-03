# VLA — TODO

Open work items, prioritized by impact. Each item includes what to build,
where it goes, and why it matters.

---

## P0 — Trust & Safety

### 1. Diff Approval System — DONE
- [x] Approver interface in agent loop (ToolApprover)
- [x] AlwaysApprover (--yes flag, piped input)
- [x] ReadlineApprover (interactive y/n/a prompt with preview)
- [x] Per-tool: only write_file, update_file, delete_file, git_commit require approval
- [x] Preview shows file path + content snippet / diff for the change

### 2. Permission System — DONE
- [x] .vla/permissions.json with allow/deny/ask rules per tool
- [x] Permission checker injected into agent loop (runs before approver)
- [x] Blocked tools return "blocked by permission rules" to LLM, never execute
- [x] Default action configurable (allow/ask/deny when no rule matches)

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

### 4. Better Compaction (token-aware) — DONE
- [x] Threshold expressed in tokens (~4 chars/token), not raw chars
- [x] Uses model's context limit from models.dev (75% = trigger point)
- [x] Oversized tool results in recent window truncated to MaxToolResultTokens
- [x] Truncation notice tells LLM to use read_file for full output
- [x] DefaultTokenThreshold fallback when context_limit unknown

### 5. Plan / Build Modes — DONE
- [x] `--plan` flag: read-only investigation, all write tools blocked
- [x] Plan mode system prompt: tells LLM to produce a numbered plan
- [x] Permission overrides applied at runtime (no config file edit needed)
- [x] User reviews plan, re-runs without --plan to execute

---

### 6. Multi-Language Support (JS, HTML, CSS, SCSS remaining)
**What:** Extend the indexer parser and LSP defaults to handle the remaining languages from the design doc. Kotlin, Java, C#, and PHP are DONE.

### 6. Multi-Language Support — DONE

All 9 languages from the design doc + your additions are implemented:

- [x] Python: regex parser (def, class, async def), LSP (pyright), inference (requirements.txt)
- [x] Go: regex parser (func, type, var, const), LSP (gopls), inference (go.mod)
- [x] Kotlin: regex parser (fun, class, val/var), LSP (kotlin-language-server), inference (build.gradle.kts)
- [x] Java: regex parser (class, interface, method), LSP (Eclipse JDT.LS), inference (pom.xml)
- [x] C#: regex parser (class, method, var), LSP (OmniSharp), inference (.csproj)
- [x] PHP: regex parser (function, class, $var, const), LSP (intelephense), inference (composer.json)
- [x] JavaScript/TypeScript: regex parser (function, class, const/arrow, interface, type), LSP (typescript-language-server), inference (package.json)
- [x] CSS/SCSS: regex parser (class selectors, ID selectors, @mixin/@include, $variables)
- [x] HTML: regex parser (id:*, class:*, prefixed to avoid collisions)

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

### 8. Cost Tracking — DONE
- [x] LLM client requests `stream_options.include_usage: true`
- [x] Parses `usage` from the final SSE chunk (prompt/completion/total tokens)
- [x] Accumulates across all API calls in the session (`Client.TotalUsage()`)
- [x] `/cost` slash command shows token usage + estimated cost

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

### 10. Slash Commands — DONE
- [x] `/help` — show all available commands
- [x] `/tools` — list registered tools
- [x] `/model [name]` — show current model or instructions to switch
- [x] `/memory search <query>` — search stored memories directly
- [x] `/memory save <text>` — save a memory directly
- [x] `/memory list` — list all memories
- [x] `/compact` — manually trigger context compaction
- [x] `/session` — show session ID, model, tool count
- [x] Unknown commands return helpful error

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
- [x] Background indexer (regex-based, polling watcher, 6 languages)
- [x] Navigation tools (go-to-def, find-references, hover, diagnostics)
- [x] Multi-language: Python, Go, Kotlin, Java, C#, PHP, JS/TS, CSS/SCSS, HTML (9 languages, parsers + LSP specs)
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
- [x] Diff approval system (human-in-the-loop before destructive tools)
- [x] Permission system (.vla/permissions.json, allow/deny/ask rules)
- [x] Slash commands (/help, /tools, /memory, /compact, /session)
- [x] 309 tests, all deterministic
