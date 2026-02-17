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

### In Progress

- Expand quality coverage beyond functional tests
- Move placeholder Godot tool behaviors toward real editor/runtime integration

### Open Tracks

- Godot integration depth
- Prompt catalog runtime
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

- [ ] Real Godot API-backed tool execution
- [ ] Real-time scene tree and script state reads
- [ ] Editor state synchronization lifecycle
- [ ] Clear runtime availability signals for Godot-dependent calls

### Track C: Prompt Catalog Runtime

- [x] Prompt catalog discovery and registration
- [x] Prompt lookup (`prompts/list`, `prompts/get`) backed by runtime data
- [x] Prompt-to-tool routing with validated arguments
- [x] Prompt catalog compatibility tests (Inspector + HTTP smoke)

Reference implementation and contracts live in `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md`.

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

## Execution Priority

1. Complete real Godot-backed behaviors for high-value tools.
2. Complete prompt catalog endpoint correctness and routing.
3. Enforce policy checks and structured error semantics.
4. Close performance and concurrency test gaps.
5. Finish release documentation and packaging.

## Update Rules

- Update this file when roadmap status or priority changes.
- Update `docs/PROMPT_CATALOG_COMPLETENESS_PLAN_V1.md` when prompt catalog contract or internal design changes.
- Keep duplicated details out of this file; link to the owning document instead.
