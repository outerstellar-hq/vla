# TUI (Terminal Interface)

When launched in an interactive terminal, VLA uses a full-screen bubbletea
interface. This document covers all key bindings, modes, and features.

## Key Bindings

### Input

| Key | Action |
|-----|--------|
| `Ctrl+Enter` / `Ctrl+J` | Submit input |
| `Ctrl+C` | Quit VLA |

### Navigation

| Key | Action |
|-----|--------|
| `Up` / `Down` | Scroll conversation history (or navigate autocomplete/picker) |
| `Page Up` / `Page Down` | Scroll by page |
| `Ctrl+F` | Toggle auto-scroll follow mode (pause/resume auto-scroll) |

### Tools

| Key | Action |
|-----|--------|
| `Tab` | Expand/collapse the last tool call block |
| `Ctrl+D` | Toggle split-pane diff preview |
| `Shift+Up` / `Shift+Down` | Scroll the diff pane independently |

### Sessions

| Key | Action |
|-----|--------|
| `Ctrl+S` | Open session switcher |
| `Enter` (in picker) | Switch to selected session |
| `Esc` | Close session picker / diff pane / autocomplete |

### Approval Prompts

When a destructive tool (`write_file`, `update_file`, `delete_file`,
`git_commit`) requires approval:

| Key | Action |
|-----|--------|
| `y` | Approve this tool call |
| `n` | Deny this tool call |
| `a` | Approve all remaining tool calls in this turn |

## Status Bar

The status bar at the top shows:

```
vla │ gpt-4o │ 24 tools │ ▌▌▌ thinking │ 1.2k tok │ ↓ following │ 20260704T
```

| Section | Description |
|---------|-------------|
| `vla` | App name |
| `gpt-4o` | Current model |
| `24 tools` | Number of registered tools |
| `▌▌▌ thinking` | Spinner + state (`thinking`, `running: read_file`, `idle`) |
| `1.2k tok` | Accumulated token count (from `/cost`) |
| `↓ following` | Scroll-follow indicator (`⏸ paused` when locked) |
| `20260704T` | Session ID (truncated) |

## Tool Call Blocks

Tool calls are rendered as compact blocks. Collapsed (default):

```
⚙ read_file (/src/main.go) ✓
```

Expanded (press `Tab`):

```
⚙ read_file (/src/main.go) ✓
  args:
    { "path": "/src/main.go" }
  result:
    package main
    
    func main() { ... }
```

Status icons:
- `✓` — completed successfully
- `⊘` — denied or blocked
- `⟳` — running (with spinner in status bar)

## Diff Preview

When the LLM calls `write_file` or `update_file`, the diff pane
automatically appears on the right side of the screen:

- `write_file`: all content shown as green (new)
- `update_file`: old lines in red (`-`), new lines in green (`+`)
- `Ctrl+D` toggles the pane on/off
- `Esc` hides it

## Session Switcher

Press `Ctrl+S` to open the session picker. It shows all sessions for the
current project, sorted by last active:

```
 Switch Session
 ↑↓ navigate · Enter select · Esc cancel

 20260704T120000Z  30m   gpt-4o
 20260704T090000Z  4h    gpt-4o
 20260703T160000Z  20h   claude-3.5-sonnet
 20260702T110000Z  2d    gpt-4o

 4 session(s)
```

Selecting a session tears down the current agent loop, loads the selected
session's history, and restarts — all without quitting bubbletea.

## Markdown Rendering

Assistant messages are rendered with [glamour](https://github.com/charmbracelet/glamour):
- Code blocks with syntax highlighting
- Headings, lists, tables, bold/italic
- Dark theme (auto-detected)

## Slash Commands

Type `/` to see the autocomplete menu:

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/tools` | List registered tools |
| `/model [name]` | Show current model (switching requires restart) |
| `/memory search <q>` | Search stored memories |
| `/memory save <text>` | Save a memory |
| `/memory list` | List all memories |
| `/compact` | Manually trigger context compaction |
| `/session` | Show session ID, model, tool count |
| `/cost` | Show token usage and estimated cost |
| `/clear` | Clear conversation (requires restart) |

## Plan Mode

Launch with `--plan` for read-only investigation:
- All file-modifying tools are blocked (`write_file`, `update_file`, `delete_file`, `git_commit`)
- The system prompt tells the LLM to produce a plan, not execute changes
- Review the plan, then re-run without `--plan` to execute

## Piped Input

When stdin is piped (`echo "fix bug" | vla`), VLA falls back to readline
mode — a simpler line-by-line interface without the full-screen TUI. All
slash commands and tools still work; only the rendering differs.
