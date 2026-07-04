# Configuration

VLA is configured through a combination of `config.json`, CLI flags, and
`.vla/` project-level files.

## config.json

The main configuration file. VLA searches for it in this order:

1. `--config <path>` flag (highest priority)
2. `./config.json` (current working directory)
3. `~/.vla/config.json` (global default)

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `api_key` | Yes | OpenAI-compatible API key (e.g. `sk-...`) |
| `base_url` | Yes | API base URL (e.g. `https://api.openai.com/v1`) |
| `model` | Yes | Model name (e.g. `gpt-4o`, `claude-3.5-sonnet`) |
| `context_limit` | No | Model's context window in tokens (for compaction). If omitted, uses a default threshold. |

### Example

```json
{
  "api_key": "sk-...",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o",
  "context_limit": 128000
}
```

Copy `config.json.example` to get started:

```bash
cp config.json.example config.json
# Edit with your API key and model
```

## models.dev Integration

VLA integrates with [models.dev](https://models.dev) for automatic provider
configuration. Instead of manually finding the right `base_url`, you can use:

```bash
# List all providers (150+)
vla models

# List models for a specific provider
vla models openai

# Filter by name
vla models anthropic claude

# Configure: writes config.json automatically
vla use openai/gpt-4o
```

`vla use <provider/model>` fetches the provider's `base_url`, writes it to
`config.json` along with the model name. You still need to set `api_key`
manually.

The models.dev catalog is cached at `~/.vla/models-cache.json` for 24 hours.

## CLI Flags

All flags are optional. VLA works with zero flags if `config.json` is present.

| Flag | Description |
|------|-------------|
| `--resume <id>` | Resume a session by ID (from `vla sessions`) |
| `--model <name>` | Override the config model for this run |
| `--config <path>` | Path to config.json |
| `--yes` | Auto-approve all tool calls (skip diff approval prompts) |
| `--plan` | Plan mode: read-only investigation, all write tools blocked |
| `--sandbox` | Run inside an OS-level sandbox (macOS: sandbox-exec, Linux: bwrap) |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Fallback API key (if not in config.json) |
| `OPENAI_BASE_URL` | Fallback base URL |
| `VLA_SANDBOXED` | Set to `1` by the sandbox re-exec (internal) |

## Project-Level Configuration

VLA reads project-specific configuration from `.vla/` in the project root:

```
.vla/
  mcp.json          MCP server configuration (see docs/MCP.md)
  permissions.json  Tool permission rules (see docs/PERMISSIONS.md)
  hooks.json        Event hook scripts (see docs/HOOKS.md)
  plugins/          Script-based plugins (see docs/PLUGINS.md)
```

## Global State

VLA stores global state in `~/.vla/`:

```
~/.vla/
  config.json         Global config (fallback when no local config.json)
  sessions/           Session transcripts
    <id>.json         NDJSON transcript for each session
    index.json        Cross-project session index
  memory/             Persistent memories (per project)
  models-cache.json   models.dev catalog cache (24h TTL)
```
