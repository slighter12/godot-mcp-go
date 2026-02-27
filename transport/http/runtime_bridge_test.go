package http

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
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
			"name": "godot-runtime-sync",
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
			"name": "godot-runtime-sync",
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
			"name":      "godot-editor-get-state",
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
		"name": "godot-runtime-sync",
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
		"name": "godot-runtime-sync",
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
		"name":      "godot-runtime-ping",
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
		"name":      "godot-runtime-ping",
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

func TestRuntimeBridgeConcurrentSessionStress(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	const sessionCount = 24
	sessionIDs := make([]string, 0, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sessionID := fmt.Sprintf("session-stress-%02d", i)
		server.sessionManager.CreateSession(sessionID)
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
				"name": "godot-runtime-sync",
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
				"name":      "godot-editor-get-state",
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
	server.sessionManager.MarkInitialized(sessionID)
	runtimebridge.DefaultStore().Upsert(sessionID, runtimebridge.Snapshot{
		RootSummary: runtimebridge.RootSummary{ActiveScene: "res://Bench.tscn"},
	}, time.Now().UTC())

	params, err := json.Marshal(map[string]any{
		"name":      "godot-editor-get-state",
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
