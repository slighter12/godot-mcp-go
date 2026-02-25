# Install and Upgrade Guide

## Install

1. Build server:

```bash
go build
```

2. Place or link plugin into your Godot project `addons/godot_mcp`.

3. Start server (default streamable HTTP):

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

## Validation Checklist

1. `go test ./...`
2. `make test-http-smoke`
3. `make test-http-ping`
4. `make test-http-delete`
5. `make test-http-session-isolation`
6. `make test-inspector-docker`
