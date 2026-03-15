# Godot MCP Go Server

A Go implementation of an MCP server for Godot with `stdio` and Streamable HTTP transports, runtime bridge integration, and prompt catalog support.

## Features

- Dual transport support: `stdio` and `streamable_http`
- Canonical `godot.*` tool contract
- Session-scoped runtime bridge with stale + grace freshness policy
- Session mutating capability gate (`capabilities.godot.mutating=true`)
- Prompt catalog with `legacy`, `strict`, and `advanced` rendering modes
- Prompt source governance tiers (`restricted`, `trusted`)
- Prompt source watch modes (`poll`, `event`)
- Runtime observability:
  - tool: `godot.runtime.health.get`
  - resource: `godot://runtime/metrics`
- Tool controls: schema validation, unknown argument rejection, permission policy, progress notifications

## Prerequisites

- Go 1.26+
- Godot 4.x

## Installation

```bash
git clone https://github.com/slighter12/godot-mcp-go.git
cd godot-mcp-go
go mod tidy
go build
```

Link plugin into your Godot project:

```bash
mkdir -p /path/to/project/addons
ln -s /path/to/godot-mcp-go/godot-plugin /path/to/project/addons/godot_mcp
```

## Usage

### Streamable HTTP (default)

```bash
./godot-mcp-go
```

Endpoints:

- MCP: `http://localhost:9080/mcp`
- Info: `http://localhost:9080/`
- Required header: `MCP-Protocol-Version: 2025-11-25`

### Stdio

```bash
MCP_USE_STDIO=true ./godot-mcp-go
```

Initialize requests over stdio must include `params.protocolVersion="2025-11-25"`.

## Protocol and Progress Contract

- Supported protocol version is strict: `2025-11-25` only.
- Tool progress is emitted only as `notifications/progress`.
- Progress notifications require `tools/call` `_meta.progressToken`.

## Streamable HTTP Lifecycle

Streamable HTTP requests are strictly lifecycle-gated:

1. `initialize` must succeed.
2. Client must send `notifications/initialized`.
3. Only then regular methods are accepted (`tools/*`, `resources/*`, `prompts/*`, `ping`).

If `initialize` fails validation, the server does not create a usable new session and does not return `MCP-Session-Id`.
If `initialized` is sent before successful `initialize`, server returns JSON-RPC `invalid_request`.

## Mutating Capability Negotiation

Mutating tools are blocked by default. Clients must negotiate during `initialize`:

```json
{
  "jsonrpc": "2.0",
  "id": "init-1",
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-11-25",
    "capabilities": {
      "godot": {
        "mutating": true
      }
    },
    "clientInfo": {
      "name": "demo",
      "version": "0.1.0"
    }
  }
}
```

Without this capability, mutating calls return semantic `not_supported` with `reason=mutating_capability_required`.

## Configuration

Default config path resolution order:

1. `MCP_CONFIG_PATH`
2. `config/mcp_config.json`
3. `~/.godot-mcp/config/mcp_config.json`

Default config shape:

```json
{
  "name": "godot-mcp-go",
  "version": "0.1.0",
  "description": "Go-based Model Context Protocol server for Godot",
  "server": {
    "host": "localhost",
    "port": 9080,
    "debug": false
  },
  "transports": [
    { "type": "stdio", "enabled": true },
    {
      "type": "streamable_http",
      "enabled": true,
      "url": "http://localhost:9080/mcp",
      "headers": {
        "Accept": "application/json, text/event-stream",
        "Content-Type": "application/json",
        "MCP-Protocol-Version": "2025-11-25"
      }
    }
  ],
  "logging": {
    "level": "debug",
    "format": "json",
    "path": "logs/mcp.log"
  },
  "prompt_catalog": {
    "enabled": true,
    "paths": [],
    "allowed_roots": [],
    "watch": { "mode": "poll" },
    "governance": { "roots": [] },
    "auto_reload": {
      "enabled": false,
      "interval_seconds": 5
    },
    "rendering": {
      "mode": "legacy",
      "reject_unknown_arguments": false
    }
  },
  "tool_controls": {
    "schema_validation_enabled": true,
    "reject_unknown_arguments": false,
    "permission_mode": "allow_all",
    "allowed_tools": [],
    "emit_progress_notifications": true
  },
  "runtime_bridge": {
    "stale_after_seconds": 10,
    "stale_grace_ms": 1500
  }
}
```

### Environment Variables

- `MCP_USE_STDIO`
- `MCP_DEBUG`
- `MCP_CONFIG_PATH`
- `MCP_PORT`
- `MCP_HOST`
- `MCP_LOG_LEVEL`
- `MCP_LOG_PATH`
- `MCP_PROMPT_CATALOG_ENABLED`
- `MCP_PROMPT_CATALOG_PATHS`
- `MCP_PROMPT_CATALOG_ALLOWED_ROOTS`
- `MCP_PROMPT_CATALOG_WATCH_MODE` (`poll` or `event`)
- `MCP_PROMPT_CATALOG_GOVERNANCE_ROOTS` (CSV: `path:tier,path:tier`)
- `MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED`
- `MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS`
- `MCP_PROMPT_CATALOG_RENDERING_MODE` (`legacy`, `strict`, `advanced`)
- `MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS`
- `MCP_TOOL_CONTROLS_SCHEMA_VALIDATION_ENABLED`
- `MCP_TOOL_CONTROLS_REJECT_UNKNOWN_ARGUMENTS`
- `MCP_TOOL_CONTROLS_PERMISSION_MODE` (`allow_all`, `read_only`, `allow_list`)
- `MCP_TOOL_CONTROLS_ALLOWED_TOOLS`
- `MCP_TOOL_CONTROLS_EMIT_PROGRESS_NOTIFICATIONS`
- `MCP_RUNTIME_BRIDGE_STALE_AFTER_SECONDS`
- `MCP_RUNTIME_BRIDGE_STALE_GRACE_MS`

## Available Tools

### Scene

- `godot.scene.list`
- `godot.scene.read`
- `godot.scene.create`
- `godot.scene.save`
- `godot.scene.apply`

### Node

- `godot.node.tree.get`
- `godot.node.properties.get`
- `godot.node.create`
- `godot.node.delete`
- `godot.node.modify`

### Script

- `godot.script.list`
- `godot.script.read`
- `godot.script.create` (`replace` optional, default `false`)
- `godot.script.modify`
- `godot.script.analyze`

### Project

- `godot.project.settings.get` (paginated)
- `godot.project.resources.list` (paginated)
- `godot.editor.state.get`
- `godot.project.run`
- `godot.project.stop`

### Utility

- `godot.offerings.list`
- `godot.runtime.health.get`
- `godot.runtime.sync` (internal)
- `godot.runtime.ping` (internal)
- `godot.runtime.ack` (internal)
- `godot.prompts.reload`

Internal runtime bridge tools are exempt from `tool_controls.permission_mode` filtering:

- `godot.runtime.sync`
- `godot.runtime.ping`
- `godot.runtime.ack`

## Resources

- `godot://project/info`
- `godot://scene/current`
- `godot://script/current`
- `godot://policy/godot-checks`
- `godot://runtime/metrics`

## Development

### Test and Validation

```bash
go test ./...
make test-http-smoke
make test-http-ping
make test-http-delete
make test-http-session-isolation
make test-inspector-docker
```

### Project Structure

```text
godot-mcp-go/
├── config/
├── docs/
├── logger/
├── mcp/
├── promptcatalog/
├── runtimebridge/
├── tools/
├── transport/
├── godot-plugin/
├── skills/
└── main.go
```

## Skills Library

The `skills/` directory contains companion AI agent skills for
**consumers** of this MCP server. They are not runtime features of the
server itself, not part of the MCP tool contract, and not contributor
workflow docs for developing this repository.

These skills are external prompt/skill artifacts used by an agent that already supports skills. The relationship is:

- the skill decides the workflow
- the skill calls this server through the `godot.*` MCP tools
- the server does not load, expose, or execute the skills as built-in MCP functionality

These files are authored as agent-side skill artifacts (`SKILL.md` plus
optional references), not as npm packages. This repository is the current
source and distribution location for those skills, so Skills CLI consumers can
install from this same GitHub repository. See
[`docs/SKILLS_PUBLISHING.md`](docs/SKILLS_PUBLISHING.md) for the single-repo
publishing rules and future split guidance.

Current repository version: `0.1.0` (recommended first Git tag: `v0.1.0`).

Install examples:

```bash
npx skills add https://github.com/slighter12/godot-mcp-go --skill policy-godot
bunx skills add https://github.com/slighter12/godot-mcp-go --skill godot-game-dev-workflow
```

Pinned install examples:

```bash
npx skills add https://github.com/slighter12/godot-mcp-go/tree/v0.1.0/skills/policy-godot
bunx skills add https://github.com/slighter12/godot-mcp-go/tree/v0.1.0/skills/godot-game-dev-workflow
```

See [`docs/SKILLS_PUBLISHING.md`](docs/SKILLS_PUBLISHING.md) for the
versioning strategy and future split guidance.

Available skills:

- **`skills/policy-godot`** — Cross-project Godot 4 design and review policy for
  2D and early 3D architectural decisions. It covers scene ownership,
  physics/collision decisions, UI/input ownership, signals, resources,
  gameplay patterns, Rust/gdext boundaries, and engineering-quality
  review/debug guidance. Use it as the general design reference before
  choosing a repo-specific execution flow.
- **`skills/godot-game-dev-workflow`** — Vertical-slice workflow for Godot 4
  gameplay feature work, bug fixes, and refactors through this repository's
  `godot.*` MCP tools. It is the execution layer for this server, currently
  centered on common 2D-oriented gameplay slices and tool recipes. It depends
  on `skills/policy-godot` for design guidance. See
  [`SKILL.md`](skills/godot-game-dev-workflow/SKILL.md) for workflow usage.

## Limitations

- **The workflow skill currently targets 2D-oriented execution examples.** The
  current `godot-game-dev-workflow` references, playbooks, and recipes focus on
  common 2D gameplay tasks. This limitation applies to the workflow skill, not
  to the underlying `godot.*` MCP tools themselves. `skills/policy-godot`
  already provides broader cross-project architectural guidance, including
  early 3D-oriented design decisions, but its lane-specific examples are still
  less complete than the 2D workflow coverage.

## Documentation

- `docs/DEVELOPMENT.md`
- `docs/TOOL_CONTRACT.md`
- `docs/RUNTIME_COMMAND_PROTOCOL.md`
- `docs/PROMPT_CATALOG_COMPLETENESS_PLAN.md`
- `docs/INSTALL_UPGRADE.md`
- `docs/SKILLS_PUBLISHING.md`

## License

MIT. See `LICENSE`.
