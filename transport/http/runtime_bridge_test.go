package http

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
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
			"clientInfo":      map[string]any{"name": "test-a", "version": "0.2.0"},
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
			"clientInfo":      map[string]any{"name": "test-b", "version": "0.2.0"},
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
			"name": "godot.bridge.editor.sync",
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
			"name": "godot.bridge.editor.sync",
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
			"name":      "godot.editor.state.get",
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

func TestGetEditorStateTool_UsesLatestFreshEditorSessionAcrossHTTPPostSessions(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-state-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-state-cross", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	initAIReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-state-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "ai-state-cross", "version": "0.2.0"},
		},
	}
	_, aiSessionID, status := postMCP(t, server, initAIReq, "", protocolVersion)
	if status != 200 || aiSessionID == "" {
		t.Fatalf("initialize ai session failed, status=%d session=%q", status, aiSessionID)
	}
	notifyInitialized(t, server, aiSessionID)

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-editor-state-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://EditorOwner.tscn"},
					"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("sync editor snapshot failed, status=%d", status)
	}

	resp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "state-ai-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.editor.state.get",
			"arguments": map[string]any{},
		},
	}, aiSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("state get failed, status=%d", status)
	}
	result := mustMap(t, resp["result"])
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
	state := mustMap(t, result["result"])
	if state["active_scene"] != "res://EditorOwner.tscn" {
		t.Fatalf("expected active_scene res://EditorOwner.tscn, got %v", state["active_scene"])
	}
	if state["session_id"] != editorSessionID {
		t.Fatalf("expected session_id=%q, got %v", editorSessionID, state["session_id"])
	}
}

func TestProjectIsRunningTool_UsesLatestFreshEditorSessionAcrossHTTPPostSessions(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-running-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-running-cross", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	initAIReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-running-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "ai-running-cross", "version": "0.2.0"},
		},
	}
	_, aiSessionID, status := postMCP(t, server, initAIReq, "", protocolVersion)
	if status != 200 || aiSessionID == "" {
		t.Fatalf("initialize ai session failed, status=%d session=%q", status, aiSessionID)
	}
	notifyInitialized(t, server, aiSessionID)

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-editor-running-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://EditorOwner.tscn"},
					"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("sync editor snapshot failed, status=%d", status)
	}

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_cross", editorSessionID, "res://EditorOwner.tscn", "launch_cross", now)

	resp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "is-running-ai-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.project.is_running",
			"arguments": map[string]any{},
		},
	}, aiSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("project is_running failed, status=%d", status)
	}
	result := mustMap(t, resp["result"])
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
	running := mustMap(t, result["result"])
	if running["running"] != true {
		t.Fatalf("expected running=true, got %v", running["running"])
	}
	if running["session_id"] != "game_cross" {
		t.Fatalf("expected session_id=game_cross, got %v", running["session_id"])
	}
	if running["editor_session_id"] != editorSessionID {
		t.Fatalf("expected editor_session_id=%q, got %v", editorSessionID, running["editor_session_id"])
	}
}

func TestRuntimeSessionGetActiveTool_UsesLatestFreshEditorSessionAcrossHTTPPostSessions(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-active-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-active-cross", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	initAIReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-active-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "ai-active-cross", "version": "0.2.0"},
		},
	}
	_, aiSessionID, status := postMCP(t, server, initAIReq, "", protocolVersion)
	if status != 200 || aiSessionID == "" {
		t.Fatalf("initialize ai session failed, status=%d session=%q", status, aiSessionID)
	}
	notifyInitialized(t, server, aiSessionID)

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-editor-active-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://EditorOwner.tscn"},
					"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("sync editor snapshot failed, status=%d", status)
	}

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_active_cross", editorSessionID, "res://EditorOwner.tscn", "launch_active_cross", now)

	resp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "get-active-ai-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.runtime.session.get_active",
			"arguments": map[string]any{},
		},
	}, aiSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime session get_active failed, status=%d", status)
	}
	result := mustMap(t, resp["result"])
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
	active := mustMap(t, result["result"])
	if active["session_id"] != "game_active_cross" {
		t.Fatalf("expected session_id=game_active_cross, got %v", active["session_id"])
	}
	if active["editor_session_id"] != editorSessionID {
		t.Fatalf("expected editor_session_id=%q, got %v", editorSessionID, active["editor_session_id"])
	}
	if active["running"] != true {
		t.Fatalf("expected running=true, got %v", active["running"])
	}
}

func TestProjectRunTool_EnforcesMutatingGateAndUsesEditorOwnerAcrossHTTPPostSessions(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-run-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-run-cross", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	initAIMutatingReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-mutating-run-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "ai-mutating-run-cross", "version": "0.2.0"},
		},
	}
	_, aiMutatingSessionID, status := postMCP(t, server, initAIMutatingReq, "", protocolVersion)
	if status != 200 || aiMutatingSessionID == "" {
		t.Fatalf("initialize mutating ai session failed, status=%d session=%q", status, aiMutatingSessionID)
	}
	notifyInitialized(t, server, aiMutatingSessionID)

	initAIReadonlyReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-readonly-run-cross",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "ai-readonly-run-cross", "version": "0.2.0"},
		},
	}
	_, aiReadonlySessionID, status := postMCP(t, server, initAIReadonlyReq, "", protocolVersion)
	if status != 200 || aiReadonlySessionID == "" {
		t.Fatalf("initialize readonly ai session failed, status=%d session=%q", status, aiReadonlySessionID)
	}
	notifyInitialized(t, server, aiReadonlySessionID)

	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-editor-run-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://EditorOwner.tscn"},
					"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("sync editor snapshot failed, status=%d", status)
	}

	dispatchedTo := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		if sessionID != editorSessionID {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		gameSessionID, _ := arguments["session_id"].(string)
		go func() {
			now := time.Now().UTC()
			runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport(gameSessionID, "runtime_cross", editorSessionID, "res://EditorOwner.tscn", now, "launch_cross")
			runtimebridge.DefaultRuntimeSnapshotStore().Upsert(gameSessionID, runtimebridge.RuntimeSnapshot{
				SessionID:     gameSessionID,
				SnapshotID:    "snap_cross_1",
				Frame:         1,
				UpdatedAt:     now.Format(time.RFC3339Nano),
				RootScenePath: "res://EditorOwner.tscn",
				RootNodeName:  "Main",
				NodeCount:     1,
				Running:       true,
			}, now)
			_ = runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result: map[string]any{
					"running":    true,
					"session_id": gameSessionID,
					"scene_path": "res://EditorOwner.tscn",
				},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	runResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "run-ai-mutating-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.project.run",
			"arguments": map[string]any{},
		},
	}, aiMutatingSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("project run (mutating session) failed, status=%d", status)
	}
	if dispatchedTo != editorSessionID {
		t.Fatalf("expected dispatch to editor session %q, got %q", editorSessionID, dispatchedTo)
	}
	runResult := mustMap(t, runResp["result"])
	if runResult["isError"] != false {
		t.Fatalf("expected run isError=false, got %v", runResult["isError"])
	}

	readonlyRunResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "run-ai-readonly-cross",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.project.run",
			"arguments": map[string]any{},
		},
	}, aiReadonlySessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("project run (readonly session) failed, status=%d", status)
	}
	readonlyResult := mustMap(t, readonlyRunResp["result"])
	if readonlyResult["isError"] != true {
		t.Fatalf("expected readonly run isError=true, got %v", readonlyResult["isError"])
	}
	readonlyErr := mustMap(t, readonlyResult["error"])
	if readonlyErr["reason"] != "mutating_capability_required" {
		t.Fatalf("expected mutating_capability_required, got %v", readonlyErr["reason"])
	}
}

func TestSyncEditorRuntimeTool_WithInitializedSession(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-sync"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitializeAccepted(sessionID)
	server.sessionManager.MarkInitialized(sessionID)

	params, err := json.Marshal(map[string]any{
		"name": "godot.bridge.editor.sync",
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

	stored, ok, reason := runtimebridge.DefaultEditorStore().LatestFresh(time.Now().UTC())
	if !ok {
		t.Fatalf("expected stored snapshot, reason=%s", reason)
	}
	if stored.SessionID != sessionID {
		t.Fatalf("expected %s, got %s", sessionID, stored.SessionID)
	}
}

func TestSyncEditorRuntimeTool_RejectsBeforeInitializedLifecycleGate(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-sync-uninitialized"
	server.sessionManager.CreateSession(sessionID)

	params, err := json.Marshal(map[string]any{
		"name": "godot.bridge.editor.sync",
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
	if resp.Error == nil {
		t.Fatalf("expected invalid request lifecycle gate error, got result %#v", resp.Result)
	}
	if resp.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("expected invalid request code %d, got %d", int(jsonrpc.ErrInvalidRequest), resp.Error.Code)
	}
	if resp.Error.Message != "Session is not initialized" {
		t.Fatalf("expected session not initialized message, got %q", resp.Error.Message)
	}
}

func TestPingEditorRuntimeTool_WithInitializedSessionAndSnapshot(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	sessionID := "session-ping"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitializeAccepted(sessionID)
	server.sessionManager.MarkInitialized(sessionID)
	runtimebridge.DefaultEditorStore().Upsert(sessionID, runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Main.tscn"},
		SceneTree:   runtimebridge.CompactNode{Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		NodeDetails: map[string]runtimebridge.NodeDetail{
			"/Root": {Path: "/Root", Name: "Root", Type: "Node2D", ChildCount: 0},
		},
	}, time.Now().UTC().Add(-9*time.Second))

	params, err := json.Marshal(map[string]any{
		"name":      "godot.bridge.editor.ping",
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
	server.sessionManager.MarkInitializeAccepted(sessionID)
	server.sessionManager.MarkInitialized(sessionID)

	params, err := json.Marshal(map[string]any{
		"name":      "godot.bridge.editor.ping",
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

func TestRuntimeBridgeReadOnlyPolicy_AllowsInternalToolChain(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)
	server.config.ToolControls.PermissionMode = "read_only"

	sessionID := "session-read-only-internal"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitializeAccepted(sessionID)
	server.sessionManager.MarkInitialized(sessionID)

	syncParams, err := json.Marshal(map[string]any{
		"name": "godot.bridge.editor.sync",
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
	if err != nil {
		t.Fatalf("marshal sync params: %v", err)
	}
	syncRespAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "sync",
		Method:  "tools/call",
		Params:  syncParams,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handle sync message: %v", handleErr)
	}
	syncResp, ok := syncRespAny.(*jsonrpc.Response)
	if !ok || syncResp.Error != nil {
		t.Fatalf("expected sync success response, got %#v", syncRespAny)
	}
	syncResult := mustMap(t, syncResp.Result)
	if syncResult["isError"] != false {
		t.Fatalf("expected sync isError=false, got %v", syncResult["isError"])
	}

	editorStateParams, err := json.Marshal(map[string]any{
		"name":      "godot.editor.state.get",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal editor state params: %v", err)
	}
	editorStateRespAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "state",
		Method:  "tools/call",
		Params:  editorStateParams,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handle editor state message: %v", handleErr)
	}
	editorStateResp, ok := editorStateRespAny.(*jsonrpc.Response)
	if !ok || editorStateResp.Error != nil {
		t.Fatalf("expected editor state success response, got %#v", editorStateRespAny)
	}
	editorStateResult := mustMap(t, editorStateResp.Result)
	if editorStateResult["isError"] != false {
		t.Fatalf("expected editor state isError=false, got %v", editorStateResult["isError"])
	}

	pingParams, err := json.Marshal(map[string]any{
		"name":      "godot.bridge.editor.ping",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal ping params: %v", err)
	}
	pingRespAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "ping",
		Method:  "tools/call",
		Params:  pingParams,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handle ping message: %v", handleErr)
	}
	pingResp, ok := pingRespAny.(*jsonrpc.Response)
	if !ok || pingResp.Error != nil {
		t.Fatalf("expected ping success response, got %#v", pingRespAny)
	}
	pingResult := mustMap(t, pingResp.Result)
	if pingResult["isError"] != false {
		t.Fatalf("expected ping isError=false, got %v", pingResult["isError"])
	}

	ackParams, err := json.Marshal(map[string]any{
		"name": "godot.bridge.command.ack",
		"arguments": map[string]any{
			"command_id": "cmd-nonexistent",
			"success":    true,
			"result":     map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("marshal ack params: %v", err)
	}
	ackRespAny, handleErr := server.handleMessage(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "ack",
		Method:  "tools/call",
		Params:  ackParams,
	}, sessionID)
	if handleErr != nil {
		t.Fatalf("handle ack message: %v", handleErr)
	}
	ackResp, ok := ackRespAny.(*jsonrpc.Response)
	if !ok || ackResp.Error != nil {
		t.Fatalf("expected ack tool response, got %#v", ackRespAny)
	}
	ackResult := mustMap(t, ackResp.Result)
	if ackResult["isError"] != true {
		t.Fatalf("expected ack isError=true for unknown command id, got %v", ackResult["isError"])
	}
	ackErr := mustMap(t, ackResult["error"])
	if ackErr["kind"] != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected ack kind %q, got %v", tooltypes.SemanticKindNotAvailable, ackErr["kind"])
	}
	if ackErr["reason"] != "unknown_or_expired_command" {
		t.Fatalf("expected ack reason unknown_or_expired_command, got %v", ackErr["reason"])
	}
}

func TestRuntimeLogRoundTripViaHTTPPost(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-log",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-log", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	initRuntimeReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-runtime-log",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "runtime-log", "version": "0.2.0"},
		},
	}
	_, runtimeSessionID, status := postMCP(t, server, initRuntimeReq, "", protocolVersion)
	if status != 200 || runtimeSessionID == "" {
		t.Fatalf("initialize runtime session failed, status=%d session=%q", status, runtimeSessionID)
	}
	notifyInitialized(t, server, runtimeSessionID)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_http_log", editorSessionID, "res://Main.tscn", "launch_ok", now)

	registerResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "register-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.runtime.register",
			"arguments": map[string]any{
				"session_id":        "game_http_log",
				"editor_session_id": editorSessionID,
				"scene_path":        "res://Main.tscn",
				"launch_token":      "launch_ok",
				"started_at":        now.Format(time.RFC3339Nano),
			},
		},
	}, runtimeSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime register failed, status=%d", status)
	}
	registerPayload := mustMap(t, registerResp["result"])
	if registerPayload["isError"] != false {
		t.Fatalf("expected register isError=false, got %v", registerPayload["isError"])
	}
	registerResult := mustMap(t, registerPayload["result"])
	if registerResult["runtime_session_id"] != runtimeSessionID {
		t.Fatalf("expected runtime_session_id %q, got %v", runtimeSessionID, registerResult["runtime_session_id"])
	}

	pushResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "push-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.runtime.log.push",
			"arguments": map[string]any{
				"session_id": "game_http_log",
				"entries": []map[string]any{
					{"level": "info", "message": "boot", "source": "runtime_companion"},
					{"level": "error", "message": "boom-1", "source": "runtime_command:godot.runtime.input.tap", "stack_trace": "input.gd:10"},
					{"level": "error", "message": "boom-2", "source": "runtime_command:godot.runtime.screenshot.get"},
				},
			},
		},
	}, runtimeSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log push failed, status=%d", status)
	}
	pushPayload := mustMap(t, pushResp["result"])
	if pushPayload["isError"] != false {
		t.Fatalf("expected log push isError=false, got %v", pushPayload["isError"])
	}
	pushResult := mustMap(t, pushPayload["result"])
	if pushResult["appended"] != float64(3) {
		t.Fatalf("expected appended=3, got %v", pushResult["appended"])
	}

	getResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "get-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.get",
			"arguments": map[string]any{
				"session_id": "game_http_log",
				"level":      "error",
				"limit":      10,
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log get failed, status=%d", status)
	}
	getPayload := mustMap(t, getResp["result"])
	if getPayload["isError"] != false {
		t.Fatalf("expected log get isError=false, got %v", getPayload["isError"])
	}
	getResult := mustMap(t, getPayload["result"])
	entriesRaw, ok := getResult["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries array, got %T", getResult["entries"])
	}
	if len(entriesRaw) != 2 {
		t.Fatalf("expected 2 error entries, got %d", len(entriesRaw))
	}
	firstEntry := mustMap(t, entriesRaw[0])
	secondEntry := mustMap(t, entriesRaw[1])
	if firstEntry["source"] != "runtime_command:godot.runtime.input.tap" {
		t.Fatalf("unexpected first source %v", firstEntry["source"])
	}
	if secondEntry["message"] != "boom-2" {
		t.Fatalf("expected second message boom-2, got %v", secondEntry["message"])
	}
	firstSequence, ok := firstEntry["sequence"].(float64)
	if !ok {
		t.Fatalf("expected numeric sequence, got %T", firstEntry["sequence"])
	}

	getSinceResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "get-runtime-log-since",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.get",
			"arguments": map[string]any{
				"session_id":     "game_http_log",
				"level":          "error",
				"since_sequence": firstSequence,
				"limit":          10,
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log get since failed, status=%d", status)
	}
	getSincePayload := mustMap(t, getSinceResp["result"])
	if getSincePayload["isError"] != false {
		t.Fatalf("expected log get since isError=false, got %v", getSincePayload["isError"])
	}
	getSinceResult := mustMap(t, getSincePayload["result"])
	sinceEntries, ok := getSinceResult["entries"].([]any)
	if !ok {
		t.Fatalf("expected since entries array, got %T", getSinceResult["entries"])
	}
	if len(sinceEntries) != 1 {
		t.Fatalf("expected 1 entry after since_sequence, got %d", len(sinceEntries))
	}
	sinceEntry := mustMap(t, sinceEntries[0])
	if sinceEntry["message"] != "boom-2" {
		t.Fatalf("expected remaining message boom-2, got %v", sinceEntry["message"])
	}

	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		if sessionID != runtimeSessionID {
			return false
		}
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"cleared": true},
				AckedAt:   time.Now().UTC(),
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	clearResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "clear-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.clear",
			"arguments": map[string]any{
				"session_id": "game_http_log",
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log clear failed, status=%d", status)
	}
	clearPayload := mustMap(t, clearResp["result"])
	if clearPayload["isError"] != false {
		t.Fatalf("expected log clear isError=false, got %v", clearPayload["isError"])
	}
	clearResult := mustMap(t, clearPayload["result"])
	if clearResult["cleared"] != float64(3) {
		t.Fatalf("expected cleared=3, got %v", clearResult["cleared"])
	}

	getAfterClearResp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "get-runtime-log-after-clear",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.get",
			"arguments": map[string]any{
				"session_id": "game_http_log",
				"level":      "all",
				"limit":      10,
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log get after clear failed, status=%d", status)
	}
	getAfterClearPayload := mustMap(t, getAfterClearResp["result"])
	if getAfterClearPayload["isError"] != false {
		t.Fatalf("expected log get after clear isError=false, got %v", getAfterClearPayload["isError"])
	}
	getAfterClearResult := mustMap(t, getAfterClearPayload["result"])
	clearedEntries, ok := getAfterClearResult["entries"].([]any)
	if !ok {
		t.Fatalf("expected cleared entries array, got %T", getAfterClearResult["entries"])
	}
	if len(clearedEntries) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(clearedEntries))
	}
}

func TestRuntimeLogGetTool_ReturnsStoppedSessionErrorViaHTTPPost(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"
	initReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-stopped-log",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "stopped-log", "version": "0.2.0"},
		},
	}
	_, sessionID, status := postMCP(t, server, initReq, "", protocolVersion)
	if status != 200 || sessionID == "" {
		t.Fatalf("initialize session failed, status=%d session=%q", status, sessionID)
	}
	notifyInitialized(t, server, sessionID)

	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game_stopped", sessionID, "res://Stopped.tscn", "launch_ok", now)
	runtimebridge.DefaultGameSessionRegistry().StopSession("game_stopped", now.Add(time.Second))

	resp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "get-stopped-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.get",
			"arguments": map[string]any{
				"session_id": "game_stopped",
			},
		},
	}, sessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("runtime log get for stopped session failed, status=%d", status)
	}
	result := mustMap(t, resp["result"])
	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result["isError"])
	}
	errPayload := mustMap(t, result["error"])
	if errPayload["kind"] != tooltypes.SemanticKindNotAvailable {
		t.Fatalf("expected kind %q, got %v", tooltypes.SemanticKindNotAvailable, errPayload["kind"])
	}
	if errPayload["code"] != "game_not_running" {
		t.Fatalf("expected code game_not_running, got %v", errPayload["code"])
	}
}

func TestRuntimeBridgeConcurrentSessionStress(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	const sessionCount = 24
	sessionIDs := make([]string, 0, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sessionID := fmt.Sprintf("session-stress-%02d", i)
		server.sessionManager.CreateSession(sessionID)
		server.sessionManager.MarkInitializeAccepted(sessionID)
		server.sessionManager.MarkInitialized(sessionID)
		sessionIDs = append(sessionIDs, sessionID)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, sessionCount*2)
	for i, sessionID := range sessionIDs {
		wg.Add(1)
		go func(index int, sid string) {
			defer wg.Done()

			syncParams, err := json.Marshal(map[string]any{
				"name": "godot.bridge.editor.sync",
				"arguments": map[string]any{
					"snapshot": map[string]any{
						"root_summary": map[string]any{
							"active_scene": fmt.Sprintf("res://Scene-%02d.tscn", index),
						},
						"scene_tree": map[string]any{
							"path":        fmt.Sprintf("/Scene%02d", index),
							"name":        fmt.Sprintf("Scene%02d", index),
							"type":        "Node2D",
							"child_count": 0,
						},
					},
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("marshal sync params: %w", err)
				return
			}
			syncRespAny, handleErr := server.handleMessage(jsonrpc.Request{
				JSONRPC: jsonrpc.Version,
				ID:      fmt.Sprintf("sync-%02d", index),
				Method:  "tools/call",
				Params:  syncParams,
			}, sid)
			if handleErr != nil {
				errCh <- fmt.Errorf("handle sync message: %w", handleErr)
				return
			}
			syncResp, ok := syncRespAny.(*jsonrpc.Response)
			if !ok || syncResp == nil {
				errCh <- fmt.Errorf("sync response type mismatch: %T", syncRespAny)
				return
			}
			if syncResp.Error != nil {
				errCh <- fmt.Errorf("sync response error: %+v", syncResp.Error)
				return
			}

			stateParams, err := json.Marshal(map[string]any{
				"name":      "godot.editor.state.get",
				"arguments": map[string]any{},
			})
			if err != nil {
				errCh <- fmt.Errorf("marshal state params: %w", err)
				return
			}
			stateRespAny, handleErr := server.handleMessage(jsonrpc.Request{
				JSONRPC: jsonrpc.Version,
				ID:      fmt.Sprintf("state-%02d", index),
				Method:  "tools/call",
				Params:  stateParams,
			}, sid)
			if handleErr != nil {
				errCh <- fmt.Errorf("handle state message: %w", handleErr)
				return
			}
			stateResp, ok := stateRespAny.(*jsonrpc.Response)
			if !ok || stateResp == nil {
				errCh <- fmt.Errorf("state response type mismatch: %T", stateRespAny)
				return
			}
			if stateResp.Error != nil {
				errCh <- fmt.Errorf("state response error: %+v", stateResp.Error)
				return
			}
			result, ok := stateResp.Result.(map[string]any)
			if !ok {
				errCh <- fmt.Errorf("state result type mismatch: %T", stateResp.Result)
				return
			}
			toolResult, ok := result["result"].(map[string]any)
			if !ok {
				errCh <- fmt.Errorf("tool result type mismatch: %T", result["result"])
				return
			}
			expectedScene := fmt.Sprintf("res://Scene-%02d.tscn", index)
			if toolResult["active_scene"] != expectedScene {
				errCh <- fmt.Errorf("expected active_scene %q, got %v", expectedScene, toolResult["active_scene"])
				return
			}
			if toolResult["session_id"] != sid {
				errCh <- fmt.Errorf("expected session_id %q, got %v", sid, toolResult["session_id"])
				return
			}
		}(i, sessionID)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent stress failure: %v", err)
		}
	}
}

func TestSendRuntimeCommandProgressNotification_UsesCanonicalMethod(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.config.ToolControls.EmitProgressNotifications = true

	sessionID := "session-progress-method"
	server.sessionManager.CreateSession(sessionID)
	rec := httptest.NewRecorder()
	transport := NewStreamableHTTPTransport(rec, rec)
	if !server.sessionManager.SetTransport(sessionID, transport) {
		t.Fatalf("failed to bind transport for session %q", sessionID)
	}

	server.SendRuntimeCommandProgressNotification(tooltypes.RuntimeCommandProgressEvent{
		SessionID:     sessionID,
		CommandName:   "godot.project.run",
		Progress:      0.4,
		Message:       "dispatching runtime command",
		ProgressToken: "req-progress-1",
	})

	body := rec.Body.String()
	if !strings.Contains(body, "\"method\":\"notifications/progress\"") {
		t.Fatalf("expected notifications/progress, got %q", body)
	}
	if strings.Contains(body, "notifications/tools/progress") {
		t.Fatalf("expected no legacy progress notification method, got %q", body)
	}
	if !strings.Contains(body, "\"progressToken\":\"req-progress-1\"") {
		t.Fatalf("expected progress token payload, got %q", body)
	}
	if !strings.Contains(body, "\"total\":1") {
		t.Fatalf("expected total=1 payload, got %q", body)
	}
}

func TestSendRuntimeCommandProgressNotification_DropsInvalidToken(t *testing.T) {
	server := newTestHTTPServer(t, true)
	server.config.ToolControls.EmitProgressNotifications = true

	sessionID := "session-progress-invalid"
	server.sessionManager.CreateSession(sessionID)
	rec := httptest.NewRecorder()
	transport := NewStreamableHTTPTransport(rec, rec)
	if !server.sessionManager.SetTransport(sessionID, transport) {
		t.Fatalf("failed to bind transport for session %q", sessionID)
	}

	server.SendRuntimeCommandProgressNotification(tooltypes.RuntimeCommandProgressEvent{
		SessionID:     sessionID,
		CommandName:   "godot.project.run",
		Progress:      0.4,
		Message:       "dispatching runtime command",
		ProgressToken: "",
	})

	if rec.Body.Len() != 0 {
		t.Fatalf("expected no progress notification for invalid token, got %q", rec.Body.String())
	}
}

func BenchmarkHandleMessageGetEditorStateParallel(b *testing.B) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	initHTTPTestLogger.Do(func() {
		if err := logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON); err != nil {
			b.Fatalf("init logger: %v", err)
		}
	})
	cfg := config.NewConfig()
	cfg.PromptCatalog.Enabled = true
	server := NewServer(cfg)
	server.promptCatalog = promptcatalog.NewRegistry(true)
	server.toolManager.RegisterDefaultTools()
	if err := server.registerRuntimeTools(); err != nil {
		b.Fatalf("register runtime tools: %v", err)
	}
	if err := server.registry.RegisterServer("default", server.toolManager.GetTools()); err != nil {
		b.Fatalf("register default server: %v", err)
	}

	sessionID := "session-bench-state"
	server.sessionManager.CreateSession(sessionID)
	server.sessionManager.MarkInitializeAccepted(sessionID)
	server.sessionManager.MarkInitialized(sessionID)
	runtimebridge.DefaultEditorStore().Upsert(sessionID, runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Bench.tscn"},
	}, time.Now().UTC())

	params, err := json.Marshal(map[string]any{
		"name":      "godot.editor.state.get",
		"arguments": map[string]any{},
	})
	if err != nil {
		b.Fatalf("marshal params: %v", err)
	}

	req := jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      "bench-state",
		Method:  "tools/call",
		Params:  params,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, handleErr := server.handleMessage(req, sessionID)
			if handleErr != nil {
				b.Fatalf("handleMessage failed: %v", handleErr)
			}
		}
	})
}

func TestNodeCreateTool_DispatchesToEditorSessionNotCallerViaHTTPPost(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultCommandBrokerForTests(2 * time.Second)
	runtimebridge.ResetDefaultRuntimeSnapshotStoreForTests(10*time.Second, 0)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	server := newTestHTTPServer(t, true)

	const protocolVersion = "2025-11-25"

	// Initialize editor session with mutating capability.
	initEditorReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-editor-node-dispatch",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "editor-node-dispatch", "version": "0.2.0"},
		},
	}
	_, editorSessionID, status := postMCP(t, server, initEditorReq, "", protocolVersion)
	if status != 200 || editorSessionID == "" {
		t.Fatalf("initialize editor session failed, status=%d session=%q", status, editorSessionID)
	}
	notifyInitialized(t, server, editorSessionID)

	// Initialize AI session with mutating capability.
	initAIReq := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "init-ai-node-dispatch",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"godot": map[string]any{"mutating": true},
			},
			"clientInfo": map[string]any{"name": "ai-node-dispatch", "version": "0.2.0"},
		},
	}
	_, aiSessionID, status := postMCP(t, server, initAIReq, "", protocolVersion)
	if status != 200 || aiSessionID == "" {
		t.Fatalf("initialize AI session failed, status=%d session=%q", status, aiSessionID)
	}
	notifyInitialized(t, server, aiSessionID)

	// Sync editor snapshot to editor session.
	_, _, status = postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "sync-editor-node-dispatch",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
					"scene_tree":   map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					"node_details": map[string]any{
						"/Root": map[string]any{"path": "/Root", "name": "Root", "type": "Node2D", "child_count": 0},
					},
				},
			},
		},
	}, editorSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("sync editor snapshot failed, status=%d", status)
	}

	// Set NotificationSender that captures dispatch target and auto-acks.
	dispatchedTo := ""
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		dispatchedTo = sessionID
		params, _ := message["params"].(map[string]any)
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"created": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	// POST node.create from AI session — should dispatch to editor session.
	resp, _, status := postMCP(t, server, map[string]any{
		"jsonrpc": jsonrpc.Version,
		"id":      "node-create-ai",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.node.create",
			"arguments": map[string]any{
				"type":   "Node2D",
				"parent": "/root",
				"name":   "TestNode",
			},
		},
	}, aiSessionID, protocolVersion)
	if status != 200 {
		t.Fatalf("node.create via AI session failed, status=%d", status)
	}
	if dispatchedTo != editorSessionID {
		t.Fatalf("expected dispatch to editor session %q, got %q", editorSessionID, dispatchedTo)
	}
	result := mustMap(t, resp["result"])
	if result["isError"] != false {
		t.Fatalf("expected isError=false, got %v", result["isError"])
	}
}
