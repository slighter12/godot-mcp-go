package stdio

import (
	"bytes"
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

		requests, prebuiltResponses, acceptedOneWay, parseErr := parseJSONRPCMessages(raw)
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
		logger.Debug("Handling initialized notification")
		return nil, nil
	case "notifications/initialized":
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil), nil
		}
		logger.Debug("Handling notifications/initialized notification")
		return nil, nil
	default:
		logger.Debug("Handling standard/unknown stdio message", "method", msg.Method)
		return shared.DispatchStandardMethod(msg, s.toolManager, readGodotResource), nil
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
	preferred := mcp.ProtocolVersion
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return preferred
	}

	supported := map[string]struct{}{
		"2024-11-05": {},
		"2025-03-26": {},
		"2025-06-18": {},
		"2025-11-25": {},
		"2025-06-14": {}, // legacy compatibility for older clients.
		preferred:    {},
	}
	if _, ok := supported[params.ProtocolVersion]; ok {
		return params.ProtocolVersion
	}
	return preferred
}

func parseJSONRPCMessages(raw json.RawMessage) ([]jsonrpc.Request, []any, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, false, fmt.Errorf("empty message")
	}

	// stdio transport processes one JSON-RPC message per frame.
	if trimmed[0] == '[' {
		return nil, []any{jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)}, false, nil
	}
	rawMessages := []json.RawMessage{json.RawMessage(trimmed)}

	requests := make([]jsonrpc.Request, 0, len(rawMessages))
	errors := make([]any, 0)
	acceptedOneWay := false
	for _, rawMsg := range rawMessages {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &envelope); err != nil {
			errors = append(errors, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requestID, hasID, validID := parseIDFromEnvelope(envelope)
		if !validID {
			errors = append(errors, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		var msg jsonrpc.Request
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			errors = append(errors, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "" {
			_, hasResult := envelope["result"]
			_, hasErr := envelope["error"]
			if hasResult || hasErr {
				if msg.JSONRPC != jsonrpc.Version || !hasID || (hasResult && hasErr) {
					errors = append(errors, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
				} else {
					acceptedOneWay = true
				}
				continue
			}
			errors = append(errors, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.JSONRPC != jsonrpc.Version {
			errors = append(errors, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if rawParams, ok := envelope["params"]; ok && !isValidParamsValue(rawParams) {
			errors = append(errors, jsonrpc.NewErrorResponse(requestID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "initialize" && msg.ID == nil {
			errors = append(errors, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requests = append(requests, msg)
	}

	return requests, errors, acceptedOneWay, nil
}

func parseIDFromEnvelope(envelope map[string]json.RawMessage) (any, bool, bool) {
	rawID, exists := envelope["id"]
	if !exists {
		return nil, false, true
	}
	trimmed := bytes.TrimSpace(rawID)
	if len(trimmed) == 0 {
		return nil, true, false
	}

	var id any
	if err := json.Unmarshal(trimmed, &id); err != nil {
		return nil, true, false
	}
	if !isValidJSONRPCID(id) {
		return nil, true, false
	}
	return id, true, true
}

func isValidJSONRPCID(id any) bool {
	switch id.(type) {
	case nil, string, float64, int, int64, int32, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func isValidParamsValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}
