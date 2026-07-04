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

### 3. Sandbox (OS-level file access control) — DONE
- [x] Symlink-safe path confinement (`fsutil.Confine` now resolves symlinks via `EvalSymlinks`)
- [x] OS-level sandbox via `--sandbox` flag (re-exec pattern)
- [x] macOS: sandbox-exec (Seatbelt sandbox profile, restricts FS to project dir)
- [x] Linux: bwrap (bubblewrap user namespaces, read-only system mounts)
- [x] Windows: not supported (relies on hardened fsutil + user account restrictions)
- [x] Platform-specific build tags (`//go:build darwin/linux/windows`)
- [x] 15 fsutil tests (including symlink escape, dir escape, parent symlink, new file in symlinked dir)
- [x] 6 sandbox tests (mode detection, command construction, arg preservation)

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

### 7. Image / Multimodal Support — DONE
- [x] `agent.ContentPart` type for text and image_url content parts
- [x] `agent.Message.ContentParts` field — when set, LLM client sends content as array
- [x] LLM client serializes multimodal messages correctly (content array with text + image_url)
- [x] Plain text messages still send content as string (backward compatible)
- [x] `Message.HasImage()` helper to check if a message contains images

---

### 8. Cost Tracking — DONE
- [x] LLM client requests `stream_options.include_usage: true`
- [x] Parses `usage` from the final SSE chunk (prompt/completion/total tokens)
- [x] Accumulates across all API calls in the session (`Client.TotalUsage()`)
- [x] `/cost` slash command shows token usage + estimated cost

---

### 9. TUI Polish — DONE
- [x] Markdown rendering for assistant messages via glamour (code blocks, headings, lists, bold/italic, tables)
- [x] Tool call blocks with expand/collapse (Tab toggles, shows args + result; status icons ✓/⊘/⟳)
- [x] Auto-scroll lock with follow toggle (Ctrl+F; pauses on scroll-up, resumes on new message)
- [x] Live status bar: spinner + "thinking"/"running: tool" state + live token count (1.2k tok format)
- [x] Fixed `Init()` polling bug (channels were never started)
- [x] Fixed dead `toolCh`/`doneCh` channels (replaced by typed `Event` channel)
- [x] TUI-native approval system (fixes ReadlineApprover deadlock in alt-screen mode)
- [x] Slash command autocomplete (filtered menu on `/` prefix, arrow keys + Enter)
- [x] Typed event system: `agent.Event` channel (TurnStart/End, ToolStart/Result, Usage, ApprovalReq)
- [x] Block-based rendering model (replaces flat chatMsg with typed blocks)
- [x] Split-pane diff preview — LCS diff engine, auto-shows on write_file/update_file, Ctrl+D toggle, Shift+Up/Down scroll
- [x] Session switcher — Ctrl+S opens full-screen picker, filtered by current project, Up/Down/Enter to select, runner tears down and restarts loop on switch
- [x] Fixed resume-into-TUI bug (messages weren't loaded in TUI path)

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

### 11. Multi-Agent / Parallel Execution — DONE
- [x] `agent.Coordinator` spawns N sub-agents in parallel goroutines
- [x] Each sub-agent has independent message history, shared tool registry
- [x] Sub-agents execute tool calls autonomously (no approval prompts)
- [x] Results collected in order, formatted as combined summary
- [x] Error handling per sub-agent (failures don't block others)

### 12. Plugin System — DONE
- [x] `internal/plugins/` — script-based plugin system (cross-platform)
- [x] `.vla/plugins/<name>/plugin.json` manifest with tool schema
- [x] `.vla/plugins/<name>/run.sh` (or .py, .js, .cmd, .ps1) executable
- [x] Arguments passed as JSON on stdin, result returned from stdout
- [x] Auto-discovery: scans .vla/plugins/ for valid plugins on launch
- [x] Registered alongside built-in + MCP tools

---

### 13. Hooks — DONE
- [x] `internal/hooks/` — .vla/hooks.json with before_tool, after_tool, on_write, on_session_start events
- [x] before_tool hooks can block tool calls (exit non-zero = blocked)
- [x] after_tool/on_write hooks are non-blocking (run after, log errors)
- [x] Env vars passed: VLA_EVENT, VLA_TOOL, VLA_RESULT
- [x] Agent loop integration: HookRunner interface, runs in executeToolCall

---

### 14. Session Index — DONE
- [x] ~/.vla/sessions/index.json maps session IDs to project/model/timestamps
- [x] `vla sessions` subcommand lists all sessions
- [x] `vla sessions --project /path` filters by project
- [x] Sessions recorded automatically on launch/resume

---

### 15. .gitignore Awareness — DONE
- [x] `internal/gitignore/` — reads .gitignore, supports exact names, wildcards, dir patterns, negation
- [x] `list_files` skips gitignored directories and files
- [x] `search` (native fallback) skips gitignored directories and files
- [x] Combined with the hardcoded ignore list (defense in depth)

---

### 16. Concurrency in Indexer — DONE
- [x] Build() uses 4-goroutine worker pool for parallel file parsing
- [x] Phase 1 (definitions) and Phase 2 (references) both parallelized

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
- [x] 420 tests, all deterministic

---

## v0.3.0 — Competitive Features (vs Claude Code / ZCode)

### P0 — Core UX (users expect these immediately)

### 17. Esc to Cancel Stream
**What:** Press Esc during LLM streaming to interrupt the current response. The agent stops, keeps partial output, and returns control to the user.
**Where:** `internal/agent/loop.go` (cancellable context on StreamTo), `internal/tui/model.go` (Esc key binding)
**Why:** Every chat interface supports this. Without it, users wait for long responses they already know are wrong.

### 18. /undo — Rollback Last Changes
**What:** Undo the most recent file modifications made by the agent. Keeps a stack of (path, old_content) pairs before each write/update/delete. `/undo` restores the previous state.
**Where:** `internal/undo/` (new package — change stack), `internal/tools/builtin/` (record before mutation), `internal/commands/` (/undo slash command)
**Why:** Safety net for bad edits. Currently the only recourse is git stash/reset, which doesn't know which changes the agent made vs the user.

### 19. @file Autocomplete
**What:** Type `@` in the input to trigger file path autocomplete. Shows matching files from the project, arrow keys to select, Tab/Enter to insert the path.
**Where:** `internal/tui/model.go` (@-trigger autocomplete, like the existing `/` slash autocomplete)
**Why:** Users constantly reference files. Typing full paths is error-prone. Claude Code, ZCode, and Cursor all support this.

### 20. Context Window Visualization
**What:** A visual indicator in the status bar showing how much of the context window is used (e.g. a progress bar or percentage). Changes color as it fills (green → yellow → red).
**Where:** `internal/tui/model.go` (status bar rendering), `internal/agent/` (expose context usage)
**Why:** Users need to know when they're running low on context so they can `/compact` or start a new session before the agent loses early context.

### P1 — Important Features

### 21. Session-Wide Diff View
**What:** A `/diff` slash command that shows all file changes made during the current session as a unified git-style diff. Lets the user review everything before committing.
**Where:** `internal/commands/` (/diff command), `internal/tools/builtin/git.go` (reuse git diff logic)
**Why:** The split-pane diff shows one tool call at a time. Users need to see the complete picture of what the agent changed.

### 22. vla init — Project Scaffolding
**What:** `vla init` creates the `.vla/` directory, generates a `config.json` from the example, creates empty `permissions.json`/`hooks.json`, and runs `vla use` to pick a model.
**Where:** `init_cmd.go` (new — `vla init` subcommand)
**Why:** Onboarding friction. New users don't know what files to create. `vla init` sets everything up in one command.

### 23. Project-Level Steering Messages
**What:** A `.vla/steering.md` file whose contents are prepended to the system prompt for every session in that project. Persists across sessions — unlike `--persona` which is per-invocation.
**Where:** `internal/app/resume.go` (load steering.md into system prompt), `.vla/steering.md`
**Why:** ZCode has persistent steering messages. Users want project-specific instructions that survive restarts without passing `--persona` every time.

### 24. File Watcher Notifications
**What:** When the polling watcher detects file changes, notify the agent that a file it previously read has been modified. The agent can then re-read it before acting on stale data.
**Where:** `internal/indexer/watcher.go` (already detects changes), `internal/agent/loop.go` (invalidate cached reads)
**Why:** If the user edits a file in another editor while the agent is working, the agent may act on outdated file contents. This prevents that.

### P2 — Nice to Have

### 25. Image Paste (Multimodal Input)
**What:** In the TUI, support pasting an image from the clipboard (Ctrl+V) which gets added to the next message as an image_url content part.
**Where:** `internal/tui/model.go` (paste detection), `internal/agent/message.go` (ContentPart — already supports image_url)
**Why:** VLA's message types already support multimodal content but there's no way to actually send an image from the TUI.

### 26. Config Hot-Reload
**What:** Detect changes to `config.json` and `.vla/` files at runtime and apply them without restart.
**Where:** `internal/config/` (file watcher), `main.go` (reload hook)
**Why:** Users shouldn't need to quit and restart to change models or permissions.

### 27. Sub-Agent Dispatch from TUI
**What:** A `/spawn <task>` command that dispatches a sub-agent (via the Coordinator) to work on a task in parallel. The sub-agent's results appear in the conversation when done.
**Where:** `internal/agent/multi.go` (already exists), `internal/commands/` (/spawn command), `internal/tui/` (progress indicator)
**Why:** The Coordinator exists but isn't wired to the UI. Users can't leverage parallel agents from the TUI.
