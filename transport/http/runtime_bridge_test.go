package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

func TestModernHTTPRuntimeStateIsSessionScoped(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)

	for _, session := range []string{"editor-a", "editor-b"} {
		response, _, status := postModernMCP(t, server, map[string]any{
			"jsonrpc": "2.0",
			"id":      "sync-" + session,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "godot.bridge.editor.sync",
				"arguments": map[string]any{
					"snapshot": map[string]any{
						"root_summary": map[string]any{"active_scene": "res://" + session + ".tscn"},
					},
				},
			},
		}, session, mcpv20260728.ProtocolVersion)
		if status != http.StatusOK || response["error"] != nil {
			t.Fatalf("sync %s failed: status=%d response=%#v", session, status, response)
		}
	}

	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "state-a",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.editor.state.get",
			"arguments": map[string]any{},
		},
	}, "editor-a", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("state get status=%d", status)
	}
	result := mustMap(t, mustMap(t, response["result"])["structuredContent"])
	if result["active_scene"] != "res://editor-a.tscn" || result["session_id"] != "editor-a" {
		t.Fatalf("unexpected session-scoped state: %#v", result)
	}
}

func TestModernHTTPRuntimeReadUsesLatestFreshEditorForUnownedClient(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	server := newTestHTTPServer(t, true)

	_, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "sync-editor",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://EditorOwner.tscn"},
				},
			},
		},
	}, "editor-owner", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("editor sync status=%d", status)
	}
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun(
		"game-owner", "editor-owner", "res://EditorOwner.tscn", "launch-owner", time.Now().UTC(),
	)

	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "is-running",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.project.is_running",
			"arguments": map[string]any{},
		},
	}, "", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("project.is_running status=%d", status)
	}
	result := mustMap(t, mustMap(t, response["result"])["structuredContent"])
	if result["running"] != true || result["editor_session_id"] != "editor-owner" {
		t.Fatalf("unexpected latest-editor result: %#v", result)
	}
}

func TestModernHTTPMutatingCapabilityErrorUses400(t *testing.T) {
	server := newTestHTTPServer(t, true)
	response, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "mutating-denied",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "godot.project.run",
			"arguments": map[string]any{},
		},
	}, "editor-readonly", mcpv20260728.ProtocolVersion)
	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
	errorObject := assertRPCError(t, response, jsonrpc.ErrMissingRequiredClientCapability)
	data := mustMap(t, errorObject["data"])
	if data["tool"] != "godot.project.run" {
		t.Fatalf("unexpected capability error data: %#v", data)
	}
	capabilities := data["requiredCapabilities"].([]any)
	if len(capabilities) != 1 || capabilities[0] != "extensions.com.slighter12/godot-mcp.mutating" {
		t.Fatalf("unexpected required capabilities: %#v", capabilities)
	}

	progressParams := modernParams(map[string]any{
		"name":      "godot.project.run",
		"arguments": map[string]any{},
	}, modernClientOptions{EditorSessionID: "editor-readonly"})
	progressParams["_meta"].(map[string]any)["progressToken"] = "progress-denied"
	progressResponse, _, progressStatus := postRawMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "mutating-denied-progress",
		"method":  "tools/call",
		"params":  progressParams,
	}, map[string]string{
		headerProtocolVersion: mcpv20260728.ProtocolVersion,
		headerMethod:          "tools/call",
		headerName:            "godot.project.run",
	})
	if progressStatus != http.StatusBadRequest {
		t.Fatalf("expected progress capability status %d, got %d", http.StatusBadRequest, progressStatus)
	}
	assertRPCError(t, progressResponse, jsonrpc.ErrMissingRequiredClientCapability)
}

func TestModernHTTPMutatingCommandUsesExplicitEditorOwner(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(500 * time.Millisecond)
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)
	_, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "sync-owner",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{
				"snapshot": map[string]any{
					"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
				},
			},
		},
	}, "editor-owner", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("editor sync status=%d", status)
	}
	var dispatchedTo string
	runtimebridge.SetNotificationSender(func(sessionID string, message map[string]any) bool {
		params, _ := message["params"].(map[string]any)
		if message["method"] != "notifications/godot/command" {
			return false
		}
		dispatchedTo = sessionID
		commandID, _ := params["command_id"].(string)
		go func() {
			_ = runtimebridge.DefaultCommandBroker().Ack(sessionID, runtimebridge.CommandAck{
				CommandID: commandID,
				Success:   true,
				Result:    map[string]any{"running": true},
			})
		}()
		return true
	})
	defer runtimebridge.SetNotificationSender(nil)

	response, _, status := postModernMCPWithOptions(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "mutating-allowed",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.node.create",
			"arguments": map[string]any{
				"type": "Node2D", "parent": "/root", "name": "CreatedNode",
			},
		},
	}, modernClientOptions{EditorSessionID: "editor-owner", Mutating: true}, mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("expected mutating call status 200, got %d response=%#v", status, response)
	}
	if dispatchedTo != "editor-owner" {
		t.Fatalf("expected command dispatch to editor-owner, got %q", dispatchedTo)
	}
	result := mustMap(t, response["result"])
	if result["isError"] != false {
		t.Fatalf("expected successful mutating result, got %#v", result)
	}
}

func TestModernHTTPRuntimeLogRoundTrip(t *testing.T) {
	runtimebridge.ResetDefaultGameSessionRegistryForTests()
	runtimebridge.ResetDefaultRuntimeLogStoreForTests(100)
	server := newTestHTTPServer(t, true)
	now := time.Now().UTC()
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun("game-log", "editor-log", "res://Main.tscn", "launch-log", now)

	register, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "register-runtime",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.runtime.register",
			"arguments": map[string]any{
				"session_id": "game-log", "editor_session_id": "editor-log",
				"scene_path": "res://Main.tscn", "launch_token": "launch-log",
				"started_at": now.Format(time.RFC3339Nano),
			},
		},
	}, "editor-log", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK || mustMap(t, register["result"])["isError"] != false {
		t.Fatalf("runtime register failed: status=%d response=%#v", status, register)
	}

	push, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "push-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.runtime.log.push",
			"arguments": map[string]any{
				"session_id": "game-log",
				"entries": []map[string]any{
					{"level": "info", "message": "boot", "source": "runtime"},
					{"level": "error", "message": "boom", "source": "runtime-command"},
				},
			},
		},
	}, "editor-log", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK || mustMap(t, push["result"])["isError"] != false {
		t.Fatalf("runtime log push failed: status=%d response=%#v", status, push)
	}

	get, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "get-runtime-log",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.runtime.log.get",
			"arguments": map[string]any{
				"session_id": "game-log", "level": "error", "limit": 10,
			},
		},
	}, "editor-log", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("runtime log get status=%d", status)
	}
	result := mustMap(t, mustMap(t, get["result"])["structuredContent"])
	entries := result["entries"].([]any)
	if len(entries) != 1 || mustMap(t, entries[0])["message"] != "boom" {
		t.Fatalf("unexpected runtime log entries: %#v", entries)
	}
}

func TestModernHTTPConcurrentEditorStateRequests(t *testing.T) {
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)
	const count = 8
	var waitGroup sync.WaitGroup
	errorsCh := make(chan string, count)
	for index := 0; index < count; index++ {
		index := index
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			sessionID := "editor-stress-" + string(rune('a'+index))
			response, _, status := postModernMCP(t, server, map[string]any{
				"jsonrpc": "2.0",
				"id":      index,
				"method":  "tools/call",
				"params": map[string]any{
					"name": "godot.bridge.editor.sync",
					"arguments": map[string]any{"snapshot": map[string]any{
						"root_summary": map[string]any{"active_scene": "res://stress.tscn"},
					}},
				},
			}, sessionID, mcpv20260728.ProtocolVersion)
			if status != http.StatusOK || response["error"] != nil {
				errorsCh <- sessionID
			}
		}()
	}
	waitGroup.Wait()
	close(errorsCh)
	for sessionID := range errorsCh {
		t.Fatalf("concurrent sync failed for %s", sessionID)
	}
}

func TestModernHTTPProgressCancellationSuppressesLateEditorFallback(t *testing.T) {
	runtimebridge.ResetDefaultCommandBrokerForTests(100 * time.Millisecond)
	runtimebridge.ResetDefaultStoreForTests(10 * time.Second)
	server := newTestHTTPServer(t, true)
	server.config.ToolControls.EmitProgressNotifications = true
	_, _, status := postModernMCP(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      "sync-cancel-owner",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "godot.bridge.editor.sync",
			"arguments": map[string]any{"snapshot": map[string]any{
				"root_summary": map[string]any{"active_scene": "res://Main.tscn"},
			}},
		},
	}, "editor-cancel", mcpv20260728.ProtocolVersion)
	if status != http.StatusOK {
		t.Fatalf("editor sync status=%d", status)
	}
	var fallbackCalls int
	var mu sync.Mutex
	runtimebridge.SetNotificationSender(func(_ string, message map[string]any) bool {
		if message["method"] == "notifications/godot/command" {
			return true
		}
		mu.Lock()
		fallbackCalls++
		mu.Unlock()
		return false
	})
	defer runtimebridge.SetNotificationSender(nil)

	params := modernParams(map[string]any{
		"name": "godot.node.create",
		"arguments": map[string]any{
			"type": "Node2D", "parent": "/root", "name": "CancelledNode",
		},
	}, modernClientOptions{EditorSessionID: "editor-cancel", Mutating: true})
	meta := params["_meta"].(map[string]any)
	meta["progressToken"] = "progress-cancel"
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "cancelled-request", "method": "tools/call", "params": params,
	})
	if err != nil {
		t.Fatalf("marshal cancellation request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(raw))).WithContext(ctx)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAccept, "application/json, text/event-stream")
	req.Header.Set(headerProtocolVersion, mcpv20260728.ProtocolVersion)
	req.Header.Set(headerMethod, "tools/call")
	req.Header.Set(headerName, "godot.node.create")
	recorder := newSynchronizedResponseRecorder()
	done := make(chan error, 1)
	go func() {
		done <- server.handleStreamableHTTPPost(echo.New().NewContext(req, recorder))
	}()
	waitForBodyContains(t, recorder, "notifications/progress")
	cancel()
	select {
	case handlerErr := <-done:
		if handlerErr != nil {
			t.Fatalf("cancelled progress handler: %v", handlerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled progress handler did not stop")
	}
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	lateFallbackCalls := fallbackCalls
	mu.Unlock()
	if lateFallbackCalls != 0 {
		t.Fatalf("expected no late editor fallback after cancellation, got %d", lateFallbackCalls)
	}
}

func toolStructuredContent(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	result := mustMap(t, response["result"])
	return mustMap(t, result["structuredContent"])
}
