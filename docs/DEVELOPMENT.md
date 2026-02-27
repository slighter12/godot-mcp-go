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
- Internal runtime bridge tools (`godot-runtime-sync`, `godot-runtime-ping`, `godot-runtime-ack`)
- Runtime-backed read tools (`godot-editor-get-state`, `godot-node-get-tree`, `godot-node-get-properties`)
- File-backed scene/script reads with safe project-path validation (`res://` + relative only)
- Semantic tool error contract (`isError=true` + `error.kind`)
- Runtime command bridge for project and scene/node/script mutating tools
- Canonical v1 `godot-*` tool naming with strict binding
- CI workflow for Go tests + HTTP smoke + inspector docker checks

### In Progress

- Post-v1 Godot integration depth planning

### Open Tracks

- Godot integration depth
- Advanced execution controls
- Release documentation and packaging

## Roadmap

### Track A: Runtime Stability

- [x] Base JSON-RPC envelope handling
- [x] Session negotiation and protocol version checks
- [x] Streamable HTTP policy documented: target protocol `2025-11-25` and explicit `MCP-Protocol-Version` usage
- [x] Performance benchmarks under concurrent requests
- [x] Concurrent connection stress tests

### Track B: Godot Integration

- [x] Runtime bridge domain model/store and stale-aware reads
- [x] Runtime sync lifecycle (change-driven full snapshot + heartbeat ping)
- [x] Runtime command bridge (`godot-project-run`, `godot-project-stop`)
- [x] Real-time scene tree and script state reads
- [x] Editor state synchronization lifecycle
- [x] Clear runtime availability signals for Godot-dependent calls
- [x] Real Godot API-backed write/edit execution for scene/script/node tools

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

- [x] Tool parameter schema validation completeness
- [x] Tool progress and execution telemetry
- [x] Permission model for tool operations
- [x] Cache strategy for deterministic reads
- [x] Consolidate repeated GDScript runtime handler argument validation into shared helper utilities.
- [x] Consolidate duplicated runtime command dispatch/validation helpers across `tools/node`, `tools/scene`, and `tools/script` into a shared package.

### Track E: Release Readiness

- [x] API reference docs
- [x] Installation and upgrade guide
- [x] End-to-end usage examples
- [x] Changelog and release notes discipline

## Post-v1 Implementation Plan (Godot Wave 2)

1. Scene write pipeline

- Implement `godot-scene-create`, `godot-scene-save`, `godot-scene-apply` through runtime command bridge.
- Return deterministic command acknowledgements (success/error/reason) instead of placeholder behavior.

1. Node write pipeline

- Implement `godot-node-create`, `godot-node-delete`, `godot-node-modify` with validated request schema and whitelist field updates.
- Add node path resolution guarantees (exact path first, then deterministic fallback policy if enabled).

1. Script write pipeline

- Implement `godot-script-create`, `godot-script-modify` for `.gd` and `.rs` with path safety rules consistent with read tools.
- Add overwrite/conflict policy (explicit replace flag and clear rejection reasons).

1. Runtime freshness negotiation and resiliency

- Expose server-side stale threshold as runtime metadata and align plugin heartbeat automatically.
- Add jitter-tolerant guardrails so transient timer drift does not cause avoidable `runtime_snapshot_stale`.

1. Runtime bridge observability and safety

- Add structured metrics for command dispatch latency, ack timeout reason, and stale transitions.
- Finalize permission model for mutating tools (session-scoped capability gating).

1. Verification expansion

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
