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
ln -s /path/to/godot-mcp-go/godot-plugin/addons/godot_mcp /path/to/project/addons/godot_mcp
```

Then open Godot and enable `Godot MCP` in `Project > Project Settings > Plugins`. The runtime companion autoload (`GodotMCPRuntimeCompanion`) is registered automatically.

## Usage

### Streamable HTTP (default)

```bash
./godot-mcp-go
```

Or use the repo helper:

```bash
make run-http
```

Endpoints:

- MCP: `http://localhost:9080/mcp`
- Info: `http://localhost:9080/`
- Required header: `MCP-Protocol-Version: 2025-11-25`

### Codex/Desktop Session Attach

When Codex is configured with a URL-based MCP server such as `http://localhost:9080/mcp`, the HTTP server must already be listening before a new Codex session starts.

Recommended sequence:

1. Start the HTTP server with `make run-http` (or an equivalent foreground/background process manager).
2. Open a new Codex session after `http://localhost:9080/` is reachable.
3. If the server was down when the session started, create a fresh session after the server is back up. Existing sessions do not hot-attach `godot.*` tools mid-lifecycle.

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
      "version": "0.2.0"
    }
  }
}
```

Without this capability, mutating calls return semantic `not_supported` with `reason=mutating_capability_required`.

For clients that cannot send custom Godot capabilities during `initialize` (for example some Codex/Desktop URL-based sessions), the server can opt into a compatibility fallback:

```json
{
  "tool_controls": {
    "allow_mutating_without_capability": true
  }
}
```

Keep this disabled unless you trust every MCP client that can reach the server.

## Runtime Session Model

Two MCP sessions can exist at the same time by design:

- Godot IDE/plugin session (editor snapshot owner)
- AI/agent caller session

Scope rules:

- Caller-session scoped:
  - lifecycle gate (`initialize` -> `notifications/initialized`)
  - mutating capability negotiation (`initialize.params.capabilities.godot.mutating=true`)
- Editor-owner sourced:
  - `godot.editor.state.get`
  - `godot.project.is_running`
  - `godot.runtime.session.get_active`
  - editor command routing for `godot.project.run`, `godot.project.stop`, `godot.editor.scene.apply`
  - resolution order is:
    1. optional `editor_session_id` argument
    2. caller session when caller has a fresh editor snapshot
    3. latest fresh editor snapshot session
  - if no healthy editor snapshot exists, tools return semantic `not_available` with runtime snapshot reason.
  - `godot.project.run` uses attach/recover behavior when editor is already playing: it refreshes handshake metadata and reuses the running game session path instead of failing immediately.
  - if `godot.project.run` times out waiting for first runtime snapshot, the game session mapping is kept for late `godot.bridge.runtime.register` recovery instead of being deleted immediately.
- Runtime game session scoped:
  - runtime tools still require explicit game `session_id` unless they are lifecycle/session discovery calls.
  - runtime snapshots, logs, inputs, screenshots, and on-demand property reads stay bound to that game session.

The `runtime_bridge.allow_latest_session_fallback` config remains in the file format for compatibility, but public runtime tools no longer borrow the latest session implicitly.

For AI sessions, mutating tools still require caller-side capability negotiation. If a client cannot send `capabilities.godot.mutating=true`, use `tool_controls.allow_mutating_without_capability=true` only for trusted clients.

## Configuration

Default config path resolution order:

1. `MCP_CONFIG_PATH`
2. `config/mcp_config.json`
3. `~/.godot-mcp/config/mcp_config.json`

Default config shape:

```json
{
  "name": "godot-mcp-go",
  "version": "0.2.0",
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
    "emit_progress_notifications": true,
    "allow_mutating_without_capability": false
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
- `MCP_TOOL_CONTROLS_ALLOW_MUTATING_WITHOUT_CAPABILITY`
- `MCP_RUNTIME_BRIDGE_STALE_AFTER_SECONDS`
- `MCP_RUNTIME_BRIDGE_STALE_GRACE_MS`
- `MCP_RUNTIME_BRIDGE_ALLOW_LATEST_SESSION_FALLBACK`

## Available Tools

### Scene

- `godot.scene.list`
- `godot.scene.read`
- `godot.scene.create`
- `godot.scene.save`
- `godot.editor.scene.apply`

### Node

- `godot.runtime.scene_tree.get`
- `godot.runtime.node_properties.get`
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
- `godot.project.is_running`
- `godot.project.run`
- `godot.project.stop`

### Runtime

- `godot.runtime.session.get_active`
- `godot.runtime.sync_now`
- `godot.runtime.await_snapshot`
- `godot.runtime.input.tap`
- `godot.runtime.input.press`
- `godot.runtime.input.release`
- `godot.runtime.log.get`
- `godot.runtime.log.clear`
- `godot.runtime.screenshot.get`

Runtime log note:

- `godot.runtime.log.get(level="error")` is the current runtime diagnostics stream for the active game session
- `godot.runtime.log.get` supports `level`, `limit`, and `since_sequence`
- current diagnostics sources are `runtime_companion`, `runtime_lifecycle`, and `runtime_command:<tool_name>`
- full Godot-native parse/runtime error coverage is still tracked as deferred backlog in `docs/RUNTIME_LOG_BACKLOG.md`

### Utility / Internal

- `godot.offerings.list`
- `godot.runtime.health.get`
- `godot.runtime.diagnose`
- `godot.bridge.editor.sync` (internal)
- `godot.bridge.editor.ping` (internal)
- `godot.bridge.runtime.register` (internal)
- `godot.bridge.runtime.snapshot.push` (internal)
- `godot.bridge.runtime.log.push` (internal)
- `godot.bridge.command.ack` (internal)
- `godot.prompts.reload`

Internal runtime bridge tools are exempt from `tool_controls.permission_mode` filtering:

- `godot.bridge.editor.sync`
- `godot.bridge.editor.ping`
- `godot.bridge.runtime.register`
- `godot.bridge.runtime.snapshot.push`
- `godot.bridge.runtime.log.push`
- `godot.bridge.command.ack`

## Tool Dependency Categories

Each tool's description includes a category prefix indicating its runtime dependency:

- **`[file-based]`** — Works without any Godot plugin connected. Reads files directly from the project directory.
- **`[editor-plugin]`** — Requires the Godot MCP editor plugin to be connected with a fresh editor snapshot.
- **`[runtime]`** — Requires a running game session with the runtime companion registered.

Call `godot.offerings.list` to get a coarse global view of live component status before probing editor-backed or runtime-backed tools. The `status.tool_availability` block is not a task-scoped guarantee for any specific caller, editor, or game session:

```json
{
  "tool_availability": {
    "file_based": "available",
    "editor_backed": "available",
    "runtime_backed": "unavailable"
  }
}
```

All tools also include MCP `annotations` with `readOnlyHint`, `destructiveHint`, and `idempotentHint` fields.

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
make test-http-runtime-log-smoke
make test-http-ping
make test-http-delete
make test-http-session-isolation
make test-inspector-docker
```

`make test-http-runtime-log-smoke` is a focused runtime diagnostics smoke check. It validates that `godot.runtime.log.get` and `godot.runtime.log.clear` are exposed over Streamable HTTP and return the expected `game_session_missing` semantic error when no runtime session exists.

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

The `skills/` directory primarily contains companion AI agent skills for
**consumers** of this MCP server. They are not part of the `godot.*` MCP tool
contract and they are not contributor workflow docs for developing this
repository.

These skills are agent-side workflow artifacts used by an agent that already supports skills. The default relationship is:

- the skill decides the workflow
- the skill calls this server through the `godot.*` MCP tools
- default behavior: the server does not load `skills/` as built-in prompt catalog content
- exception: prompt catalog can expose those files only when you explicitly point `prompt_catalog.paths` at them

These files are authored as agent-side skill artifacts (`SKILL.md` plus
optional references), not as npm packages. This repository is the current
source and distribution location for those skills, so Skills CLI consumers can
install from this same GitHub repository. See
[`docs/SKILLS_PUBLISHING.md`](docs/SKILLS_PUBLISHING.md) for the single-repo
publishing rules and future split guidance.

Current repository version: `0.2.0` (recommended first Git tag: `v0.2.0`).

Install examples:

```bash
npx skills add https://github.com/slighter12/godot-mcp-go --skill policy-godot
bunx skills add https://github.com/slighter12/godot-mcp-go --skill godot-game-dev-workflow
```

Pinned install examples:

```bash
npx skills add https://github.com/slighter12/godot-mcp-go/tree/v0.2.0/skills/policy-godot
bunx skills add https://github.com/slighter12/godot-mcp-go/tree/v0.2.0/skills/godot-game-dev-workflow
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

Typical conservative live-state flow for the workflow skill:

- use `godot.offerings.list` only as a coarse global signal
- choose the intended `editor_session_id` for the task
- use `godot.editor.state.get` only for editor-backed context, preferably with that explicit `editor_session_id`
- call `godot.runtime.session.get_active` with that explicit `editor_session_id`
- fail closed unless the returned `editor_session_id` still matches the intended editor owner
- call `godot.runtime.await_snapshot` when runtime freshness matters
- pass only that verified `session_id` to later runtime reads and verification tools

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
