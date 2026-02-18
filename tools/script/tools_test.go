package script

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
		t.Fatalf("execute read-script: %v", err)
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
