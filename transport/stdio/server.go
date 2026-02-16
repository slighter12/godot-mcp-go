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
	"github.com/slighter12/godot-mcp-go/transport/shared"
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
	case *jsonrpc.Response:
		if r.Error != nil {
			return "error"
		}
		return "result"
	default:
		return "unknown"
	}
}

func (s *StdioServer) handleMessage(msg jsonrpc.Request) (any, error) {
	switch msg.Method {
	case "initialize":
		logger.Debug("Handling init message")
		return s.handleInit(msg)
	case "initialized":
		logger.Debug("Handling initialized notification")
		return nil, nil
	case "notifications/initialized":
		logger.Debug("Handling notifications/initialized notification")
		return nil, nil
	case "tools/list":
		logger.Debug("Handling tools/list message")
		return s.handleToolsList(msg), nil
	case "resources/list":
		logger.Debug("Handling resources/list message")
		return s.handleResourcesList(msg), nil
	case "resources/read":
		logger.Debug("Handling resources/read message")
		return s.handleResourcesRead(msg), nil
	case "prompts/list":
		logger.Debug("Handling prompts/list message")
		return s.handlePromptsList(msg), nil
	case "prompts/get":
		logger.Debug("Handling prompts/get message")
		return s.handlePromptsGet(msg), nil
	case "tools/call":
		logger.Debug("Handling tool call message")
		return s.handleToolCall(msg)
	case "ping":
		logger.Debug("Handling ping message")
		return s.handlePing(msg), nil
	case "tools/progress":
		logger.Debug("Handling tool progress message")
		return nil, nil // Progress messages are one-way
	default:
		logger.Debug("Received unknown message type", "method", msg.Method)
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{
				"method": msg.Method,
			}), nil
		}
		return nil, fmt.Errorf("unknown message type: %s", msg.Method)
	}
}

func (s *StdioServer) handleInit(msg jsonrpc.Request) (*jsonrpc.Response, error) {
	// Call the list scenes tool to include basic bootstrap context in legacy field.
	result, err := s.toolManager.CallTool("list-project-scenes", map[string]any{})
	if err != nil {
		return nil, err
	}

	response := map[string]any{
		"type":            string(mcp.TypeInit),
		"version":         "0.1.0",
		"server_id":       "default",
		"tools":           s.toolManager.GetTools(),
		"protocolVersion": negotiateProtocolVersion(msg.Params),
		"capabilities":    shared.ServerCapabilities(),
		"serverInfo": map[string]any{
			"name":    "godot-mcp-go",
			"version": "0.1.0",
		},
		"data": map[string]any{
			"scenes": result,
		},
	}

	return jsonrpc.NewResponse(msg.ID, response), nil
}

func (s *StdioServer) handleToolsList(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildToolsListResponse(msg, s.toolManager.GetTools())
}

func (s *StdioServer) handleToolCall(msg jsonrpc.Request) (any, error) {
	var toolCall struct {
		Name      string         `json:"name"`
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &toolCall); err != nil {
		logger.Error("Failed to unmarshal stdio tool call", "error", err)
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil), nil
	}

	toolName := toolCall.Name
	if toolName == "" {
		toolName = toolCall.Tool
	}
	if toolName == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Tool name is required", nil), nil
	}

	logger.Debug("Calling tool via stdio", "tool", toolName, "arguments", toolCall.Arguments)
	result, err := s.toolManager.CallTool(toolName, toolCall.Arguments)
	if err != nil {
		logger.Error("Stdio tool call failed", "tool", toolName, "error", err)
		if tools.IsToolNotFound(err) {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil), nil
		}
		return jsonrpc.NewResponse(msg.ID, map[string]any{
			"type":    string(mcp.TypeResult),
			"tool":    toolName,
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}), nil
	}

	logger.Debug("Stdio tool call successful", "tool", toolName, "result", result)
	return jsonrpc.NewResponse(msg.ID, shared.BuildToolSuccessResult(toolName, result)), nil
}

func (s *StdioServer) handleResourcesList(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildResourcesListResponse(msg)
}

func (s *StdioServer) handleResourcesRead(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildResourcesReadResponse(msg, readGodotResource)
}

func (s *StdioServer) handlePromptsList(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildPromptsListResponse(msg)
}

func (s *StdioServer) handlePromptsGet(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildPromptsGetResponse(msg)
}

func (s *StdioServer) handlePing(msg jsonrpc.Request) *jsonrpc.Response {
	return shared.BuildPingResponse(msg)
}

func readGodotResource(path string) (any, error) {
	switch path {
	case "godot://script/current":
		return map[string]any{"type": "script", "path": "current"}, nil
	case "godot://scene/current":
		return map[string]any{"type": "scene", "path": "current"}, nil
	case "godot://project/info":
		return map[string]any{"name": "godot-mcp", "version": "0.1.0", "type": "godot"}, nil
	default:
		return nil, fmt.Errorf("unknown resource path: %s", path)
	}
}

func negotiateProtocolVersion(paramsRaw json.RawMessage) string {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	preferred := "2025-03-26"
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return preferred
	}

	supported := map[string]struct{}{
		"2024-11-05":        {},
		preferred:           {},
		mcp.ProtocolVersion: {},
	}
	if _, ok := supported[params.ProtocolVersion]; ok {
		return params.ProtocolVersion
	}
	return preferred
}
