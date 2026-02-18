package scene

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestReadSceneTool_ReadsContentAndNodeSummary(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "project.godot"), []byte("[application]"), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "scenes"), 0o755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	sceneContent := "[gd_scene format=3]\n[node name=\"Root\" type=\"Node2D\"]\n[node name=\"Child\" type=\"Sprite2D\" parent=\".\"]\n"
	if err := os.WriteFile(filepath.Join(projectRoot, "scenes", "Main.tscn"), []byte(sceneContent), 0o644); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	t.Setenv("GODOT_PROJECT_ROOT", projectRoot)

	tool := &ReadSceneTool{}
	rawArgs, _ := json.Marshal(map[string]any{"path": "res://scenes/Main.tscn"})
	resultRaw, err := tool.Execute(rawArgs)
	if err != nil {
		t.Fatalf("execute read-scene: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["path"] != "res://scenes/Main.tscn" {
		t.Fatalf("unexpected path: %v", result["path"])
	}
	if result["content"] != sceneContent {
		t.Fatalf("unexpected content: %v", result["content"])
	}
	nodes, ok := result["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("expected two parsed nodes, got %T %v", result["nodes"], result["nodes"])
	}
}

func TestSceneWriteTools_ReturnNotAvailable(t *testing.T) {
	tools := []interface {
		Execute(args json.RawMessage) ([]byte, error)
	}{
		&CreateSceneTool{},
		&SaveSceneTool{},
		&ApplySceneTool{},
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
