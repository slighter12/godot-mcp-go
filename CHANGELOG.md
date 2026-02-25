# Changelog

## [1.0.0] - 2026-02-25

### Added
- Canonical v1 `godot-*` tool naming across scene/node/script/project/utility surfaces.
- Runtime command ack metadata support: `schema_version`, `reason`, `retryable`.
- Runtime-backed mutating command pipelines for scene/node/script tools.
- CI workflow for Go tests, HTTP smoke checks, session isolation, and MCP Inspector docker checks.
- v1 contract and runtime protocol documents.

### Changed
- `tools/list` exposes canonical names.
- Tool dispatch now strictly requires canonical names (legacy aliases removed).
- Non-semantic tool execution failures now include `error.kind=execution_failed`.
- Godot plugin runtime bridge calls now use canonical runtime tool names.

### Security
- Added edited-scene subtree guardrails for node path resolution in plugin runtime handlers.
