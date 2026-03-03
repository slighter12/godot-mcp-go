# Install and Upgrade Guide

## Install

1. Build server:

```bash
go build
```

1. Place or link plugin into your Godot project `addons/godot_mcp`.

2. Start server (default Streamable HTTP):

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

## Runtime Bridge Config Additions

```json
{
  "runtime_bridge": {
    "stale_after_seconds": 10,
    "stale_grace_ms": 1500
  }
}
```

## Validation Checklist

1. `go test ./...`
2. `make test-http-smoke`
3. `make test-http-ping`
4. `make test-http-delete`
5. `make test-http-session-isolation`
6. `make test-inspector-docker`
