package mcp

import (
	"testing"

	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

func TestNewServerErrorMessageNormalizesOutOfRangeCodes(t *testing.T) {
	for _, code := range []jsonrpc.ErrorCode{-31999, -32100, -32603} {
		message := NewServerErrorMessage("client", "server", code, "error", nil)
		if message.Code != jsonrpc.ErrServerError {
			t.Fatalf("expected out-of-range code %d to normalize to %d, got %d", code, jsonrpc.ErrServerError, message.Code)
		}
	}
}

func TestNewServerErrorMessagePreservesReservedServerCode(t *testing.T) {
	message := NewServerErrorMessage("client", "server", jsonrpc.ErrMissingRequiredClientCapability, "error", nil)
	if message.Code != jsonrpc.ErrMissingRequiredClientCapability {
		t.Fatalf("expected reserved server code to be preserved, got %d", message.Code)
	}
}
