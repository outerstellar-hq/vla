# Hooks

VLA's hooks system lets you run custom scripts at specific points in the
agent loop — before a tool runs, after it completes, or when a file is
written. Hooks are defined in `.vla/hooks.json`.

## Events

| Event | When | Can Block? | Env Vars |
|-------|------|------------|----------|
| `before_tool` | Before any tool executes | Yes (exit non-zero = blocked) | `VLA_EVENT`, `VLA_TOOL` |
| `after_tool` | After a tool completes | No (non-blocking) | `VLA_EVENT`, `VLA_TOOL`, `VLA_RESULT` |
| `on_write` | After `write_file` or `update_file` | No | `VLA_EVENT`, `VLA_TOOL`, `VLA_FILE` |

## Blocking Semantics

- **`before_tool`**: If the script exits with a non-zero code, the tool call
  is blocked. The LLM sees "blocked by before_tool hook" as the tool result.
  Use this to enforce project policies (e.g., block commits to `main`).
- **`after_tool` / `on_write`**: Always run after the tool completes. Script
  errors are logged but don't block execution. Use these for notifications,
  logging, or triggering builds.

## Configuration

Create `.vla/hooks.json` in your project root:

```json
{
  "hooks": [
    {
      "event": "before_tool",
      "tool": "git_commit",
      "command": "scripts/pre-commit-check.sh"
    },
    {
      "event": "on_write",
      "tool": "",
      "command": "gofmt -w $VLA_FILE"
    },
    {
      "event": "after_tool",
      "tool": "write_file",
      "command": "echo 'File written: $VLA_FILE' >> .vla/vla.log"
    }
  ]
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `event` | Yes | One of: `before_tool`, `after_tool`, `on_write` |
| `tool` | No | Tool name to match (empty = matches all tools) |
| `command` | Yes | Shell command to execute |

## Environment Variables

The following variables are available to hook scripts:

| Variable | Description |
|----------|-------------|
| `VLA_EVENT` | The event type (`before_tool`, `after_tool`, `on_write`) |
| `VLA_TOOL` | The tool name (e.g. `write_file`, `read_file`) |
| `VLA_RESULT` | The tool's result string (for `after_tool` only) |
| `VLA_FILE` | The file path (for `on_write` only) |

## Examples

### Format Go files on write

```json
{
  "hooks": [
    {
      "event": "on_write",
      "tool": "",
      "command": "gofmt -w $VLA_FILE 2>/dev/null || true"
    }
  ]
}
```

### Block commits to main branch

```json
{
  "hooks": [
    {
      "event": "before_tool",
      "tool": "git_commit",
      "command": "test \"$(git branch --show-current)\" != \"main\""
    }
  ]
}
```

### Notify on file changes (macOS)

```json
{
  "hooks": [
    {
      "event": "on_write",
      "tool": "",
      "command": "osascript -e 'display notification \"VLA modified $VLA_FILE\" with title \"VLA\"'"
    }
  ]
}
```
