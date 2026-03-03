package toolpipeline

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestMain(m *testing.M) {
	_ = logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON)
	os.Exit(m.Run())
}

func TestExecute_AllowsRuntimeSyncInReadOnlyMode(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name": "godot.runtime.sync",
		"arguments": map[string]any{
			"snapshot": map[string]any{
				"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
				"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
				"node_details": map[string]any{
					"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
				},
			},
		},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "sync-read-only",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-read-only",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "read_only",
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != false {
		t.Fatalf("expected runtime sync success, got isError=%v", result["isError"])
	}
}

func TestExecute_AllowsRuntimeAckInAllowListMode(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name": "godot.runtime.ack",
		"arguments": map[string]any{
			"command_id": "cmd-missing",
			"success":    true,
			"result":     map[string]any{},
		},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "ack-allow-list",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-allow-list",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "allow_list",
			AllowedTools:            []string{"godot.editor.state.get"},
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected runtime ack semantic error envelope, got isError=%v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected kind %q, got %v", tooltypes.SemanticKindNotAvailable, errPayload["kind"])
	}
	if errPayload["reason"] != "unknown_or_expired_command" {
		t.Fatalf("expected reason unknown_or_expired_command, got %v", errPayload["reason"])
	}
}

func TestExecute_MutatingToolRequiresMutatingCapability(t *testing.T) {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()

	params := mustMarshalParams(t, map[string]any{
		"name":      "godot.project.run",
		"arguments": map[string]any{},
	})

	resp := Execute(ExecuteInput{
		Message: jsonrpc.Request{
			JSONRPC: jsonrpc.Version,
			ID:      "run-mutating-false",
			Method:  "tools/call",
			Params:  params,
		},
		ToolManager: manager,
		Context: ToolCallContext{
			SessionID:          "session-mutating-false",
			SessionInitialized: true,
			MutatingAllowed:    false,
		},
		Options: ToolCallOptions{
			SchemaValidationEnabled: true,
			PermissionMode:          "allow_all",
		},
	})

	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected mutating capability semantic error, got isError=%v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != tooltypes.SemanticKindNotSupported {
		t.Fatalf("expected kind %q, got %v", tooltypes.SemanticKindNotSupported, errPayload["kind"])
	}
	if errPayload["reason"] != "mutating_capability_required" {
		t.Fatalf("expected reason mutating_capability_required, got %v", errPayload["reason"])
	}
}

func mustMarshalParams(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return raw
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return out
}
