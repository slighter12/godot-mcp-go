# Development Guide (v1)

This document is the canonical implementation status for the repository.

## Document Boundary

| Document | Owns | Does Not Own |
| --- | --- | --- |
| `docs/DEVELOPMENT.md` | Cross-project delivery status, release readiness, verification gates | Prompt payload field-level schema details |
| `docs/PROMPT_CATALOG_COMPLETENESS_PLAN.md` | Prompt catalog runtime contract details and policy behavior | Non-prompt roadmap and packaging sequence |

## Status Snapshot

### Completed

- Project setup and module layout
- MCP initialization with strict protocol validation (`2025-11-25` only)
- Dual transport support (`stdio`, `streamable_http`)
- Session lifecycle gate (`initialize` -> `notifications/initialized` -> regular methods), protocol header validation, SSE transport lifecycle
- Tool manager and canonical `godot.*` name binding
- Layered server boundary:
  - `internal/protocol` for MCP frame/version validation
  - `internal/application/toolpipeline` for `tools/call` orchestration
  - `internal/domain/toolspec` for naming and permission policy
  - `internal/infra/notifications` for standard progress payloads
- Runtime bridge snapshot store with stale + grace freshness policy
- Runtime bridge internal tools (`godot.runtime.sync`, `godot.runtime.ping`, `godot.runtime.ack`)
- Runtime command broker with dispatch/ack/timeout observability metrics
- Runtime-backed read tools (`godot.editor.state.get`, `godot.node.tree.get`, `godot.node.properties.get`)
- Runtime mutating command bridge for project/scene/node/script tools
- Session-scoped mutating capability gate (`initialize.params.capabilities.godot.mutating=true`)
- Project tool completeness (`godot.project.settings.get`, `godot.project.resources.list`) with deterministic pagination
- Script create overwrite policy (`replace=false` default, conflict reason surfaced in runtime ack`)
- Prompt catalog strict rendering mode and advanced rendering mode
- Prompt catalog watch modes (`poll` + `event`)
- Prompt catalog governance tiers for advanced rendering (`restricted`, `trusted`)
- Policy metadata and runtime metrics resources (`godot://policy/godot-checks`, `godot://runtime/metrics`)
- Runtime health tool (`godot.runtime.health.get`)
- Tool controls (schema validation, unknown argument rejection, read-only/allow-list permission modes)
- Runtime bridge internal permission bypass (`godot.runtime.sync`, `godot.runtime.ping`, `godot.runtime.ack`)
- Plugin modularized entry/wiring:
  - `connection_state_machine.gd`
  - `streamable_http_client.gd`
  - `mcp_protocol_adapter.gd`
  - `runtime_snapshot_collector.gd`
  - `runtime_command_dispatcher.gd`
  - `tool_catalog.gd`
- CI and manual verification scripts (Go tests, HTTP smoke/ping/delete/session isolation, Inspector docker)

### Release State

- Roadmap tracks are closed for the current v1 repository line.
- No deferred implementation backlog is tracked in this file.

## Verification Gate

1. `go test ./...`
2. `make test-http-smoke`
3. `make test-http-ping`
4. `make test-http-delete`
5. `make test-http-session-isolation`
6. `make test-http-protocol-header`
7. `make test-http-allow-list-runtime-bridge`
8. `make test-lifecycle-initialized-id`
9. `make test-inspector-docker`
10. `make test-inspector-header-negative`

## Acceptance Failure Criteria

- Any JSON-RPC lifecycle deviation from `initialize -> notifications/initialized -> regular methods`
- Any transport mismatch between stdio and streamable HTTP for lifecycle error semantics
- Any permission regression where non-internal tools bypass `permission_mode`
- Any runtime bridge regression where `godot.runtime.sync`, `godot.runtime.ping`, or `godot.runtime.ack` is blocked by `read_only` / `allow_list`
- Any protocol version acceptance outside `2025-11-25`

## Update Rules

- Update this file when delivery status or verification gates change.
- Keep prompt contract payload details in `docs/PROMPT_CATALOG_COMPLETENESS_PLAN.md`.
- Keep user-facing migration steps in `docs/INSTALL_UPGRADE.md`.
