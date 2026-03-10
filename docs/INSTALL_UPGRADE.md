# Install and Upgrade Guide

## Install

1. Build server:

```bash
go build
```

2. Place or link plugin into your Godot project `addons/godot_mcp`.

3. Start server (default Streamable HTTP):

```bash
./godot-mcp-go
```

## Upgrade (Current v1 line)

Current line introduces the following compatibility changes:

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

- Runtime mutating tools require:
  - `streamable_http`
  - initialized session
  - mutating capability negotiation
  - active runtime bridge
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
    "emit_progress_notifications": true
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

This prompt catalog feature is separate from the repository `skills/` directory. The bundled companion skills are external agent-side artifacts that call this server through the `godot.*` MCP tools; the server does not load them as built-in prompt catalog content.

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

Scene mutating tools (`godot.scene.create`, `godot.scene.save`, `godot.scene.apply`) are runtime-backed operations and still require an initialized HTTP session, mutating capability negotiation, and a healthy runtime bridge.

## Validation Checklist

See `docs/DEVELOPMENT.md` — Verification Gate section for the full test matrix.
