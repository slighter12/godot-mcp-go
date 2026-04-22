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

## Upgrade (Current pre-1.0 line)

Current line introduces the following compatibility changes:

0. Single plugin model:
   - If you previously enabled `Godot MCP Runtime Companion` as a separate plugin, disable it in `Project > Project Settings > Plugins`
   - The main `Godot MCP` plugin now manages the runtime companion autoload automatically
   - Runtime companion scripts are now inside `addons/godot_mcp/` — no separate directory needed
1. Mutating tools require capability negotiation:
   - Send `initialize.params.capabilities.godot.mutating=true`
2. `godot.script.create` supports `replace` (default `false`)
   - Existing file + `replace=false` returns conflict semantic reason
3. Prompt rendering mode adds `advanced` with governance enforcement
4. Runtime observability is exposed through:
   - tool: `godot.runtime.health.get`
   - resource: `godot://runtime/metrics`
5. Project tools now return real paginated payloads:
   - `godot.project.settings.get`
   - `godot.project.resources.list`
6. Protocol compatibility is strict:
   - HTTP requires `MCP-Protocol-Version: 2025-11-25`
   - stdio requires `initialize.params.protocolVersion=2025-11-25`

## Transport Notes

- Dual session model is expected:
  - Godot IDE/plugin session can be different from AI/agent caller session
  - this is not an error condition
- Runtime mutating tools require:
  - `streamable_http`
  - initialized caller session
  - caller mutating capability negotiation
  - active runtime bridge
- Editor-backed tools resolve editor owner session by:
  1. optional `editor_session_id`
  2. caller session if caller has fresh editor snapshot
  3. latest fresh editor snapshot
  - applies to `godot.editor.state.get`, `godot.project.is_running`, `godot.runtime.session.get_active`, and editor command routing (`godot.project.run`, `godot.project.stop`, `godot.editor.scene.apply`)
  - if no healthy editor snapshot exists, tool returns semantic `not_available`
- `godot.project.run` keeps game session mapping when first snapshot await times out, so late runtime register can still attach.
- `godot.project.run` attach/recover now preserves effective launch token when remapping to an already-running session id, preventing `godot.bridge.runtime.register` launch token mismatch on runtime side.
- Compatibility fallback for clients that cannot send `capabilities.godot.mutating=true`:
  - set `tool_controls.allow_mutating_without_capability=true`
  - use only for trusted local clients
- Runtime tools no longer borrow the latest session implicitly:
  - use `godot.offerings.list` only as a coarse global signal for `editor_backed` / `runtime_backed` health
  - use `godot.project.is_running` with the intended `editor_session_id` before run/stop/attach-recover decisions when current runtime state is uncertain
  - resolve the active game session through `godot.runtime.session.get_active` with explicit `editor_session_id`
  - fail closed unless the returned `editor_session_id` still matches the intended editor owner
  - call `godot.runtime.await_snapshot` when the next runtime read depends on fresh live state
  - pass only that verified `session_id` to runtime tools
- `stdio` supports read/non-runtime operations and requires strict initialize protocol version.
- Progress notifications (`notifications/progress`) are best-effort and require `_meta.progressToken` in `tools/call`.

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

Scene mutating tools (`godot.scene.create`, `godot.scene.save`, `godot.editor.scene.apply`) are runtime-backed operations and still require an initialized HTTP caller session, caller mutating capability negotiation, and a healthy runtime bridge. `godot.editor.scene.apply` also supports optional `editor_session_id` override for explicit editor owner routing.

## Validation Checklist

See `docs/DEVELOPMENT.md` — Verification Gate section for the full test matrix.
