GO ?= go
SERVER_HOST ?= localhost
SERVER_PORT ?= 9080
SERVER_URL ?= http://$(SERVER_HOST):$(SERVER_PORT)/mcp
INSPECTOR_SERVER_URL ?= http://host.docker.internal:$(SERVER_PORT)/mcp
INSPECTOR_IMAGE ?= ghcr.io/modelcontextprotocol/inspector:latest

.PHONY: help test-go test-http-smoke test-http-ping test-http-delete inspector-pull test-inspector-docker test-all

help:
	@echo "Available targets:"
	@echo "  make test-go               - Run Go unit tests"
	@echo "  make test-http-smoke       - Run Streamable HTTP smoke checks"
	@echo "  make test-http-ping        - Verify Streamable HTTP ping returns an empty result object"
	@echo "  make test-http-delete      - Verify Streamable HTTP DELETE session lifecycle"
	@echo "  make test-inspector-docker - Run MCP Inspector CLI checks in Docker"
	@echo "  make test-all              - Run all tests above"

test-go:
	$(GO) test ./...

test-http-smoke:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-smoke.sh

test-http-ping:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-ping.sh

test-http-delete:
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" SERVER_URL="$(SERVER_URL)" ./scripts/test-http-delete.sh

inspector-pull:
	docker pull $(INSPECTOR_IMAGE)

test-inspector-docker: inspector-pull
	@GO="$(GO)" SERVER_HOST="$(SERVER_HOST)" SERVER_PORT="$(SERVER_PORT)" INSPECTOR_SERVER_URL="$(INSPECTOR_SERVER_URL)" INSPECTOR_IMAGE="$(INSPECTOR_IMAGE)" ./scripts/test-inspector-docker.sh

test-all: test-go test-http-smoke test-http-ping test-http-delete test-inspector-docker
