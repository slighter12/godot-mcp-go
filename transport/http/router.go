package http

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

func RegisterRoutes(e *echo.Echo, s *Server) {
	e.GET("/", s.handleHTTPInfo)
	e.POST("/mcp", s.handleStreamableHTTPPost)
	e.GET("/mcp", s.handleStreamableHTTPGet)
	e.DELETE("/mcp", s.handleStreamableHTTPDelete)
	e.OPTIONS("/mcp", s.handleOptions)
}

func (s *Server) handleHTTPInfo(c echo.Context) error {
	logger.Debug("HTTP info requested", "remote_addr", c.RealIP())
	info := map[string]any{
		"version": "0.1.0",
		"type":    "godot-mcp",
		"capabilities": map[string]any{
			"stdio":           true,
			"streamable_http": true,
		},
		"streamable_http_endpoint": "/mcp",
	}
	return c.JSON(http.StatusOK, info)
}

func (s *Server) handleOptions(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

func (s *Server) handleStreamableHTTPPost(c echo.Context) error {
	logger.Info("Streamable HTTP POST request", "remote_addr", c.RealIP())

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}

	requests, prebuiltResponses, isBatch, err := parseJSONRPCRequests(body)
	if err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}
	if len(requests) == 0 && len(prebuiltResponses) == 0 {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
	}

	sessionID := c.Request().Header.Get("Mcp-Session-Id")
	if len(requests) > 0 {
		hasInitialize := false
		hasNonInitialize := false
		for _, req := range requests {
			if req.Method == "initialize" {
				hasInitialize = true
			} else {
				hasNonInitialize = true
			}
		}

		if hasInitialize {
			if sessionID == "" {
				sessionID = generateSessionID()
				s.sessionManager.CreateSession(sessionID)
				logger.Debug("Generated new session ID", "session_id", sessionID)
			} else if !s.sessionManager.TouchSession(sessionID) {
				return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
			}
		}

		if !hasInitialize || hasNonInitialize {
			if sessionID == "" {
				return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing Mcp-Session-Id header", nil))
			}
			if !s.sessionManager.TouchSession(sessionID) {
				return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
			}
		}
	}

	responses := make([]any, 0, len(requests)+len(prebuiltResponses))
	responses = append(responses, prebuiltResponses...)

	for _, request := range requests {
		logger.Debug("Streamable HTTP request received", "method", request.Method, "id", request.ID)
		response, handleErr := s.handleMessage(request, sessionID)
		if handleErr != nil {
			logger.Error("Error handling message", "error", handleErr, "method", request.Method)
			if request.ID != nil {
				responses = append(responses, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "Internal error", nil))
			}
			continue
		}
		if request.ID == nil || response == nil {
			continue
		}
		responses = append(responses, response)
	}

	if sessionID != "" {
		c.Response().Header().Set("Mcp-Session-Id", sessionID)
	}

	if len(requests) == 0 && len(prebuiltResponses) > 0 {
		if isBatch {
			return c.JSON(http.StatusBadRequest, prebuiltResponses)
		}
		return c.JSON(http.StatusBadRequest, prebuiltResponses[0])
	}

	if len(responses) == 0 {
		return c.NoContent(http.StatusAccepted)
	}
	if isBatch {
		return c.JSON(http.StatusOK, responses)
	}
	return c.JSON(http.StatusOK, responses[0])
}

func (s *Server) handleStreamableHTTPGet(c echo.Context) error {
	logger.Info("Streamable HTTP GET request (SSE stream)", "remote_addr", c.RealIP())
	sessionID := c.Request().Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing Mcp-Session-Id header", nil))
	}
	if !s.sessionManager.TouchSession(sessionID) {
		return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Mcp-Session-Id, Last-Event-ID")

	transport := NewStreamableHTTPTransport(c.Response().Writer, c.Response().Writer.(http.Flusher))
	s.sessionManager.SetTransport(sessionID, transport)

	if err := transport.SendSSE("heartbeat", map[string]string{"status": "connected"}); err != nil {
		logger.Error("Failed to send initial heartbeat", "error", err)
		return err
	}

	go s.sendHeartbeats(sessionID, transport)
	<-c.Request().Context().Done()
	s.sessionManager.ClearTransport(sessionID)
	return nil
}

func (s *Server) handleStreamableHTTPDelete(c echo.Context) error {
	logger.Info("Streamable HTTP DELETE request", "remote_addr", c.RealIP())
	sessionID := c.Request().Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing Mcp-Session-Id header", nil))
	}
	if !s.sessionManager.HasSession(sessionID) {
		return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
	}
	s.sessionManager.RemoveSession(sessionID)
	return c.NoContent(http.StatusNoContent)
}

func parseJSONRPCRequests(body []byte) ([]jsonrpc.Request, []any, bool, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil, false, fmt.Errorf("empty request body")
	}

	isBatch := trimmed[0] == '['
	rawMessages := make([]json.RawMessage, 0)

	if isBatch {
		if err := json.Unmarshal(trimmed, &rawMessages); err != nil {
			return nil, nil, true, err
		}
		if len(rawMessages) == 0 {
			return nil, []any{jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil)}, true, nil
		}
	} else {
		rawMessages = append(rawMessages, json.RawMessage(trimmed))
	}

	requests := make([]jsonrpc.Request, 0, len(rawMessages))
	errors := make([]any, 0)

	for _, raw := range rawMessages {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			errors = append(errors, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		var msg jsonrpc.Request
		if err := json.Unmarshal(raw, &msg); err != nil {
			errors = append(errors, jsonrpc.NewErrorResponse(rawIDFromEnvelope(envelope), int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		if msg.Method == "" {
			_, hasResult := envelope["result"]
			_, hasErr := envelope["error"]
			if hasResult || hasErr {
				// Client response payloads are accepted as one-way messages.
				continue
			}
			errors = append(errors, jsonrpc.NewErrorResponse(rawIDFromEnvelope(envelope), int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		requests = append(requests, msg)
	}

	return requests, errors, isBatch, nil
}

func rawIDFromEnvelope(envelope map[string]json.RawMessage) any {
	rawID, exists := envelope["id"]
	if !exists || len(rawID) == 0 {
		return nil
	}
	var id any
	if err := json.Unmarshal(rawID, &id); err != nil {
		return nil
	}
	return id
}

func (s *Server) handleMessage(msg jsonrpc.Request, sessionID string) (any, error) {
	switch msg.Method {
	case "initialize":
		logger.Debug("Handling initialize message", "request_id", msg.ID)
		return s.handleInit(msg, sessionID)
	case "initialized", "notifications/initialized":
		logger.Debug("Handling initialized notification")
		if sessionID != "" {
			s.sessionManager.MarkInitialized(sessionID)
		}
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
		logger.Debug("Handling tools/call message")
		return s.handleToolCall(msg), nil
	case "ping":
		logger.Debug("Handling ping message")
		return s.handlePing(msg), nil
	case "tools/progress":
		logger.Debug("Handling tools/progress notification")
		return nil, nil
	default:
		logger.Debug("Received unknown message type", "method", msg.Method)
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{"method": msg.Method}), nil
		}
		return nil, nil
	}
}

func (s *Server) handleInit(msg jsonrpc.Request, sessionID string) (*jsonrpc.Response, error) {
	logger.Debug("Handling init message", "request_id", msg.ID, "session_id", sessionID)
	if sessionID != "" {
		s.sessionManager.CreateSession(sessionID)
	}

	tools, err := s.registry.GetServerTools("default")
	if err != nil {
		logger.Error("Failed to get server tools", "error", err, "server_id", "default")
		return nil, err
	}

	negotiatedVersion := negotiateProtocolVersion(msg.Params)
	result := map[string]any{
		"type":            string(mcp.TypeInit),
		"version":         "0.1.0",
		"server_id":       "default",
		"tools":           tools,
		"protocolVersion": negotiatedVersion,
		"capabilities":    serverCapabilities(),
		"serverInfo": map[string]any{
			"name":    "godot-mcp-go",
			"version": "0.1.0",
		},
	}
	if sessionID != "" {
		result["sessionId"] = sessionID
	}

	return jsonrpc.NewResponse(msg.ID, result), nil
}

func (s *Server) handleToolsList(msg jsonrpc.Request) *jsonrpc.Response {
	tools := s.toolManager.GetTools()
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	start, err := parseCursor(msg.Params, len(tools))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+50, len(tools))

	result := map[string]any{
		"tools": tools[start:end],
	}
	if end < len(tools) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func (s *Server) handleToolCall(msg jsonrpc.Request) *jsonrpc.Response {
	var toolCall struct {
		Name      string         `json:"name"`
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &toolCall); err != nil {
		logger.Error("Failed to unmarshal tool call", "error", err)
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
	}

	toolName := strings.TrimSpace(toolCall.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(toolCall.Tool)
	}
	if toolName == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Tool name is required", nil)
	}

	arguments := toolCall.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	logger.Debug("Calling tool", "tool", toolName, "arguments", arguments)
	if strings.HasPrefix(toolName, "godot://") {
		result, err := s.handleGodotResource(toolName)
		if err != nil {
			logger.Error("Resource read failed", "resource", toolName, "error", err)
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(msg.ID, buildToolSuccessResult(toolName, result))
	}

	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		logger.Error("Failed to marshal tool arguments", "error", err)
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Failed to marshal tool arguments", nil)
	}

	resultJSON, err := s.toolManager.ExecuteTool(toolName, argsJSON)
	if err != nil {
		logger.Error("Tool execution failed", "tool", toolName, "error", err)
		if strings.HasPrefix(err.Error(), "tool not found:") {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
		}
		return jsonrpc.NewResponse(msg.ID, map[string]any{
			"type":    string(mcp.TypeResult),
			"tool":    toolName,
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
	}

	var result any
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		logger.Error("Failed to unmarshal tool result", "error", err)
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInternalError), "Failed to unmarshal tool result", nil)
	}

	return jsonrpc.NewResponse(msg.ID, buildToolSuccessResult(toolName, result))
}

func (s *Server) handleResourcesList(msg jsonrpc.Request) *jsonrpc.Response {
	resources := []map[string]any{
		{
			"uri":      "godot://project/info",
			"name":     "Project Info",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://scene/current",
			"name":     "Current Scene",
			"mimeType": "application/json",
		},
		{
			"uri":      "godot://script/current",
			"name":     "Current Script",
			"mimeType": "application/json",
		},
	}

	start, err := parseCursor(msg.Params, len(resources))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	end := min(start+50, len(resources))

	result := map[string]any{
		"resources": resources[start:end],
	}
	if end < len(resources) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return jsonrpc.NewResponse(msg.ID, result)
}

func (s *Server) handleResourcesRead(msg jsonrpc.Request) *jsonrpc.Response {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid resources/read payload", nil)
	}
	if params.URI == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Resource URI is required", nil)
	}

	result, err := s.handleGodotResource(params.URI)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInternalError), "Failed to encode resource result", nil)
	}

	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"contents": []map[string]any{
			{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     string(resultJSON),
			},
		},
	})
}

func (s *Server) handlePromptsList(msg jsonrpc.Request) *jsonrpc.Response {
	prompts := []map[string]any{}
	start, err := parseCursor(msg.Params, len(prompts))
	if err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil)
	}
	if start != 0 {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid cursor value", nil)
	}
	return jsonrpc.NewResponse(msg.ID, map[string]any{
		"prompts": prompts,
	})
}

func (s *Server) handlePromptsGet(msg jsonrpc.Request) *jsonrpc.Response {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Invalid prompts/get payload", nil)
	}
	if params.Name == "" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidParams), "Prompt name is required", nil)
	}
	return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Prompt not found", map[string]any{
		"name": params.Name,
	})
}

func (s *Server) handlePing(msg jsonrpc.Request) *jsonrpc.Response {
	return jsonrpc.NewResponse(msg.ID, &mcp.PongMessage{
		Type: string(mcp.TypePong),
	})
}

func (s *Server) handleGodotResource(path string) (any, error) {
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

func buildToolSuccessResult(toolName string, result any) map[string]any {
	return map[string]any{
		"type":              string(mcp.TypeResult),
		"tool":              toolName,
		"result":            result,
		"content":           toolContentFromResult(result),
		"structuredContent": result,
		"isError":           false,
	}
}

func toolContentFromResult(result any) []map[string]any {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return []map[string]any{{"type": "text", "text": "tool call completed"}}
	}
	return []map[string]any{{"type": "text", "text": string(resultJSON)}}
}

func serverCapabilities() map[string]any {
	return map[string]any{
		"tools":     map[string]any{},
		"resources": map[string]any{},
		"prompts":   map[string]any{},
	}
}

func parseCursor(paramsRaw json.RawMessage, total int) (int, error) {
	if len(paramsRaw) == 0 {
		return 0, nil
	}

	var params struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return 0, fmt.Errorf("invalid params payload")
	}
	if strings.TrimSpace(params.Cursor) == "" {
		return 0, nil
	}

	offset, err := strconv.Atoi(params.Cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor value")
	}
	if offset < 0 || offset > total {
		return 0, fmt.Errorf("invalid cursor value")
	}
	return offset, nil
}

func (s *Server) sendHeartbeats(sessionID string, transport *StreamableHTTPTransport) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if transport.IsClosed() {
			return
		}
		if err := transport.SendSSE("heartbeat", map[string]string{"timestamp": time.Now().Format(time.RFC3339)}); err != nil {
			logger.Error("Failed to send heartbeat", "error", err, "session_id", sessionID)
			return
		}
	}
}

func generateSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("session_%d", time.Now().UnixNano())
	}
	return "session_" + hex.EncodeToString(buf)
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
