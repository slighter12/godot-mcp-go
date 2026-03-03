GO ?= go
SERVER_HOST ?= localhost
SERVER_PORT ?= 9080
SERVER_URL ?= http://$(SERVER_HOST):$(SERVER_PORT)/mcp
SESSION_ISOLATION_PORT ?= 19080
INSPECTOR_SERVER_PORT ?= 29080
INSPECTOR_SERVER_URL ?= http://host.docker.internal:$(INSPECTOR_SERVER_PORT)/mcp
INSPECTOR_IMAGE ?= ghcr.io/modelcontextprotocol/inspector:latest

.PHONY: help test-go test-http-smoke test-http-ping test-http-delete test-http-session-isolation test-http-protocol-header test-http-allow-list-runtime-bridge test-lifecycle-initialized-id inspector-pull test-inspector-docker test-inspector-header-negative test-all

help:
	@echo "Available targets:"
	@echo "  make test-go               - Run Go unit tests"
	@echo "  make test-http-smoke       - Run Streamable HTTP smoke checks"
	@echo "  make test-http-ping        - Verify Streamable HTTP ping returns an empty result object"
	@echo "  make test-http-delete      - Verify Streamable HTTP DELETE session lifecycle"
	@echo "  make test-http-session-isolation - Verify runtime snapshot data is session-scoped"
	@echo "  make test-http-protocol-header - Verify protocol header strictness (duplicate/mixed values)"
	@echo "  make test-http-allow-list-runtime-bridge - Verify internal bridge chain under allow_list"
	@echo "  make test-lifecycle-initialized-id - Verify initialized-with-id rejection (HTTP + stdio)"
	@echo "  make test-inspector-docker - Run MCP Inspector CLI checks in Docker"
	@echo "  make test-inspector-header-negative - Verify Inspector call fails without protocol header"
	@echo "  make test-all              - Run all tests above"

test-go:
	$(GO) test ./...

test-http-smoke:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-smoke.sh

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

test-all: test-go test-http-smoke test-http-ping test-http-delete test-http-session-isolation test-http-protocol-header test-http-allow-list-runtime-bridge test-lifecycle-initialized-id test-inspector-docker test-inspector-header-negative
