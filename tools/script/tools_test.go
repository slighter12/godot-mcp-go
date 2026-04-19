package script

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestReadScriptTool_ReadsFileContent(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	scriptContent := "extends Node\nfunc _ready():\n    pass\n"
	if err := os.WriteFile(filepath.Join(projectRoot, "scripts", "Player.gd"), []byte(scriptContent), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	tool := &ReadScriptTool{}
	rawArgs, _ := json.Marshal(map[string]any{"path": "res://scripts/Player.gd"})
	resultRaw, err := tool.Execute(rawArgs)
	if err != nil {
		t.Fatalf("execute godot.script.read: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["path"] != "res://scripts/Player.gd" {
		t.Fatalf("unexpected path: %v", result["path"])
	}
	if result["content"] != scriptContent {
		t.Fatalf("unexpected content: %v", result["content"])
	}
}

func TestScriptWriteTools_ReturnNotAvailable(t *testing.T) {
	tools := []interface {
		Execute(args json.RawMessage) ([]byte, error)
	}{
		&CreateScriptTool{},
		&ModifyScriptTool{},
	}
	for _, tool := range tools {
		_, err := tool.Execute(json.RawMessage(`{}`))
		if err == nil {
			t.Fatal("expected semantic not available error")
		}
		semanticErr, ok := tooltypes.AsSemanticError(err)
		if !ok {
			t.Fatalf("expected semantic error, got %T", err)
		}
		if semanticErr.Kind != tooltypes.SemanticKindNotAvailable {
			t.Fatalf("expected not_available kind, got %s", semanticErr.Kind)
		}
	}
}

func TestScriptModifyTool_DispatchesToRuntimeCommandSessionID(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"modified": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &ModifyScriptTool{}
	raw := json.RawMessage(`{
		"path": "res://scripts/Player.gd",
		"content": "extends Node\nfunc _ready():\n    print('hello')\n",
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": "editor-1"
		}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.script.modify: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}

func TestScriptCreateTool_DispatchesToRuntimeCommandSessionID(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultEditorStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert("editor-1", runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
	}, now)

	dispatchedTo := ""
	broker := runtimebridge.DefaultCommandBroker()
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = broker.Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"created": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	tool := &CreateScriptTool{}
	raw := json.RawMessage(`{
		"path": "res://scripts/NewScript.gd",
		"content": "extends Node\n",
		"_mcp": {
			"session_id": "ai-session",
			"session_initialized": true,
			"runtime_command_session_id": "editor-1"
		}
	}`)
	resultRaw, err := tool.Execute(raw)
	if err != nil {
		t.Fatalf("execute godot.script.create: %v", err)
	}
	if dispatchedTo != "editor-1" {
		t.Fatalf("expected dispatch to editor-1, got %q", dispatchedTo)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
}
