package stdio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20251125"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/tools"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

// StdioServer handles MCP communication over stdio
type StdioServer struct {
	toolManager         *tools.Manager
	promptCatalog       *promptcatalog.Registry
	promptRenderOptions shared.PromptRenderOptions
	toolCallOptions     shared.ToolCallOptions
	initializeAccepted  bool
	initialized         bool
}

// NewStdioServer creates a new stdio server
func NewStdioServer(toolManager *tools.Manager) *StdioServer {
	return &StdioServer{
		toolManager:         toolManager,
		promptRenderOptions: shared.DefaultPromptRenderOptions(),
		toolCallOptions:     shared.DefaultToolCallOptions(),
	}
}

// AttachPromptCatalog injects prompt metadata into stdio transport.
func (s *StdioServer) AttachPromptCatalog(registry *promptcatalog.Registry) {
	s.promptCatalog = registry
}

// AttachPromptRenderOptions applies runtime prompt rendering validation settings.
func (s *StdioServer) AttachPromptRenderOptions(options shared.PromptRenderOptions) {
	s.promptRenderOptions = options
}

// AttachToolCallOptions applies runtime tool call controls.
func (s *StdioServer) AttachToolCallOptions(options shared.ToolCallOptions) {
	s.toolCallOptions = options
}

// Start starts the stdio server
func (s *StdioServer) Start() error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	logger.Debug("Stdio server started and waiting for messages")

	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				logger.Debug("Stdio EOF received, terminating server")
				return nil
			}
			logger.Error("Error decoding message", "error", err)
			if encodeErr := encoder.Encode(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil)); encodeErr != nil {
				logger.Error("Error encoding parse error response", "error", encodeErr)
			}
			continue
		}

		requests, prebuiltResponses, acceptedOneWay, parseErr := shared.ParseJSONRPCFrame(raw)
		if parseErr != nil {
			logger.Error("Error parsing stdio message", "error", parseErr)
			if encodeErr := encoder.Encode(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil)); encodeErr != nil {
				logger.Error("Error encoding parse error response", "error", encodeErr)
			}
			continue
		}

		if len(requests) == 0 && len(prebuiltResponses) == 0 && !acceptedOneWay {
			if err := encoder.Encode(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)); err != nil {
				logger.Error("Error encoding response", "error", err)
			}
			continue
		}

		responses := make([]any, 0, len(requests)+len(prebuiltResponses))
		responses = append(responses, prebuiltResponses...)

		for _, msg := range requests {
			logger.Debug("Stdio message received", "method", msg.Method, "id", msg.ID)

			response, err := s.handleMessage(msg)
			if err != nil {
				logger.Error("Error handling message", "error", err, "method", msg.Method)
				if msg.ID != nil {
					responses = append(responses, jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInternalError), "Internal error", nil))
				}
				continue
			}

			if msg.ID == nil || response == nil {
				continue
			}
			responses = append(responses, response)
		}

		if len(requests) == 0 && len(prebuiltResponses) > 0 {
			if err := encoder.Encode(prebuiltResponses[0]); err != nil {
				logger.Error("Error encoding response", "error", err)
			}
			continue
		}

		if len(responses) == 0 {
			continue
		}

		if err := encoder.Encode(responses[0]); err != nil {
			logger.Error("Error encoding response", "error", err)
			continue
		}
		logger.Debug("Stdio response sent", "type", getResponseType(responses[0]))
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
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil), nil
		}
		if !s.initializeAccepted {
			return jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Initialize has not completed for this session", nil), nil
		}
		logger.Debug("Handling initialized notification")
		s.initialized = true
		return nil, nil
	case "notifications/initialized":
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil), nil
		}
		if !s.initializeAccepted {
			return jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Initialize has not completed for this session", nil), nil
		}
		logger.Debug("Handling notifications/initialized notification")
		s.initialized = true
		return nil, nil
	default:
		logger.Debug("Handling standard/unknown stdio message", "method", msg.Method)
		if !s.initializeAccepted || !s.initialized {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Session is not initialized", nil), nil
		}
		return shared.DispatchStandardMethodWithOptions(msg, s.toolManager, s.promptCatalog, readGodotResource, s.promptRenderOptions, s.toolCallOptions), nil
	}
}

func (s *StdioServer) handleInit(msg jsonrpc.Request) (*jsonrpc.Response, error) {
	if err := mcpv20251125.ValidateInitializeProtocolVersion(msg.Params); err != nil {
		if err == mcpv20251125.ErrMissingProtocolVersion {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), mcpv20251125.ErrMissingProtocolVersion.Error(), nil), nil
		}
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), mcpv20251125.ErrInvalidProtocolVersion.Error(), nil), nil
	}
	s.initializeAccepted = true
	s.initialized = false

	response := map[string]any{
		"type":            string(mcp.TypeInit),
		"version":         "0.1.0",
		"server_id":       "default",
		"tools":           s.toolManager.GetTools(),
		"protocolVersion": mcpv20251125.ProtocolVersion,
		"capabilities":    shared.ServerCapabilities(s.promptCatalog != nil && s.promptCatalog.Enabled(), false),
		"serverInfo": map[string]any{
			"name":    "godot-mcp-go",
			"version": "0.1.0",
		},
	}

	return jsonrpc.NewResponse(msg.ID, response), nil
}

func readGodotResource(path string) (any, error) {
	switch path {
	case "godot://script/current":
		return map[string]any{"type": "script", "path": "current"}, nil
	case "godot://scene/current":
		return map[string]any{"type": "scene", "path": "current"}, nil
	case "godot://project/info":
		return map[string]any{"name": "godot-mcp", "version": "0.1.0", "type": "godot"}, nil
	case "godot://policy/godot-checks":
		return map[string]any{"policy": "policy-godot", "checks": promptcatalog.GodotPolicyChecks()}, nil
	case "godot://runtime/metrics":
		return runtimebridge.HealthSnapshot(time.Now().UTC()), nil
	default:
		return nil, fmt.Errorf("unknown resource path: %s", path)
	}
}
