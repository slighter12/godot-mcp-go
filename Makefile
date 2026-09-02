GO ?= go
SERVER_HOST ?= localhost
SERVER_PORT ?= 9080
SERVER_URL ?= http://$(SERVER_HOST):$(SERVER_PORT)/mcp
SESSION_ISOLATION_PORT ?= 19080
INSPECTOR_SERVER_PORT ?= 29080
INSPECTOR_SERVER_URL ?= http://host.docker.internal:$(INSPECTOR_SERVER_PORT)/mcp
INSPECTOR_IMAGE ?= ghcr.io/modelcontextprotocol/inspector:2.4.0

.PHONY: help run-http test-go test-http-smoke test-http-runtime-log-smoke test-http-ping test-http-delete test-http-session-isolation test-http-protocol-header test-http-allow-list-runtime-bridge test-lifecycle-initialized-id inspector-pull test-inspector-docker test-inspector-header-negative test-conformance-cleanup test-conformance-2026-07-28 test-addon-static test-quick test-release test-all

help:
	@echo "Available targets:"
	@echo "  make run-http              - Run the Streamable HTTP server on the configured host/port"
	@echo "  make test-go               - Run Go unit tests"
	@echo "  make test-http-smoke       - Run Streamable HTTP smoke checks"
	@echo "  make test-http-runtime-log-smoke - Run runtime log HTTP smoke checks"
	@echo "  make test-http-ping        - Verify removed ping is rejected and server/discover is available"
	@echo "  make test-http-delete      - Verify removed Streamable HTTP DELETE returns 405"
	@echo "  make test-http-session-isolation - Verify explicit editor session routing"
	@echo "  make test-http-protocol-header - Verify duplicate/mixed protocol headers are rejected"
	@echo "  make test-http-allow-list-runtime-bridge - Verify internal bridge chain under allow_list"
	@echo "  make test-lifecycle-initialized-id - Verify removed initialize/initialized methods and direct requests"
	@echo "  make test-inspector-docker - Run MCP Inspector CLI checks in Docker"
	@echo "  make test-inspector-header-negative - Verify Inspector call fails without valid protocol header"
	@echo "  make test-conformance-2026-07-28 - Run the pinned official MCP conformance suite"
	@echo "  make test-conformance-cleanup - Verify the fixture process is reaped"
	@echo "  make test-addon-static     - Run the Godot addon static check"
	@echo "  make test-quick            - Run local Go and addon checks without Docker or Bun"
	@echo "  make test-release          - Run every documented release gate"
	@echo "  make test-all              - Alias for test-release"

run-http:
	@GOCACHE="$${GOCACHE:-$${TMPDIR:-/tmp}/godot-mcp-go-build-cache}" $(GO) run main.go

test-go:
	$(GO) test ./...

test-http-smoke:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-smoke.sh

test-http-runtime-log-smoke:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-runtime-log-smoke.sh

test-http-ping:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-ping.sh

test-http-delete:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-delete.sh

test-http-session-isolation:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SESSION_ISOLATION_PORT)" SERVER_URL="http://$(SERVER_HOST):$(SESSION_ISOLATION_PORT)/mcp" ./scripts/test-http-session-isolation.sh

test-http-protocol-header:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-protocol-header.sh

test-http-allow-list-runtime-bridge:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SESSION_ISOLATION_PORT)" SERVER_URL="http://$(SERVER_HOST):$(SESSION_ISOLATION_PORT)/mcp" ./scripts/test-http-allow-list-runtime-bridge.sh

test-lifecycle-initialized-id:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-lifecycle-initialized-id.sh

inspector-pull:
	docker pull $(INSPECTOR_IMAGE)

test-inspector-docker: inspector-pull
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(INSPECTOR_SERVER_PORT)" INSPECTOR_SERVER_URL="$(INSPECTOR_SERVER_URL)" INSPECTOR_IMAGE="$(INSPECTOR_IMAGE)" ./scripts/test-inspector-docker.sh

test-inspector-header-negative: inspector-pull
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(INSPECTOR_SERVER_PORT)" INSPECTOR_SERVER_URL="$(INSPECTOR_SERVER_URL)" INSPECTOR_IMAGE="$(INSPECTOR_IMAGE)" ./scripts/test-inspector-header-negative.sh

test-conformance-2026-07-28:
	@GO="$(GO)" ./scripts/test-conformance-2026-07-28.sh

test-conformance-cleanup:
	@GO="$(GO)" ./scripts/test-conformance-cleanup.sh

test-addon-static:
	@./godot-plugin/addons/godot_mcp/static_check.sh

test-quick: test-go test-addon-static

test-release:
	@set -e; \
	for target in \
		test-go \
		test-http-smoke \
		test-http-runtime-log-smoke \
		test-http-ping \
		test-http-delete \
		test-http-session-isolation \
		test-http-protocol-header \
		test-http-allow-list-runtime-bridge \
		test-lifecycle-initialized-id \
		test-inspector-docker \
		test-inspector-header-negative \
		test-conformance-cleanup \
		test-conformance-2026-07-28 \
		test-addon-static; do \
		$(MAKE) "$$target"; \
	done

test-all: test-release
