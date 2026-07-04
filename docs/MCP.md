# MCP (Model Context Protocol)

VLA supports the [Model Context Protocol](https://modelcontextprotocol.io)
for connecting external tools. MCP is the same protocol used by Claude Code,
Cursor, and OpenCode — any MCP server plugs in via `.vla/mcp.json`.

## How It Works

On launch, VLA:
1. Reads `.vla/mcp.json`
2. Starts each configured MCP server as a subprocess (communicating via
   JSON-RPC over stdin/stdout)
3. Performs the MCP handshake (`initialize` → `tools/list`)
4. Registers discovered tools alongside the built-in tools

MCP tools are prefixed with the server name to avoid collisions. For example,
a server named `github` that provides `create_issue` becomes `github__create_issue`.

## Configuration

Create `.vla/mcp.json` in your project root:

```json
{
  "servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "ghp_..."
      }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/allowed/path"],
      "env": {}
    },
    "sqlite": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sqlite", "--db-path", "project.db"],
      "env": {}
    }
  }
}
```

### Server Configuration

| Field | Required | Description |
|-------|----------|-------------|
| `command` | Yes | Executable to run (e.g. `npx`, `python`, `node`) |
| `args` | No | Arguments to pass to the command |
| `env` | No | Environment variables for the subprocess |

## Protocol Details

VLA's MCP client uses newline-delimited JSON-RPC 2.0 over the server's
stdin/stdout (the standard MCP transport). The lifecycle is:

1. **Initialize**: send `initialize` with capabilities → receive server capabilities
2. **Discover tools**: send `tools/list` → receive available tools
3. **Execute tools**: when the LLM calls an MCP tool, send `tools/call` → receive result
4. **Shutdown**: on VLA exit, send SIGTERM to all MCP servers

MCP tool schemas are converted to the OpenAI function-calling format so the
LLM can call them like any built-in tool.

## Popular MCP Servers

| Server | Command | Provides |
|--------|---------|----------|
| GitHub | `npx -y @modelcontextprotocol/server-github` | Issue/PR management |
| Filesystem | `npx -y @modelcontextprotocol/server-filesystem <path>` | Sandboxed file access |
| SQLite | `npx -y @modelcontextprotocol/server-sqlite --db-path <path>` | SQL queries |
| PostgreSQL | `npx -y @modelcontextprotocol/server-postgres <connstr>` | SQL queries |
| Brave Search | `npx -y @modelcontextprotocol/server-brave-search` | Web search |
| Google Drive | `npx -y @modelcontextprotocol/server-google-drive` | File access |

See the [MCP servers registry](https://github.com/modelcontextprotocol/servers)
for the full list.

## Security

- MCP servers run as subprocesses with the same permissions as VLA.
- MCP tool arguments are NOT path-confined by `fsutil.Confine` — the server
  handles its own security.
- Environment variables (including secrets like API tokens) are passed to
  servers via the `env` field. Don't commit `.vla/mcp.json` with secrets —
  use environment variable references instead.
- The `--sandbox` flag restricts filesystem access for the entire VLA process,
  including MCP servers.
