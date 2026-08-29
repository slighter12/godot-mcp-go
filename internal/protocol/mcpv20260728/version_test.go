package mcpv20260728

import (
	"encoding/base64"
	"testing"
)

func TestDecodeHeaderValueBase64Sentinel(t *testing.T) {
	name := "godot://scene/current"
	header := base64HeaderPrefix + base64.StdEncoding.EncodeToString([]byte(name)) + base64HeaderSuffix
	decoded, err := DecodeHeaderValue(header)
	if err != nil {
		t.Fatalf("decode Base64 header: %v", err)
	}
	if decoded != name {
		t.Fatalf("expected %q, got %q", name, decoded)
	}
}

func TestValidateNameHeaderAcceptsBase64Sentinel(t *testing.T) {
	name := "godot.tool"
	header := base64HeaderPrefix + base64.StdEncoding.EncodeToString([]byte(name)) + base64HeaderSuffix
	if err := ValidateNameHeader("tools/call", name, header); err != nil {
		t.Fatalf("validate Base64 Mcp-Name: %v", err)
	}
}

func TestValidateNameHeaderRejectsUnsafeLiteral(t *testing.T) {
	for _, name := range []string{"godot://scene/當前", "godot://scene/\ncurrent"} {
		if err := ValidateNameHeader("resources/read", name, name); err == nil {
			t.Fatalf("expected unsafe literal %q to require Base64 sentinel", name)
		}
	}
}

func TestValidateNameHeaderAcceptsEncodedUnsafeValue(t *testing.T) {
	name := "godot://scene/當前"
	header := base64HeaderPrefix + base64.StdEncoding.EncodeToString([]byte(name)) + base64HeaderSuffix
	if err := ValidateNameHeader("resources/read", name, header); err != nil {
		t.Fatalf("validate encoded unsafe Mcp-Name: %v", err)
	}
}

func TestDecodeHeaderValueRejectsMalformedSentinel(t *testing.T) {
	if _, err := DecodeHeaderValue("=?base64?not-valid?="); err == nil {
		t.Fatal("expected malformed Base64 sentinel error")
	}
	if _, err := DecodeHeaderValue("=?base64?abc"); err == nil {
		t.Fatal("expected partial Base64 sentinel error")
	}
}

func TestDecodeHeaderValueTreatsSuffixOnlyValueAsLiteral(t *testing.T) {
	name := "godot://x?="
	decoded, err := DecodeHeaderValue(name)
	if err != nil {
		t.Fatalf("expected suffix-only value to remain literal: %v", err)
	}
	if decoded != name {
		t.Fatalf("expected %q, got %q", name, decoded)
	}
	if err := ValidateNameHeader("tools/call", name, name); err != nil {
		t.Fatalf("expected literal Mcp-Name to validate: %v", err)
	}
}

func TestAddSupportedVersionsForInitializeCopiesData(t *testing.T) {
	original := map[string]any{"reason": "missing metadata"}
	result := AddSupportedVersionsForInitialize("initialize", original)
	if result["reason"] != original["reason"] {
		t.Fatalf("expected original data to be preserved: %#v", result)
	}
	if supported, ok := result["supported"].([]string); !ok || len(supported) != 1 || supported[0] != ProtocolVersion {
		t.Fatalf("unexpected supported versions: %#v", result["supported"])
	}
	if _, exists := original["supported"]; exists {
		t.Fatal("expected input data to remain unchanged")
	}
}
