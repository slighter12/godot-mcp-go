package stdio

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
)

func TestMain(m *testing.M) {
	// Set up logging
	logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON, "logs/stdio.log")

	// Run tests
	os.Exit(m.Run())
}

func TestStdioServer(t *testing.T) {
	// Create a tool manager
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()

	// Create a stdio server
	server := NewStdioServer(toolManager)

	// Test init message
	initMsg := jsonrpc.Request{
		Method: "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-11-25",
			"capabilities": {},
			"clientInfo": {"name":"test","version":"0.1.0"}
		}`),
	}
	response, err := server.handleMessage(initMsg)
	if err != nil {
		t.Errorf("handleInit failed: %v", err)
	}
	if response == nil {
		t.Error("handleInit returned nil response")
	}

	initializedMsg := jsonrpc.Request{
		Method: "notifications/initialized",
	}
	response, err = server.handleMessage(initializedMsg)
	if err != nil {
		t.Errorf("handle initialized notification failed: %v", err)
	}
	if response != nil {
		t.Errorf("initialized notification should not return response, got %T", response)
	}

	// Test ping message
	pingMsg := jsonrpc.Request{
		Method: "ping",
	}
	response, err = server.handleMessage(pingMsg)
	if err != nil {
		t.Errorf("handlePing failed: %v", err)
	}
	if response == nil {
		t.Error("handlePing returned nil response")
	}

	// Test tool call message
	toolCallMsg := jsonrpc.Request{
		Method: "tools/call",
		Params: json.RawMessage(`{
			"tool": "godot.scene.list",
			"arguments": {}
		}`),
	}
	response, err = server.handleMessage(toolCallMsg)
	if err != nil {
		t.Errorf("handleToolCall failed: %v", err)
	}
	if response == nil {
		t.Error("handleToolCall returned nil response")
	}

	// Test tools list message
	toolsListMsg := jsonrpc.Request{
		Method: "tools/list",
	}
	response, err = server.handleMessage(toolsListMsg)
	if err != nil {
		t.Errorf("handleToolsList failed: %v", err)
	}
	if response == nil {
		t.Error("handleToolsList returned nil response")
	}

}

func TestMessageValidation(t *testing.T) {
	// Create a tool manager
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()

	// Create a stdio server
	server := NewStdioServer(toolManager)

	// Test invalid message type before initialization gate
	invalidTypeMsg := jsonrpc.Request{
		Method: "invalid_type",
	}
	response, err := server.handleMessage(invalidTypeMsg)
	if err != nil {
		t.Errorf("handleMessage returned unexpected error for invalid notification method: %v", err)
	}
	rpcResp, ok := response.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatal("expected invalid request response before session initialization")
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("expected invalid request, got %d", rpcResp.Error.Code)
	}

	// Complete stdio handshake for standard method validation.
	initResp, err := server.handleMessage(jsonrpc.Request{
		ID:     "init",
		Method: "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-11-25",
			"capabilities": {},
			"clientInfo": {"name":"test","version":"0.1.0"}
		}`),
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	initRPCResp, ok := initResp.(*jsonrpc.Response)
	if !ok || initRPCResp.Error != nil {
		t.Fatalf("expected successful initialize response, got %#v", initResp)
	}
	notifyResp, err := server.handleMessage(jsonrpc.Request{
		Method: "notifications/initialized",
	})
	if err != nil {
		t.Fatalf("initialized notification failed: %v", err)
	}
	if notifyResp != nil {
		t.Fatalf("expected nil response for initialized notification, got %#v", notifyResp)
	}

	// Unknown request method with id should return method-not-found error response after initialization.
	unknownRequestMsg := jsonrpc.Request{
		ID:     "req-1",
		Method: "invalid_type",
	}
	response, err = server.handleMessage(unknownRequestMsg)
	if err != nil {
		t.Errorf("handleMessage returned unexpected error for unknown request method: %v", err)
	}
	rpcUnknownResp, ok := response.(*jsonrpc.Response)
	if !ok {
		t.Fatal("unknown request method should return jsonrpc.Response")
	}
	if rpcUnknownResp.Error == nil {
		t.Fatal("unknown request method should return JSON-RPC error response")
	}
	if rpcUnknownResp.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Errorf("expected method-not-found error code, got %d", rpcUnknownResp.Error.Code)
	}

	// Test invalid payload format
	invalidPayloadMsg := jsonrpc.Request{
		Method: "tools/call",
		Params: json.RawMessage(`invalid json`),
	}
	response, err = server.handleMessage(invalidPayloadMsg)
	if err != nil {
		t.Errorf("handleMessage returned unexpected error for invalid payload format: %v", err)
	}
	if response == nil {
		t.Fatal("handleMessage should return error response for invalid payload format")
	}
	rpcResp, ok = response.(*jsonrpc.Response)
	if !ok {
		t.Fatal("handleMessage should return jsonrpc.Response for invalid payload format")
	}
	if rpcResp.Error == nil {
		t.Error("invalid payload should produce JSON-RPC error response")
	}
}

func TestInitializedNotificationBeforeInitializeRejected(t *testing.T) {
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()
	server := NewStdioServer(toolManager)

	resp, err := server.handleMessage(jsonrpc.Request{
		Method: "notifications/initialized",
	})
	if err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}
	rpcResp, ok := resp.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatalf("expected invalid request response, got %T", resp)
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("expected invalid request code, got %d", rpcResp.Error.Code)
	}
}

func TestInitializedNotificationWithIDRejected(t *testing.T) {
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()
	server := NewStdioServer(toolManager)

	resp, err := server.handleMessage(jsonrpc.Request{
		ID:     "invalid-notify-id",
		Method: "notifications/initialized",
	})
	if err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}
	rpcResp, ok := resp.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatalf("expected invalid request response, got %T", resp)
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("expected invalid request code, got %d", rpcResp.Error.Code)
	}
	if rpcResp.Error.Message != "Invalid request" {
		t.Fatalf("expected invalid request message, got %q", rpcResp.Error.Message)
	}
}

func TestStandardMethodsRequireInitializedInStdio(t *testing.T) {
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()
	server := NewStdioServer(toolManager)

	resp, err := server.handleMessage(jsonrpc.Request{
		ID:     "tools-list-before-init",
		Method: "tools/list",
		Params: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}
	rpcResp, ok := resp.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatalf("expected invalid request response, got %T", resp)
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidRequest) {
		t.Fatalf("expected invalid request code, got %d", rpcResp.Error.Code)
	}
}

func TestInitializeRequiresExactProtocolVersion(t *testing.T) {
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()
	server := NewStdioServer(toolManager)

	resp, err := server.handleMessage(jsonrpc.Request{
		ID:     "init-missing",
		Method: "initialize",
		Params: json.RawMessage(`{"capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}`),
	})
	if err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}
	rpcResp, ok := resp.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatalf("expected JSON-RPC error response, got %T", resp)
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params, got %d", rpcResp.Error.Code)
	}

	resp, err = server.handleMessage(jsonrpc.Request{
		ID:     "init-wrong",
		Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}`),
	})
	if err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}
	rpcResp, ok = resp.(*jsonrpc.Response)
	if !ok || rpcResp.Error == nil {
		t.Fatalf("expected JSON-RPC error response, got %T", resp)
	}
	if rpcResp.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params, got %d", rpcResp.Error.Code)
	}

}
