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
	}
	response, err := server.handleMessage(initMsg)
	if err != nil {
		t.Errorf("handleInit failed: %v", err)
	}
	if response == nil {
		t.Error("handleInit returned nil response")
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
			"tool": "list-project-scenes",
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

	// Test progress message
	progressMsg := jsonrpc.Request{
		Method: "tools/progress",
		Params: json.RawMessage(`{
			"tool": "list-project-scenes",
			"progress": 0.5,
			"message": "Processing..."
		}`),
	}
	response, err = server.handleMessage(progressMsg)
	if err != nil {
		t.Errorf("handleProgress failed: %v", err)
	}
	if response != nil {
		t.Error("handleProgress should return nil response")
	}
}

func TestMessageValidation(t *testing.T) {
	// Create a tool manager
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()

	// Create a stdio server
	server := NewStdioServer(toolManager)

	// Test invalid message type
	invalidTypeMsg := jsonrpc.Request{
		Method: "invalid_type",
	}
	response, err := server.handleMessage(invalidTypeMsg)
	if err == nil {
		t.Error("handleMessage should return error for invalid message type")
	}
	if response != nil {
		t.Error("handleMessage should return nil response for invalid message type")
	}

	// Test invalid payload format
	invalidPayloadMsg := jsonrpc.Request{
		Method: "tools/call",
		Params: json.RawMessage(`invalid json`),
	}
	response, err = server.handleMessage(invalidPayloadMsg)
	if err == nil {
		t.Error("handleMessage should return error for invalid payload format")
	}
	if response != nil {
		t.Error("handleMessage should return nil response for invalid payload format")
	}
}
