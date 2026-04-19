package types

import "testing"

func TestNewRuntimeNotAvailableError_EnforcesCanonicalRuntimeFields(t *testing.T) {
	err := NewRuntimeNotAvailableError("runtime unavailable", "godot.runtime.log.get", "game_session_missing", map[string]any{
		"feature": "spoofed-feature",
		"tool":    "spoofed-tool",
		"code":    "spoofed-code",
		"reason":  "expected-reason",
	})

	if err == nil {
		t.Fatal("expected semantic error")
	}
	if err.Kind != SemanticKindNotAvailable {
		t.Fatalf("expected kind %q, got %q", SemanticKindNotAvailable, err.Kind)
	}
	if err.Data["feature"] != "runtime" {
		t.Fatalf("expected canonical feature runtime, got %v", err.Data["feature"])
	}
	if err.Data["tool"] != "godot.runtime.log.get" {
		t.Fatalf("expected canonical tool, got %v", err.Data["tool"])
	}
	if err.Data["code"] != "game_session_missing" {
		t.Fatalf("expected canonical code game_session_missing, got %v", err.Data["code"])
	}
	if err.Data["reason"] != "expected-reason" {
		t.Fatalf("expected caller reason preserved, got %v", err.Data["reason"])
	}
}

func TestNewRuntimeInvalidParamsError_EnforcesCanonicalRuntimeFields(t *testing.T) {
	err := NewRuntimeInvalidParamsError("invalid runtime params", "godot.runtime.input.tap", "input_not_supported", map[string]any{
		"feature": "spoofed-feature",
		"tool":    "spoofed-tool",
		"code":    "spoofed-code",
		"input":   "KEY_X",
	})

	if err == nil {
		t.Fatal("expected semantic error")
	}
	if err.Kind != SemanticKindInvalidParams {
		t.Fatalf("expected kind %q, got %q", SemanticKindInvalidParams, err.Kind)
	}
	if err.Data["feature"] != "runtime" {
		t.Fatalf("expected canonical feature runtime, got %v", err.Data["feature"])
	}
	if err.Data["tool"] != "godot.runtime.input.tap" {
		t.Fatalf("expected canonical tool, got %v", err.Data["tool"])
	}
	if err.Data["code"] != "input_not_supported" {
		t.Fatalf("expected canonical code input_not_supported, got %v", err.Data["code"])
	}
	if err.Data["input"] != "KEY_X" {
		t.Fatalf("expected caller input preserved, got %v", err.Data["input"])
	}
}
