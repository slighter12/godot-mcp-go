# Install and Upgrade Guide

## Install

1. Build server:

    ```bash
    go build
    ```

2. Place or link plugin into your Godot project at `addons/godot_mcp`.

    ```bash
    mkdir -p /path/to/project/addons
    ln -s /path/to/godot-mcp-go/godot-plugin/addons/godot_mcp /path/to/project/addons/godot_mcp
    ```

3. Enable `Godot MCP` in `Project > Project Settings > Plugins`. The runtime companion autoload is registered automatically.

4. Start server (default Streamable HTTP):

    ```bash
    ./godot-mcp-go
    ```

## Versioning

See [`SKILLS_PUBLISHING.md`](SKILLS_PUBLISHING.md) for the versioning
strategy, Git tag format, and pinned install guidance.

## Upgrade (0.3.0)

Current line introduces the following compatibility changes:

0. Single plugin model:
   - If you previously enabled `Godot MCP Runtime Companion` as a separate plugin, disable it in `Project > Project Settings > Plugins`
   - The main `Godot MCP` plugin now manages the runtime companion autoload automatically
   - Runtime companion scripts are now inside `addons/godot_mcp/` — no separate directory needed
1. MCP now uses the strict `2026-07-28` request envelope:
   - Every request carries `params._meta.io.modelcontextprotocol/protocolVersion` and `clientCapabilities`
   - HTTP also requires `MCP-Protocol-Version`, `Mcp-Method`, and `Accept: application/json, text/event-stream`
   - `Mcp-Name` is required for named tool, resource, and prompt operations
   - Base64 sentinel values in `Mcp-Name` (`=?base64?...?=`) are decoded before comparison
2. The removed `initialize`, `initialized`, `notifications/initialized`, and `MCP-Session-Id` flow must be deleted from clients.
   - HTTP `GET /mcp` and `DELETE /mcp` now return `405`
   - Use `server/discover` for capability discovery and `subscriptions/listen` for long-lived events
3. Mutating tools require a per-request Godot extension capability:
   - Set `_meta` client capability `com.slighter12/godot-mcp.mutating=true`
   - Missing capability returns JSON-RPC `-32021`; the explicit trusted-client fallback remains available
   - Unknown methods return HTTP `404`; missing required capabilities return HTTP `400`
4. `godot.script.create` supports `replace` (default `false`)
   - Existing file + `replace=false` returns conflict semantic reason
5. Prompt rendering mode adds `advanced` with governance enforcement
6. Runtime observability is exposed through:
   - tool: `godot.runtime.health.get`
   - resource: `godot://runtime/metrics`
7. Project tools now return real paginated payloads:
   - `godot.project.settings.get`
   - `godot.project.resources.list`

## Transport Notes

- The MCP transport is stateless. Godot editor ownership is application state carried by the explicit `editor_session_id` extension value.
- Runtime mutating tools require:
  - `streamable_http`
  - per-request Godot extension mutating capability
  - active runtime bridge
- Editor-backed tools resolve editor owner session by:
  1. optional `editor_session_id`
  2. caller session if caller has fresh editor snapshot
  3. latest fresh editor snapshot
  - applies to `godot.editor.state.get`, `godot.project.is_running`, `godot.runtime.session.get_active`, and editor command routing (`godot.project.run`, `godot.project.stop`, `godot.editor.scene.apply`)
  - if no healthy editor snapshot exists, tool returns semantic `not_available`
- `godot.project.run` keeps game session mapping when first snapshot await times out, so late runtime register can still attach.
- `godot.project.run` attach/recover now preserves effective launch token when remapping to an already-running session id, preventing `godot.bridge.runtime.register` launch token mismatch on runtime side.
- Compatibility fallback for trusted clients that cannot send the Godot extension:
  - set `tool_controls.allow_mutating_without_capability=true`
  - use only for trusted local clients
- Runtime tools no longer borrow the latest session implicitly:
  - use `godot.offerings.list` only as a coarse global signal for `editor_backed` / `runtime_backed` health
  - use `godot.project.is_running` with the intended `editor_session_id` before run/stop/attach-recover decisions when current runtime state is uncertain
  - resolve the active game session through `godot.runtime.session.get_active` with explicit `editor_session_id`
  - fail closed unless the returned `editor_session_id` still matches the intended editor owner
  - call `godot.runtime.await_snapshot` when the next runtime read depends on fresh live state
  - pass only that verified `session_id` to runtime tools
- `stdio` and Streamable HTTP share the same request metadata and result envelopes; stdio has no lifecycle handshake.
- Progress notifications (`notifications/progress`) are best-effort and require `_meta.progressToken` in `tools/call`. HTTP keeps progress on that request's SSE response.

## Tool Controls

`tool_controls` defaults remain permissive:

```json
{
  "tool_controls": {
    "schema_validation_enabled": true,
    "reject_unknown_arguments": false,
    "permission_mode": "allow_all",
    "allowed_tools": [],
    "emit_progress_notifications": true,
    "allow_mutating_without_capability": false
  }
}
```

## Prompt Catalog Config Additions

```json
{
  "prompt_catalog": {
    "paths": [
      "/abs/path/to/prompt-sources"
    ],
    "allowed_roots": [
      "/abs/path/to/prompt-sources"
    ],
    "watch": { "mode": "poll" },
    "governance": {
      "roots": [
        { "path": "/abs/path/to/trusted/skills", "tier": "trusted" }
      ]
    },
    "rendering": {
      "mode": "advanced",
      "reject_unknown_arguments": false
    }
  }
}
```

Use this only when you want the MCP server itself to expose file-backed prompt sources through prompt catalog endpoints.

The repository `skills/` directory still primarily serves as companion agent-side skill content. By default, the server does not treat `skills/` as built-in prompt catalog content. Prompt catalog can expose those same `SKILL.md` files as prompt sources only if you intentionally point `prompt_catalog.paths` at them and allow the relevant roots.

## Runtime Bridge Config Additions

```json
{
  "runtime_bridge": {
    "stale_after_seconds": 10,
    "stale_grace_ms": 1500
  }
}
```

## Project Root Resolution

File-backed read tools (`godot.scene.list`, `godot.scene.read`, `godot.script.read`, `godot.script.list`, `godot.script.analyze`, `godot.project.settings.get`, `godot.project.resources.list`) resolve paths against:

1. `GODOT_PROJECT_ROOT`, when set
2. otherwise the server process working directory, searching upward for `project.godot`

If the server is started outside the target Godot project tree, set `GODOT_PROJECT_ROOT=/abs/path/to/project` before using file-backed reads.

Scene mutating tools (`godot.scene.create`, `godot.scene.save`, `godot.editor.scene.apply`) are runtime-backed operations and still require modern request metadata, the per-request mutating capability, and a healthy runtime bridge. `godot.editor.scene.apply` also supports optional `editor_session_id` override for explicit editor owner routing.

## Validation Checklist

See `docs/DEVELOPMENT.md` — Verification Gate section for the full test matrix.
