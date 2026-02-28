# Development Guide (v1)

This document is the canonical implementation status for the repository.

## Document Boundary

| Document | Owns | Does Not Own |
| --- | --- | --- |
| `docs/DEVELOPMENT.md` | Cross-project delivery status, release readiness, verification gates | Prompt payload field-level schema details |
| `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md` | Prompt catalog runtime contract details and policy behavior | Non-prompt roadmap and packaging sequence |

## Status Snapshot

### Completed

- Project setup and module layout
- MCP initialization and protocol negotiation (`2025-11-25` target)
- Dual transport support (`stdio`, `streamable_http`)
- Session lifecycle, protocol header validation, SSE transport lifecycle
- Tool manager and canonical `godot-*` name binding
- Runtime bridge snapshot store with stale + grace freshness policy
- Runtime bridge internal tools (`godot-runtime-sync`, `godot-runtime-ping`, `godot-runtime-ack`)
- Runtime command broker with dispatch/ack/timeout observability metrics
- Runtime-backed read tools (`godot-editor-get-state`, `godot-node-get-tree`, `godot-node-get-properties`)
- Runtime mutating command bridge for project/scene/node/script tools
- Session-scoped mutating capability gate (`initialize.params.capabilities.godot.mutating=true`)
- Project tool completeness (`godot-project-get-settings`, `godot-project-list-resources`) with deterministic pagination
- Script create overwrite policy (`replace=false` default, semantic conflict reason)
- Prompt catalog strict rendering mode and advanced rendering mode
- Prompt catalog watch modes (`poll` + `event`) with automatic fallback behavior
- Prompt catalog governance tiers for advanced rendering (`restricted`, `trusted`)
- Policy metadata and runtime metrics resources (`godot://policy/godot-checks`, `godot://runtime/metrics`)
- Runtime health tool (`godot-runtime-get-health`)
- Tool controls (schema validation, unknown argument rejection, read-only/allow-list permission modes)
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
6. `make test-inspector-docker`

## Update Rules

- Update this file when delivery status or verification gates change.
- Keep prompt contract payload details in `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md`.
- Keep user-facing migration steps in `docs/INSTALL_UPGRADE.md`.
