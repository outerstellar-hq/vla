# Plugins

VLA's plugin system lets you extend the agent with custom tools written in
any scripting language. Plugins live in `.vla/plugins/` and are discovered
automatically on launch.

## How It Works

Each plugin is a directory under `.vla/plugins/` containing:
1. A `plugin.json` manifest describing the tool
2. An executable script that receives JSON on stdin and returns a string on stdout

When VLA starts, it scans `.vla/plugins/` for valid plugins and registers
them alongside the built-in tools. The LLM can call them like any other tool.

## Plugin Structure

```
.vla/
  plugins/
    my-linter/
      plugin.json    # manifest (required)
      run.sh         # executable (choose one: .sh, .py, .js, .cmd, .ps1)
    git-stats/
      plugin.json
      run.py
```

## plugin.json Manifest

```json
{
  "name": "my_linter",
  "description": "Run the project linter and return issues",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "File or directory to lint"
      }
    },
    "required": ["path"]
  }
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique tool name (the LLM sees this). Use snake_case. |
| `description` | Yes | What the tool does (the LLM uses this to decide when to call it) |
| `parameters` | Yes | JSON Schema for the tool's parameters (OpenAI function-calling format) |

## Executable Script

The script receives the tool arguments as a JSON object on stdin, and must
write the result as a string to stdout. The result is fed back to the LLM.

### Script Selection

VLA looks for the executable in this order (first match wins):
- `run.sh` (Unix)
- `run.cmd` or `run.ps1` (Windows)
- `run.py` (if Python is available)
- `run.js` (if Node is available)

### Bash Example (`run.sh`)

```bash
#!/bin/bash
# Read JSON args from stdin
ARGS=$(cat)

# Parse the "path" field (requires jq)
PATH=$(echo "$ARGS" | jq -r '.path')

# Run the linter
RESULT=$(golangci-lint run "$PATH" 2>&1)

# Output the result
echo "$RESULT"
```

### Python Example (`run.py`)

```python
#!/usr/bin/env python3
import json
import subprocess
import sys

args = json.load(sys.stdin)
path = args.get("path", ".")

result = subprocess.run(
    ["golangci-lint", "run", path],
    capture_output=True,
    text=True
)

print(result.stdout or result.stderr or "No issues found")
```

## Security Considerations

- Plugin scripts run with the same permissions as the VLA process.
- The `--sandbox` flag restricts filesystem access for the entire process
  (including plugins).
- Plugin arguments are NOT path-confined by `fsutil.Confine` — the plugin
  script receives raw LLM-generated JSON. Validate inputs in your script.
- Error output (stderr) is logged but not shown to the LLM. Only stdout is
  captured as the tool result.

## Real-World Examples

### Database Query Tool

```json
{
  "name": "db_query",
  "description": "Run a read-only SQL query against the project database",
  "parameters": {
    "type": "object",
    "properties": {
      "query": { "type": "string", "description": "SQL SELECT query" }
    },
    "required": ["query"]
  }
}
```

```bash
#!/bin/bash
# .vla/plugins/db-query/run.sh
QUERY=$(cat | jq -r '.query')
# Reject non-SELECT queries
echo "$QUERY" | grep -qiE "^\\s*SELECT" || { echo "Error: only SELECT queries allowed"; exit 1; }
sqlite3 -json project.db "$QUERY"
```

### Test Runner

```json
{
  "name": "run_tests",
  "description": "Run the test suite and return results",
  "parameters": {
    "type": "object",
    "properties": {
      "package": { "type": "string", "description": "Go package to test (default: ./...)" }
    }
  }
}
```
