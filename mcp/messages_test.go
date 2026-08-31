package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

func TestInputSchemaMarshalsEmptyCollectionsAsValidJSONSchema(t *testing.T) {
	raw, err := json.Marshal(InputSchema{Type: "object"})
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	if string(raw) != `{"properties":{},"required":[],"type":"object"}` {
		t.Fatalf("unexpected schema JSON: %s", raw)
	}
}

func TestInputSchemaRoundTripPreservesJSONSchema202012Keywords(t *testing.T) {
	raw := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{},"required":[],"$defs":{"name":{"type":"string"}},"additionalProperties":false}`)
	var schema InputSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}
	roundTrip, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(roundTrip, &got); err != nil {
		t.Fatalf("decode round-trip schema: %v", err)
	}
	if got["$schema"] != "https://json-schema.org/draft/2020-12/schema" || got["additionalProperties"] != false {
		t.Fatalf("schema keywords were not preserved: %#v", got)
	}
	if _, ok := got["$defs"].(map[string]any); !ok {
		t.Fatalf("$defs was not preserved: %#v", got)
	}
}

func TestInputSchemaRoundTripPreservesLargeJSONSchemaInteger(t *testing.T) {
	const source = `{"type":"object","properties":{},"required":[],"const":9007199254740993}`
	var schema InputSchema
	if err := json.Unmarshal([]byte(source), &schema); err != nil {
		t.Fatalf("unmarshal input schema: %v", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	if !strings.Contains(string(raw), `"const":9007199254740993`) {
		t.Fatalf("large integer changed during round trip: %s", raw)
	}
}

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
