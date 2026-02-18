package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

func TestGetSceneTreeTool_UsesRuntimeSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.DefaultStore().Upsert("session-1", runtimebridge.Snapshot{
		SceneTree: runtimebridge.CompactNode{Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
	}, time.Now().UTC())

	tool := &GetSceneTreeTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`))
	if err != nil {
		t.Fatalf("execute get-scene-tree: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	root, ok := result["root"].(map[string]any)
	if !ok {
		t.Fatalf("expected root map, got %T", result["root"])
	}
	if root["name"] != "Root" {
		t.Fatalf("expected root name Root, got %v", root["name"])
	}
}

func TestGetSceneTreeTool_IsSessionScoped(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	now := time.Now().UTC()
	runtimebridge.DefaultStore().Upsert("session-1", runtimebridge.Snapshot{
		SceneTree: runtimebridge.CompactNode{Path: "/RootA", Name: "RootA", Type: "Node2D", ChildCount: 0},
	}, now)
	runtimebridge.DefaultStore().Upsert("session-2", runtimebridge.Snapshot{
		SceneTree: runtimebridge.CompactNode{Path: "/RootB", Name: "RootB", Type: "Node2D", ChildCount: 0},
	}, now.Add(1*time.Second))

	tool := &GetSceneTreeTool{}
	resultRaw, err := tool.Execute(json.RawMessage(`{"_mcp":{"session_id":"session-1","session_initialized":true}}`))
	if err != nil {
		t.Fatalf("execute get-scene-tree: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	root, ok := result["root"].(map[string]any)
	if !ok {
		t.Fatalf("expected root map, got %T", result["root"])
	}
	if root["name"] != "RootA" {
		t.Fatalf("expected root name RootA, got %v", root["name"])
	}
}

func TestNodeWriteTools_ReturnNotAvailable(t *testing.T) {
	tools := []interface {
		Execute(args json.RawMessage) ([]byte, error)
	}{
		&CreateNodeTool{},
		&DeleteNodeTool{},
		&ModifyNodeTool{},
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

func TestResolveNodeDetail_NameFallbackDeterministic(t *testing.T) {
	details := map[string]runtimebridge.NodeDetail{
		"/B": {Path: "/B", Name: "Enemy"},
		"/A": {Path: "/A", Name: "Enemy"},
	}

	detail, ok := resolveNodeDetail(details, "Enemy")
	if !ok {
		t.Fatal("expected to resolve node detail by name")
	}
	if detail.Path != "/A" {
		t.Fatalf("expected deterministic first key /A, got %s", detail.Path)
	}
}
