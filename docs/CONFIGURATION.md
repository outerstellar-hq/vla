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
| `--persona <name\|path>` | System prompt persona: `architect` or path to a `.md` file |

## Personas

The `--persona` flag controls which system prompt VLA uses. This shapes
the LLM's behavior, priorities, and tone.

### Built-in personas

| Name | Description |
|------|-------------|
| *(default)* | Standard VLA: concise, tool-first, investigate-then-act |
| `architect` | Senior architect: anti-technical-debt, anti-bloat, holistic thinking, demands proven solutions over hand-rolled code |

```bash
# Use the architect persona
vla --persona architect

# Use a custom persona from a file
vla --persona ~/.vla/my-persona.md

# Project-level persona (auto-detected)
echo "You are a Rust expert..." > .vla/persona.md
vla
```

### Custom persona files

Any `.md` file can be used as a persona. The file's contents replace the
system prompt entirely — include tool instructions if needed, or keep it
purely behavioral and VLA will append the tool list automatically.

The architect persona is particularly useful for:
- Codebase audits and technical debt assessment
- Designing features that need to fit existing architecture
- Reviewing PRs or proposed changes
- Projects where quality and maintainability are non-negotiable

## Subcommands

VLA has several subcommands beyond the default agent mode:

| Command | Description |
|---------|-------------|
| `vla` | Start the agent (default — full-screen TUI or readline) |
| `vla use <provider/model>` | Configure provider via models.dev (writes config.json) |
| `vla models [provider] [filter]` | Browse available models from models.dev catalog |
| `vla sessions [--project <path>]` | List sessions, optionally filtered by project |
| `vla version` | Print the VLA version |
| `vla demo [--out=DIR] [--gif]` | Generate demo screenshots/GIF (used by CI) |
| `vla bug --title '...' --body '...'` | Report a bug (creates a GitHub issue) |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Fallback API key (if not in config.json) |
| `OPENAI_BASE_URL` | Fallback base URL |
| `GITHUB_TOKEN` | GitHub token for `vla bug` (if gh CLI not installed) |
| `GH_TOKEN` | Alternative GitHub token (same as GITHUB_TOKEN) |
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
