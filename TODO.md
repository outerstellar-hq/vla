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

### 9. TUI Polish — DONE (core features; split-pane + session switcher deferred)
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
- [ ] Session switcher (deferred — needs session list TUI component)

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
- [x] 387 tests, all deterministic (+ 20 diff tests = 407 total)
