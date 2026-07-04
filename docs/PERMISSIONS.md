# Permissions

VLA's permission system lets you control which tools the LLM can use on a
per-project basis. Rules are defined in `.vla/permissions.json`.

## How It Works

Before any tool executes, the permission checker runs:

1. If a tool has an **allow** rule → it runs immediately (no approval prompt).
2. If a tool has a **deny** rule → it never runs (the LLM sees "blocked by permission rules").
3. If a tool has an **ask** rule → the diff approval prompt is shown (if not `--yes`).
4. If no rule matches → the `default` action applies.

The permission check runs *before* the approval check, so denied tools are
blocked entirely without prompting the user.

## Configuration

Create `.vla/permissions.json` in your project root:

```json
{
  "default": "ask",
  "rules": {
    "write_file": "allow",
    "update_file": "allow",
    "delete_file": "deny",
    "git_commit": "ask",
    "web_search": "allow",
    "web_read": "allow"
  }
}
```

### Actions

| Action | Behavior |
|--------|----------|
| `allow` | Tool runs without asking |
| `deny`  | Tool never runs; LLM sees a "blocked" message |
| `ask`   | Tool triggers the diff approval prompt (destructive tools only) |

### Default Action

The `default` field controls what happens when no rule matches a tool. If
omitted, the default is `ask`. Common patterns:

- **Trusting**: `"default": "allow"` — allow everything, deny specific tools
- **Cautious**: `"default": "ask"` — ask before destructive tools (default)
- **Locked down**: `"default": "deny"` — deny everything, allow specific tools

## Interaction with --yes and --plan

- `--yes` flag: overrides all permission rules — all tools auto-approve.
- `--plan` flag: adds deny rules for `write_file`, `update_file`, `delete_file`,
  and `git_commit` (overrides permissions.json).

## Plan Mode Example

Run VLA in read-only mode to investigate a bug without risk of changes:

```bash
./vla --plan
```

This is equivalent to:
```json
{
  "default": "allow",
  "rules": {
    "write_file": "deny",
    "update_file": "deny",
    "delete_file": "deny",
    "git_commit": "deny"
  }
}
```
