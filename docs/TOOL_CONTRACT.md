# v1 Tool Contract

This document defines the public tool contract for `godot-mcp-go`.

## Tool Dependency Categories

Tools are categorized by their runtime dependency. Each tool's description includes a prefix:

| Category | Prefix | Dependency |
|----------|--------|------------|
| File-based | `[file-based]` | None — reads project files directly |
| Editor-backed | `[editor-plugin]` | Godot MCP editor plugin with fresh snapshot |
| Runtime-backed | `[runtime]` | Running game session + registered runtime companion |
| Utility | (none) | Server-only diagnostics |
| Internal bridge | (none) | Plugin-to-server communication (not called by AI) |

### Tool Annotations

All tools include MCP `annotations` in the `tools/list` response:
- `readOnlyHint` — `true` for read operations, `false` for mutations
- `destructiveHint` — `true` for `godot.node.delete` and `godot.runtime.log.clear`
- `idempotentHint` — `true` for read/query operations

### Live Status Discovery

Call `godot.offerings.list` to get live component status. The `status` block reports:
- `editor_plugin.connected` — whether the editor plugin has a fresh snapshot
- `runtime_companion.connected` — whether the runtime companion is registered
- `tool_availability` — summary of which tool categories are currently usable (`"available"` or `"unavailable"`)

## Naming

Canonical tool names:

- Pattern: `^[A-Za-z0-9._-]{1,128}$`
- Shape: `godot.<domain>.<action>` or `godot.<domain>.<object>.<action>`

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
- `godot.script.create`
- `godot.script.modify`
- `godot.script.analyze`

### Project

- `godot.project.settings.get`
- `godot.project.resources.list`
- `godot.editor.state.get`
- `godot.project.is_running`
- `godot.project.run`
- `godot.project.stop`

### Runtime

- `godot.runtime.session.get_active`
- `godot.runtime.sync_now`
- `godot.runtime.await_snapshot`
- `godot.runtime.scene_tree.get`
- `godot.runtime.node_properties.get`
- `godot.runtime.input.tap`
- `godot.runtime.input.press`
- `godot.runtime.input.release`
- `godot.runtime.log.get`
- `godot.runtime.log.clear`
- `godot.runtime.screenshot.get`

### Utility

- `godot.offerings.list`
- `godot.runtime.health.get`
- `godot.bridge.editor.sync` (internal bridge)
- `godot.bridge.editor.ping` (internal bridge)
- `godot.bridge.runtime.register` (internal bridge)
- `godot.bridge.runtime.snapshot.push` (internal bridge)
- `godot.bridge.runtime.log.push` (internal bridge)
- `godot.bridge.command.ack` (internal bridge)
- `godot.prompts.reload`

## Name Binding Policy

Canonical names are strictly required. Legacy aliases are rejected with `tool not found`.

## Session Lifecycle Gate

For both transports, tool execution requires lifecycle completion:

1. `initialize` succeeds with `protocolVersion=2025-11-25`
2. `initialized` or `notifications/initialized` is received
3. regular methods are accepted

Before lifecycle completion, regular requests return JSON-RPC `invalid_request` with message `Session is not initialized`.
If `initialized`/`notifications/initialized` is sent before successful `initialize`, server returns JSON-RPC `invalid_request`.

## Mutating Capability Gate

Mutating tools require session-scoped capability negotiation:

- Client must send `initialize.params.capabilities.godot.mutating=true`
- Without this capability, mutating tools return semantic error:
  - `isError=true`
  - `error.kind=not_supported`
  - `error.reason=mutating_capability_required`

Mutating tools covered by this gate:

- `godot.project.run`, `godot.project.stop`
- `godot.runtime.sync_now`, `godot.runtime.input.tap`, `godot.runtime.input.press`, `godot.runtime.input.release`, `godot.runtime.log.clear`
- `godot.scene.create`, `godot.scene.save`, `godot.editor.scene.apply`
- `godot.node.create`, `godot.node.delete`, `godot.node.modify`
- `godot.script.create`, `godot.script.modify`

## Session Resolution Model

Dual MCP sessions are expected:

- Godot IDE/plugin session (editor snapshot owner)
- AI/agent caller session

Scope rules:

- Caller session scoped:
  - lifecycle gate
  - mutating capability negotiation
- Editor-owner sourced:
  - `godot.editor.state.get`
  - `godot.project.is_running`
  - `godot.runtime.session.get_active`
  - editor command routing for `godot.project.run`, `godot.project.stop`, `godot.editor.scene.apply`
  - resolution order:
    1. optional `editor_session_id`
    2. caller session (when caller snapshot is fresh)
    3. latest fresh editor snapshot session
  - when no healthy editor snapshot exists, tools return semantic `not_available` with runtime snapshot reason.
- Runtime game session scoped:
  - runtime tools still bind to explicit game `session_id` (except lifecycle/session discovery).

## Transport Support Matrix

- `streamable_http`
  - Supports all read and mutating tools.
  - Mutating tools require initialized caller session + caller mutating capability + active runtime bridge.
- `stdio`
  - Supports non-runtime and read-oriented operations.
  - Runtime mutating command bridge paths are unavailable.

## Error Semantics

Tool result errors use semantic kinds in `result.error.kind`:

- `invalid_params`
- `not_supported`
- `not_available`
- `execution_failed`

## Mutating Tool Result Envelope

Mutating tool responses include:

- `success`
- `command_id`
- `result`
- `error`
- `acknowledged_at`

Optional metadata fields when provided by runtime ack:

- `schema_version`
- `reason`
- `retryable`

## Project Tool Contracts

### `godot.editor.scene.apply`

Input:

- required `path`
- optional `editor_session_id`

### `godot.project.settings.get`

Input:

- optional `cursor`
- optional `section_prefix`

Output:

- `settings`: array of `{key, section, value, raw}`
- optional `nextCursor`

### `godot.project.resources.list`

Input:

- optional `cursor`
- optional `extensions` (array)
- optional `include_hidden` (boolean)

Output:

- `resources`: array of `{path, extension, size_bytes, modified_at}`
- optional `nextCursor`

### `godot.editor.state.get`

Input:

- optional `editor_session_id`

Output:

- `source="editor"`
- `active_scene`
- `active_script`
- `root_summary`
- `session_id` (resolved editor session owner)
- `updated_at`

### `godot.project.is_running`

Input:

- optional `session_id` (game session id)
- optional `editor_session_id`

Output:

- `source="runtime"`
- `session_id`
- `editor_session_id`
- `running`
- optional `started_at`
- optional `scene_path`

Notes:

- When `session_id` is omitted, tool resolves an editor session owner first using the session resolution model above.
- If no healthy editor snapshot exists and `session_id` is omitted, tool returns semantic `not_available`.

### `godot.project.run`

Input:

- optional `session_id` (pre-allocated game session id)
- optional `scene_path`
- optional `editor_session_id`

Output:

- `success`
- `source="runtime"`
- `session_id`
- `editor_session_id` (resolved editor command session owner)
- `running`
- `started_at`
- `scene_path`
- optional `already_running`

The tool returns success only after runtime registration and the first runtime snapshot are both observed.
If the editor is already playing, run uses attach/recover behavior: it refreshes runtime handshake metadata and continues with the existing game process instead of failing with `game_already_running`.
During attach/recover remap (`ack.session_id` differs from requested id), server preserves the effective launch token (prefer ack `launch_token`, otherwise keep existing session token) to avoid runtime register token mismatch.
If first snapshot await times out, the server returns semantic `not_available` but keeps the game session mapping for late runtime register recovery.

### `godot.project.stop`

Input:

- optional `session_id` (target game session id)
- optional `editor_session_id`

Output:

- `success`
- `source="runtime"`
- `session_id`
- `editor_session_id` (resolved editor command session owner)
- `running=false`

## Runtime Tool Contracts

### `godot.runtime.session.get_active`

Input:

- optional `editor_session_id`

Output:

- `source="runtime"`
- `session_id`
- `editor_session_id`
- `running`
- `started_at`
- `scene_path`
- `runtime_session_id`
- `has_snapshot`
- `last_snapshot_at`

Notes:

- Resolution order is the same editor-owner model:
  1. optional `editor_session_id`
  2. caller session when caller snapshot is fresh
  3. latest fresh editor snapshot session
- If no healthy editor snapshot exists, returns semantic `not_available`.

### `godot.runtime.await_snapshot`

Input:

- required `session_id`
- optional `min_frame`
- optional `timeout_ms`
- optional `freshness` (`fresh`, `grace`, `stale`)

Output:

- `source="runtime"`
- `session_id`
- `snapshot_id`
- `frame`
- `updated_at`
- `freshness`
- `root_scene_path`
- `root_node_name`

### `godot.runtime.scene_tree.get`

Input:

- required `session_id`
- optional `max_depth`

Output:

- runtime metadata (`source`, `session_id`, `snapshot_id`, `frame`, `updated_at`)
- `root`

### `godot.runtime.node_properties.get`

Input:

- required `session_id`
- required `node`
- required `properties`

Phase 1 supported properties:

- `position`
- `global_position`
- `velocity`
- `visible`
- `modulate`
- `text`
- `frame`
- `animation`
- `enabled`
- `zoom`

### `godot.runtime.log.get`

Input:

- required `session_id`
- optional `level` (`debug`, `info`, `warning`, `error`, `all`)
- optional `limit`
- optional `since_sequence`

Output:

- `source="runtime"`
- `session_id`
- `entries`
- each entry uses `{sequence, time, level, message, source, stack_trace?}`

Current behavior:

- session missing returns `game_session_missing`
- stopped session returns `game_not_running`
- `level="error"` reads the runtime diagnostics stream for the current game session
- `since_sequence` returns only entries with a larger sequence number
- clean runs may legitimately return an empty `entries` array

Current limitation:

- this stream is not yet guaranteed to include all Godot-native GDScript parse/runtime errors
- current coverage is the runtime companion diagnostics path plus runtime-side structured error logging

Current diagnostics sources:

- `runtime_companion`
- `runtime_lifecycle`
- `runtime_command:<tool_name>`

### `godot.runtime.log.clear`

Input:

- required `session_id`

Output:

- `source="runtime"`
- `session_id`
- `cleared`
- `command_id`

### `godot.runtime.screenshot.get`

Output:

- `source="runtime"`
- `session_id`
- `path`
- `width`
- `height`
- `frame`
- `timestamp`

## Script Create Conflict Policy

`godot.script.create` supports:

- `replace` (optional boolean, default `false`)

When target file exists and `replace=false`, the runtime ack envelope returns:

- `success=false`
- `reason=script_exists_requires_replace`
- `error` contains the human-readable conflict message

## Tool Controls

`tool_controls` fields:

- `schema_validation_enabled`
- `reject_unknown_arguments`
- `permission_mode` (`allow_all`, `read_only`, `allow_list`)
- `allowed_tools`
- `emit_progress_notifications`

Controls are additive and do not change canonical naming.

### Internal Bridge Permission Exemption

Runtime bridge internal tools bypass `permission_mode` filtering to preserve session health:

- `godot.bridge.editor.sync`
- `godot.bridge.editor.ping`
- `godot.bridge.runtime.register`
- `godot.bridge.runtime.snapshot.push`
- `godot.bridge.runtime.log.push`
- `godot.bridge.command.ack`

All other tools continue to follow `allow_all` / `read_only` / `allow_list` policy rules.
