package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func TestGetEditorStateTool_RemainsSessionScopedViaHTTPPost(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	initReqA := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-a",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-a", "version": "0.1.0"},
		},
	}
	_, sessionA, status := postMCP(t, server, initReqA, "", "2025-11-25")
	if status != 200 || sessionA == "" {
		t.Fatalf("initialize session A failed, status=%d session=%q", status, sessionA)
	}

	initReqB := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-b",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-b", "version": "0.1.0"},
		},
	}
	_, sessionB, status := postMCP(t, server, initReqB, "", "2025-11-25")
	if status != 200 || sessionB == "" {
		t.Fatalf("initialize session B failed, status=%d session=%q", status, sessionB)
	}
	if sessionA == sessionB {
		t.Fatalf("expected distinct sessions, got %q", sessionA)
	}

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"method":  "notifications/initialized",
	}, sessionA, "2025-11-25")
	if status != 202 {
		t.Fatalf("initialized notify for A failed, status=%d", status)
	}

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"method":  "notifications/initialized",
	}, sessionB, "2025-11-25")
	if status != 202 {
		t.Fatalf("initialized notify for B failed, status=%d", status)
	}

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-a",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "sync-editor-runtime",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://A.tscn"},
					"scene_tree":   map[string]any{"path": "/A", "name": "A", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/A": map[string]any{"path": "/A", "name": "A", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, sessionA, "2025-11-25")
	if status != 200 {
		t.Fatalf("sync A failed, status=%d", status)
	}

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-b",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "sync-editor-runtime",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://B.tscn"},
					"scene_tree":   map[string]any{"path": "/B", "name": "B", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/B": map[string]any{"path": "/B", "name": "B", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, sessionB, "2025-11-25")
	if status != 200 {
		t.Fatalf("sync B failed, status=%d", status)
	}

	respA, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "state-a",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get-editor-state",
			"arguments": map[string]any{},
		},
	}, sessionA, "2025-11-25")
	if status != 200 {
		t.Fatalf("state A failed, status=%d", status)
	}
	respAResult := mustMap(t, respA["result"])
	stateA := mustMap(t, respAResult["result"])
	if got := stateA["active_scene"]; got != "res://A.tscn" {
		t.Fatalf("expected session A scene res://A.tscn, got %v", got)
	}
	if got := stateA["session_id"]; got != sessionA {
		t.Fatalf("expected session A id %q, got %v", sessionA, got)
	}
}

func TestSyncEditorRuntimeTool_WithInitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-sync"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitialized(sessionID)

	params, err := json.Marshal(map[string]any{
		"name": "sync-editor-runtime",
		"arguments": map[string]any{
			"snapshot": map[string]any{
				"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
				"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	respAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "tools/call",
		Params:  params,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handleMessage: %v", handleErr)
	}

	resp, ok := respAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", respAny)
	}
	if resp.Error != nil {
		t.Fatalf("expected success response, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}

	stored, ok, reason := runtimebridge.DefaultStore().LatestFresh(time.Now().UTC())
	if !ok {
		t.Fatalf("expected stored snapshot, reason=%s", reason)
	}
	if stored.SessionID != sessionID {
		t.Fatalf("expected %s, got %s", sessionID, stored.SessionID)
	}
}

func TestSyncEditorRuntimeTool_RejectsUninitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-sync-uninitialized"
	server.sessionManager.CreateSession(sessionID)

	params, err := json.Marshal(map[string]any{
		"name": "sync-editor-runtime",
		"arguments": map[string]any{
			"snapshot": map[string]any{
				"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	respAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "tools/call",
		Params:  params,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handleMessage: %v", handleErr)
	}

	resp, ok := respAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", respAny)
	}
	if resp.Error != nil {
		t.Fatalf("expected tool result response, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != "not_available" {
		t.Fatalf("expected not_available kind, got %v", errPayload["kind"])
	}
}

func TestPingEditorRuntimeTool_WithInitializedSessionAndSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-ping"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitialized(sessionID)
	runtimebridge.DefaultStore().Upsert(sessionID, runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
		SceneTree:   runtimebridge.CompactNode{Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		NodeDetails: map[string]runtimebridge.NodeDetail{
			"/Root": {Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		},
	}, time.Now().UTC().Add(-9*time.Second))

	params, err := json.Marshal(map[string]any{
		"name":      "ping-editor-runtime",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	respAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "tools/call",
		Params:  params,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handleMessage: %v", handleErr)
	}

	resp, ok := respAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", respAny)
	}
	if resp.Error != nil {
		t.Fatalf("expected success response, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
}

func TestPingEditorRuntimeTool_RequiresExistingSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-ping-missing"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitialized(sessionID)

	params, err := json.Marshal(map[string]any{
		"name":      "ping-editor-runtime",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	respAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "tools/call",
		Params:  params,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handleMessage: %v", handleErr)
	}

	resp, ok := respAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", respAny)
	}
	if resp.Error != nil {
		t.Fatalf("expected tool result response, got %+v", resp.Error)
	}
	result := mustMap(t, resp.Result)
	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != "not_available" {
		t.Fatalf("expected not_available kind, got %v", errPayload["kind"])
	}
}
