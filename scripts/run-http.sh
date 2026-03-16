#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
TMP_ROOT="${TMPDIR:-/tmp}"

if [ -z "${GOCACHE:-}" ]; then
  export GOCACHE="${TMP_ROOT%/}/godot-mcp-go-build-cache"
fi

exec "$GO_BIN" run main.go
