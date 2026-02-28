# Install and Upgrade Guide

## Install

1. Build server:

```bash
go build
```

1. Place or link plugin into your Godot project `addons/godot_mcp`.

2. Start server (default streamable HTTP):

```bash
./godot-mcp-go
```

## Upgrade to v1 Canonical Tools

v1 introduces canonical `godot-*` tool names.

Recommended client migration:

1. Switch tool calls to canonical names from `tools/list`.
2. Remove legacy tool-name fallbacks from clients/plugins.
3. Handle semantic `error.kind` values explicitly.

After this upgrade, legacy tool names fail with `tool not found`.

## Transport Notes

- Mutating runtime tools require `streamable_http` and initialized session context.
- `stdio` mutating calls that depend on runtime bridge return `not_available`.
- Runtime command progress notifications (`notifications/tools/progress`) are best-effort SSE notifications.

## Tool Controls Upgrade Note

`tool_controls` is optional and defaults to permissive behavior (`allow_all` + schema validation on).

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

Set `permission_mode=read_only` or `allow_list` for stricter runtime policies.

## Validation Checklist

1. `go test ./...`
2. `make test-http-smoke`
3. `make test-http-ping`
4. `make test-http-delete`
5. `make test-http-session-isolation`
6. `go test ./transport/http -run TestRuntimeBridgeConcurrentSessionStress -count=1`
7. `go test ./runtimebridge -run '^$' -bench CommandBroker -benchmem`
8. `go test ./transport/http -run '^$' -bench HandleMessageGetEditorStateParallel -benchmem`
9. `make test-inspector-docker` (optional when Docker is available)
