package stdio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/tools"
)

// StdioServer handles MCP communication over stdio
type StdioServer struct {
	toolManager *tools.Manager
}

// NewStdioServer creates a new stdio server
func NewStdioServer(toolManager *tools.Manager) *StdioServer {
	return &StdioServer{
		toolManager: toolManager,
	}
}

// Start starts the stdio server
func (s *StdioServer) Start() error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	logger.Debug("Stdio server started and waiting for messages")

	for {
		var msg jsonrpc.Request
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				logger.Debug("Stdio EOF received, terminating server")
				return nil
			}
			logger.Error("Error decoding message", "error", err)
			continue
		}

		logger.Debug("Stdio message received", "method", msg.Method)

		response, err := s.handleMessage(msg)
		if err != nil {
			logger.Error("Error handling message", "error", err)
			continue
		}

		if response != nil {
			if err := encoder.Encode(response); err != nil {
				logger.Error("Error encoding response", "error", err)
				continue
			}
			logger.Debug("Stdio response sent", "type", getResponseType(response))
		}
	}
}

// getResponseType extracts the type field from various response types
func getResponseType(response any) string {
	switch r := response.(type) {
	case *mcp.InitMessage:
		return string(r.Type)
	case *mcp.ResultMessage:
		return string(r.Type)
	case *mcp.ErrorMessage:
		return string(r.Type)
	case *mcp.PongMessage:
		return string(r.Type)
	default:
		return "unknown"
	}
}

func (s *StdioServer) handleMessage(msg jsonrpc.Request) (any, error) {
	switch msg.Method {
	case "initialize":
		logger.Debug("Handling init message")
		return s.handleInit()
	case "tools/call":
		logger.Debug("Handling tool call message")
		return s.handleToolCall(msg)
	case "ping":
		logger.Debug("Handling ping message")
		return s.handlePing()
	case "tools/progress":
		logger.Debug("Handling tool progress message")
		return nil, nil // Progress messages are one-way
	default:
		logger.Debug("Received unknown message type", "method", msg.Method)
		return nil, fmt.Errorf("unknown message type: %s", msg.Method)
	}
}

func (s *StdioServer) handleInit() (*mcp.InitMessage, error) {
	// Create a new tool manager
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()

	// Call the listScenes tool to get available scenes
	result, err := toolManager.CallTool("list-project-scenes", map[string]any{})
	if err != nil {
		return nil, err
	}

	// Create the response with available tools and scenes
	response := &mcp.InitMessage{
		Type:    string(mcp.TypeInit),
		Version: "0.1.0",
		Tools:   toolManager.GetTools(),
		Data: map[string]any{
			"scenes": result,
		},
	}

	return response, nil
}

func (s *StdioServer) handleToolCall(msg jsonrpc.Request) (any, error) {
	var toolCall mcp.ToolCallMessage
	if err := json.Unmarshal(msg.Params, &toolCall); err != nil {
		logger.Error("Failed to unmarshal stdio tool call", "error", err)
		return nil, fmt.Errorf("invalid tool call payload: %w", err)
	}

	logger.Debug("Calling tool via stdio", "tool", toolCall.Tool, "arguments", toolCall.Arguments)
	result, err := s.toolManager.CallTool(toolCall.Tool, toolCall.Arguments)
	if err != nil {
		logger.Error("Stdio tool call failed", "tool", toolCall.Tool, "error", err)
		return &mcp.ErrorMessage{
			Type:    mcp.TypeError,
			Message: err.Error(),
			Code:    jsonrpc.ErrInternalError,
		}, nil
	}

	logger.Debug("Stdio tool call successful", "tool", toolCall.Tool, "result", result)
	return &mcp.ResultMessage{
		Type:   string(mcp.TypeResult),
		Tool:   toolCall.Tool,
		Result: result,
	}, nil
}

func (s *StdioServer) handlePing() (*mcp.PongMessage, error) {
	return &mcp.PongMessage{
		Type: string(mcp.TypePong),
	}, nil
}
