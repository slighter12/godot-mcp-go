# Development Guide (0.3.0)

This document is the canonical implementation status for the repository.

## Document Boundary

| Document | Owns | Does Not Own |
| --- | --- | --- |
| `docs/DEVELOPMENT.md` | Cross-project delivery status, release readiness, verification gates | Prompt payload field-level schema details |
| `docs/PROMPT_CATALOG_COMPLETENESS_PLAN.md` | Prompt catalog runtime contract details and policy behavior | Non-prompt roadmap and packaging sequence |

## Status Snapshot

### Completed

- Project setup and module layout
- MCP 2026-07-28 request metadata and strict version validation
- MCP 2026-07-28 resource templates, cache metadata, bounded complete results, production-configurable completion, and shared confidential MRTR requestState
- Dual transport support (`stdio`, `streamable_http`)
- Stateless HTTP routing with method/name/version and `x-mcp-header` parameter validation, request-scoped progress SSE, cancellation, and `subscriptions/listen`
- Tool manager and canonical `godot.*` name binding
- Layered server boundary:
  - `internal/protocol` for MCP frame/version validation
  - `internal/application/toolpipeline` for `tools/call` orchestration
  - `internal/domain/toolspec` for naming and permission policy
  - `internal/infra/notifications` for standard progress payloads
- Runtime bridge snapshot store with stale + grace freshness policy
- Runtime bridge internal tools (`godot.bridge.editor.sync`, `godot.bridge.editor.ping`, `godot.bridge.command.ack`)
- Runtime command broker with dispatch/ack/timeout observability metrics
- Editor-backed/runtime-backed read tools (`godot.editor.state.get`, `godot.runtime.scene_tree.get`, `godot.runtime.node_properties.get`)
- Runtime mutating command bridge for project/scene/node/script tools
- Per-request mutating capability gate (`_meta` Godot extension `mutating=true`)
- Split-session editor owner resolution for editor-backed/session-discovery tools (`godot.editor.state.get`, `godot.project.is_running`, `godot.runtime.session.get_active`, `godot.project.run`, `godot.project.stop`, `godot.editor.scene.apply`)
- Runtime run/attach resilience: `godot.project.run` preserves game session mapping when first snapshot await times out, allowing late runtime register recovery
- Runtime run attach token consistency: when attach remaps to an existing game session id, server preserves effective launch token (ack token first, existing token fallback) so runtime register validation remains stable
- Project tool completeness (`godot.project.settings.get`, `godot.project.resources.list`) with deterministic pagination
- Script create overwrite policy (`replace=false` default, conflict reason surfaced in runtime ack`)
- Prompt catalog strict rendering mode and advanced rendering mode
- Prompt catalog watch modes (`poll` + `event`)
- Prompt catalog governance tiers for advanced rendering (`restricted`, `trusted`)
- Policy metadata and runtime metrics resources (`godot://policy/godot-checks`, `godot://runtime/metrics`)
- Runtime health tool (`godot.runtime.health.get`)
- Tool controls (schema validation, unknown argument rejection, read-only/allow-list permission modes)
- Runtime bridge internal permission bypass (`godot.bridge.editor.sync`, `godot.bridge.editor.ping`, `godot.bridge.command.ack`)
- Plugin modularized entry/wiring:
  - `connection_state_machine.gd`
  - `streamable_http_client.gd`
  - `mcp_protocol_adapter.gd`
  - `runtime_snapshot_collector.gd`
  - `runtime_command_dispatcher.gd`
  - `tool_catalog.gd`
- Isolated `cmd/conformance-fixture` catalog for the pinned official MCP server runner; fixture names are never registered in production
- CI and manual verification scripts (Go tests, HTTP gates, Inspector Docker, frozen conformance requirements, addon static check)

### Release State

- Roadmap tracks are closed for the current v1 repository line.
- Runtime log backlog is tracked separately in `docs/RUNTIME_LOG_BACKLOG.md`.

## Verification Gate

Run the canonical aggregate gate:

```bash
make test-release
```

For a fast local check that does not require Docker or Bun, run `make test-quick`.

It runs `test-go` and `test-addon-static`.

The complete `make test-release` gate runs, in order:

1. `go test ./...`
2. `make test-http-smoke`
3. `make test-http-runtime-log-smoke`
4. `make test-http-ping`
5. `make test-http-delete`
6. `make test-http-session-isolation`
7. `make test-http-protocol-header`
8. `make test-http-allow-list-runtime-bridge`
9. `make test-lifecycle-initialized-id`
10. `make test-inspector-docker`
11. `make test-inspector-header-negative`
12. `make test-conformance-cleanup`
13. `make test-conformance-2026-07-28`
14. `make test-addon-static`

The conformance target starts the loopback-only fixture and invokes exactly `@modelcontextprotocol/conformance@0.2.0-alpha.11` with `--requirements 2026-07-28`. Its exit status scores only frozen core scenarios; extension and pending scenarios remain visible in the report. Default temporary reports are removed after success and copied to the printed `Conformance failure output retained` path after failure; set `CONFORMANCE_OUTPUT_DIR` to always retain them. Before changing the pinned package version, manually diff its frozen requirement and fixture scenario lists and record acceptance of additions in the change review.

## Acceptance Failure Criteria

- Any acceptance of the removed lifecycle or `MCP-Session-Id` transport
- Any transport mismatch between stdio and Streamable HTTP for request metadata/result semantics
- Any permission regression where non-internal tools bypass `permission_mode`
- Any runtime bridge regression where `godot.bridge.editor.sync`, `godot.bridge.editor.ping`, or `godot.bridge.command.ack` is blocked by `read_only` / `allow_list`
- Any protocol version acceptance outside `2026-07-28`
- Any scored failure in the frozen `2026-07-28` official server requirements
- Any `test_*` or `json_schema_2020_12_tool` fixture entry exposed by the production catalog

## Update Rules

- Update this file when delivery status or verification gates change.
- Keep prompt contract payload details in `docs/PROMPT_CATALOG_COMPLETENESS_PLAN.md`.
- Keep user-facing migration steps in `docs/INSTALL_UPGRADE.md`.
