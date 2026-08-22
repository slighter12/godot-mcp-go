package stdio

import (
	"os"
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

func TestMain(m *testing.M) {
	_ = logger.Init(logger.GetLevelFromString("error"), logger.FormatJSON, "logs/stdio.log")
	os.Exit(m.Run())
}

func TestModernStdioDispatchesDiscoverToolsAndCalls(t *testing.T) {
	server := newTestStdioServer(true)
	options := modernStdioClientOptions{EditorSessionID: "editor-stdio"}

	discover := dispatchModernTest(t, server, "discover", "server/discover", map[string]any{}, options)
	if discover.Error != nil {
		t.Fatalf("server/discover error: %#v", discover.Error)
	}
	if mustMap(t, discover.Result)["resultType"] != "complete" {
		t.Fatalf("unexpected discovery result: %#v", discover.Result)
	}

	list := dispatchModernTest(t, server, "tools", "tools/list", map[string]any{}, options)
	if list.Error != nil || mustMap(t, list.Result)["resultType"] != "complete" {
		t.Fatalf("unexpected tools/list response: %#v", list)
	}

	call := dispatchModernTest(t, server, "health", "tools/call", map[string]any{
		"name":      "godot.runtime.health.get",
		"arguments": map[string]any{},
	}, options)
	if call.Error != nil {
		t.Fatalf("tools/call error: %#v", call.Error)
	}
	result := mustMap(t, call.Result)
	if result["resultType"] != "complete" || result["isError"] != false {
		t.Fatalf("unexpected tools/call result: %#v", result)
	}
}

func TestModernStdioRemovedMethodsReturnDiagnostics(t *testing.T) {
	server := newTestStdioServer(true)
	response := dispatchModernTest(t, server, 1, "initialize", map[string]any{}, modernStdioClientOptions{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected method-not-found error, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if supported, ok := data["supported"].([]string); !ok || len(supported) != 1 || supported[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("expected supported version diagnostic, got %#v", data["supported"])
	}
}

func TestModernStdioInvalidMetadataAndUnknownMethod(t *testing.T) {
	server := newTestStdioServer(true)
	response := protocolErrorResponse(1, "initialize", mcpv20260728.ErrMissingClientCapabilities)
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid metadata response, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if data["supported"].([]string)[0] != mcpv20260728.ProtocolVersion {
		t.Fatalf("expected supported version diagnostic, got %#v", data)
	}

	unknown := dispatchModernTest(t, server, "unknown", "not/implemented", map[string]any{}, modernStdioClientOptions{})
	if unknown.Error == nil || unknown.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected unknown method error, got %#v", unknown)
	}
}
