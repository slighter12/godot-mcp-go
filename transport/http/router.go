package http

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	var request jsonrpc.Request
	if err := c.Bind(&request); err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON-RPC request"})
	}
	logger.Debug("Streamable HTTP request received", "method", request.Method, "id", request.ID)
	sessionID := c.Request().Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = generateSessionID()
		logger.Debug("Generated new session ID", "session_id", sessionID)
	}
	response, err := s.handleMessage(request)
	if err != nil {
		logger.Error("Error handling message", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if s.shouldStartStreaming(request) {
		return s.handleStreamingResponse(c, request, sessionID)
	}
	c.Response().Header().Set("Mcp-Session-Id", sessionID)
	if response != nil {
		return c.JSON(http.StatusOK, response)
	}
	return c.NoContent(http.StatusAccepted)
}

func (s *Server) handleStreamableHTTPGet(c echo.Context) error {
	logger.Info("Streamable HTTP GET request (SSE stream)", "remote_addr", c.RealIP())
	sessionID := c.Request().Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing Mcp-Session-Id header"})
	}
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Mcp-Session-Id, Last-Event-ID")
	transport := NewStreamableHTTPTransport(c.Response().Writer, c.Response().Writer.(http.Flusher))
	s.sessionManager.CreateSession(sessionID, transport)
	if err := transport.SendSSE("heartbeat", map[string]string{"status": "connected"}); err != nil {
		logger.Error("Failed to send initial heartbeat", "error", err)
		return err
	}
	go s.sendHeartbeats(sessionID, transport)
	<-c.Request().Context().Done()
	s.sessionManager.RemoveSession(sessionID)
	return nil
}

func (s *Server) handleMessage(msg jsonrpc.Request) (any, error) {
	switch msg.Method {
	case "initialize":
		logger.Debug("Handling init message", "client_id", msg.ID)
		return s.handleInit(msg)
	case "tools/call":
		clientID := string(msg.ID)
		if !s.registry.IsClientInitialized(clientID) {
			return &mcp.ErrorMessage{
				Type:    mcp.TypeError,
				Message: "client not initialized",
				Code:    jsonrpc.ErrInvalidRequest,
			}, nil
		}
		var toolCall mcp.ToolCallMessage
		if err := json.Unmarshal(msg.Params, &toolCall); err != nil {
			logger.Error("Failed to unmarshal tool call", "error", err)
			return nil, err
		}
		logger.Debug("Calling tool", "tool", toolCall.Tool, "arguments", toolCall.Arguments)
		return s.handleToolCall(toolCall)
	case "ping":
		logger.Debug("Handling ping message")
		return s.handlePing()
	case "tools/progress":
		logger.Debug("Handling tool progress message")
		return nil, nil
	default:
		logger.Debug("Received unknown message type", "method", msg.Method)
		return nil, nil
	}
}

func (s *Server) handleInit(msg jsonrpc.Request) (*mcp.InitMessage, error) {
	logger.Debug("Handling init message", "client_id", msg.ID)
	clientID := string(msg.ID)
	if clientID != "" {
		if err := s.registry.RegisterClient(clientID, "default"); err != nil {
			logger.Error("Failed to register client", "error", err, "client_id", clientID)
			return nil, err
		}
		logger.Debug("Client registered successfully", "client_id", clientID)
	}
	tools, err := s.registry.GetServerTools("default")
	if err != nil {
		logger.Error("Failed to get server tools", "error", err, "server_id", "default")
		return nil, err
	}
	if clientID != "" {
		if err := s.registry.InitializeClient(clientID); err != nil {
			logger.Error("Failed to initialize client", "error", err, "client_id", clientID)
			return nil, err
		}
		logger.Debug("Client initialized successfully", "client_id", clientID)
	}
	return &mcp.InitMessage{
		Type:     string(mcp.TypeInit),
		Version:  "0.1.0",
		ClientID: clientID,
		ServerID: "default",
		Tools:    tools,
	}, nil
}

func (s *Server) handleToolCall(toolCall mcp.ToolCallMessage) (any, error) {
	logger.Debug("Calling tool", "tool", toolCall.Tool, "arguments", toolCall.Arguments)
	if strings.HasPrefix(toolCall.Tool, "godot://") {
		return s.handleGodotResource(toolCall.Tool)
	}
	argsJSON, err := json.Marshal(toolCall.Arguments)
	if err != nil {
		logger.Error("Failed to marshal tool arguments", "error", err)
		return &mcp.ErrorMessage{
			Type:    mcp.TypeError,
			Message: "Failed to marshal tool arguments",
			Code:    jsonrpc.ErrInvalidParams,
		}, nil
	}
	resultJSON, err := s.toolManager.ExecuteTool(toolCall.Tool, argsJSON)
	if err != nil {
		logger.Error("Tool execution failed", "tool", toolCall.Tool, "error", err)
		return &mcp.ErrorMessage{
			Type:    mcp.TypeError,
			Message: err.Error(),
			Code:    jsonrpc.ErrInternalError,
		}, nil
	}
	var result any
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		logger.Error("Failed to unmarshal tool result", "error", err)
		return &mcp.ErrorMessage{
			Type:    mcp.TypeError,
			Message: "Failed to unmarshal tool result",
			Code:    jsonrpc.ErrInternalError,
		}, nil
	}
	return &mcp.ResultMessage{
		Type:   string(mcp.TypeResult),
		Tool:   toolCall.Tool,
		Result: result,
	}, nil
}

func (s *Server) handlePing() (*mcp.PongMessage, error) {
	return &mcp.PongMessage{
		Type: string(mcp.TypePong),
	}, nil
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

func (s *Server) shouldStartStreaming(request jsonrpc.Request) bool {
	return false
}

func (s *Server) handleStreamingResponse(c echo.Context, request jsonrpc.Request, sessionID string) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Mcp-Session-Id", sessionID)
	transport := NewStreamableHTTPTransport(c.Response().Writer, c.Response().Writer.(http.Flusher))
	s.sessionManager.CreateSession(sessionID, transport)
	response, err := s.handleMessage(request)
	if err != nil {
		return err
	}
	if response != nil {
		return transport.Send(response)
	}
	return nil
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
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}
