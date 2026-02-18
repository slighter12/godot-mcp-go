# Development Guide

This file is the canonical roadmap for the whole repository.

## Document Boundary

| Document | Owns | Does Not Own |
| --- | --- | --- |
| `docs/DEVELOPMENT.md` | Cross-project milestones, sequencing, release readiness, testing gates | Prompt endpoint payload details, prompt catalog schema, prompt policy hook payloads |
| `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md` | Prompt catalog runtime contract and implementation phases | Godot runtime feature roadmap, release packaging scope, non-prompt architecture plans |

If content belongs to both docs, keep the high-level milestone here and keep protocol/runtime details in the prompt catalog plan.

## Status Snapshot

### Completed

- Project setup and module layout
- Core MCP server initialization
- Dual transport support (`stdio`, `streamable_http`)
- Session lifecycle and CORS controls
- Tool manager and category-based tool registration
- Config file load/validate/normalize flow
- Runtime bridge snapshot store with stale detection (`10s`) and session binding
- Internal runtime bridge tools (`sync-editor-runtime`, `ping-editor-runtime`, `ack-editor-command`)
- Runtime-backed read tools (`get-editor-state`, `get-scene-tree`, `get-node-properties`)
- File-backed scene/script reads with safe project-path validation (`res://` + relative only)
- Semantic tool error contract (`isError=true` + `error.kind`)
- Runtime command bridge for `run-project` / `stop-project`

### In Progress

- Expand quality coverage beyond functional tests
- Expand Godot write/edit execution beyond current read + run/stop bridge coverage

### Open Tracks

- Godot integration depth
- Advanced execution controls
- Release documentation and packaging

## Roadmap

### Track A: Runtime Stability

- [x] Base JSON-RPC envelope handling
- [x] Session negotiation and protocol version checks
- [x] Streamable HTTP policy documented: target protocol `2025-11-25` and explicit `MCP-Protocol-Version` usage
- [ ] Performance benchmarks under concurrent requests
- [ ] Concurrent connection stress tests

### Track B: Godot Integration

- [x] Runtime bridge domain model/store and stale-aware reads
- [x] Runtime sync lifecycle (change-driven full snapshot + heartbeat ping)
- [x] Runtime command bridge (`run-project`, `stop-project`)
- [x] Real-time scene tree and script state reads
- [x] Editor state synchronization lifecycle
- [x] Clear runtime availability signals for Godot-dependent calls
- [ ] Real Godot API-backed write/edit execution for scene/script/node tools

### Track C: Prompt Catalog Runtime

- [x] Prompt catalog discovery and registration
- [x] Prompt lookup (`prompts/list`, `prompts/get`) backed by runtime data
- [x] Prompt-to-tool routing with validated arguments
- [x] Prompt catalog compatibility tests (Inspector + HTTP smoke)
- [x] Prompt catalog path governance (`allowed_roots` with fallback to `paths`)
- [x] Polling-based prompt catalog auto-reload and list-changed notifications
- [x] Strict rendering mode (optional) with required/unknown argument validation

Reference implementation and contracts live in `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md` (including the deferred backlog).

### Track D: Advanced Controls

- [ ] Tool parameter schema validation completeness
- [ ] Tool progress and execution telemetry
- [ ] Permission model for tool operations
- [ ] Cache strategy for deterministic reads

### Track E: Release Readiness

- [ ] API reference docs
- [ ] Installation and upgrade guide
- [ ] End-to-end usage examples
- [ ] Changelog and release notes discipline

## Post-v1 Implementation Plan (Godot Wave 2)

1. Scene write pipeline
- Implement `create-scene`, `save-scene`, `apply-scene` through runtime command bridge.
- Return deterministic command acknowledgements (success/error/reason) instead of placeholder behavior.

2. Node write pipeline
- Implement `create-node`, `delete-node`, `modify-node` with validated request schema and whitelist field updates.
- Add node path resolution guarantees (exact path first, then deterministic fallback policy if enabled).

3. Script write pipeline
- Implement `create-script`, `modify-script` for `.gd` and `.rs` with path safety rules consistent with read tools.
- Add overwrite/conflict policy (explicit replace flag and clear rejection reasons).

4. Runtime freshness negotiation and resiliency
- Expose server-side stale threshold as runtime metadata and align plugin heartbeat automatically.
- Add jitter-tolerant guardrails so transient timer drift does not cause avoidable `runtime_snapshot_stale`.

5. Runtime bridge observability and safety
- Add structured metrics for command dispatch latency, ack timeout reason, and stale transitions.
- Finalize permission model for mutating tools (session-scoped capability gating).

6. Verification expansion
- Add integration tests covering read/write bridge flows, session loss, stale transitions, and reconnect behavior.
- Add manual verification script/checklist for end-to-end Godot editor scenarios.

## Execution Priority

1. Complete real Godot-backed behaviors for high-value tools.
2. Enforce policy checks and structured error semantics.
3. Close performance and concurrency test gaps.
4. Finish release documentation and packaging.

## Update Rules

- Update this file when roadmap status or priority changes.
- Update `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md` when prompt catalog contract or internal design changes.
- Keep duplicated details out of this file; link to the owning document instead.
