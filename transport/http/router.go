package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

const maxJSONRPCBodyBytes = 1 << 20

const (
	headerSessionID       = "MCP-Session-Id"
	headerProtocolVersion = "MCP-Protocol-Version"
	legacyProtocolVersion = "2025-03-26"
)

var supportedProtocolVersions = map[string]struct{}{
	"2024-11-05":          {},
	legacyProtocolVersion: {},
	"2025-06-18":          {},
	"2025-11-25":          {},
	"2025-06-14":          {}, // legacy compatibility for older clients.
}

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

	limitedBody := http.MaxBytesReader(c.Response(), c.Request().Body, maxJSONRPCBodyBytes)
	defer limitedBody.Close()

	body, err := io.ReadAll(limitedBody)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			logger.Warn("Request body too large", "limit_bytes", maxJSONRPCBodyBytes, "remote_addr", c.RealIP())
			return c.JSON(http.StatusRequestEntityTooLarge, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Request body too large", nil))
		}
		logger.Error("Failed to read request body", "error", err)
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}

	requests, prebuiltResponses, acceptedOneWay, err := shared.ParseJSONRPCFrame(body)
	if err != nil {
		logger.Error("Failed to parse JSON-RPC request", "error", err)
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}
	if len(requests) == 0 && len(prebuiltResponses) == 0 && !acceptedOneWay {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
	}

	sessionID := c.Request().Header.Get(headerSessionID)
	requestedProtocolVersion := strings.TrimSpace(c.Request().Header.Get(headerProtocolVersion))
	if requestedProtocolVersion != "" && !isSupportedProtocolVersion(requestedProtocolVersion) {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unsupported MCP-Protocol-Version header", nil))
	}

	hasInitialize := false
	hasNonInitialize := false
	for _, req := range requests {
		if req.Method == "initialize" {
			hasInitialize = true
		} else {
			hasNonInitialize = true
		}
	}

	if len(requests) > 0 {
		if hasInitialize {
			if sessionID == "" {
				sessionID, err = generateSessionID()
				if err != nil {
					logger.Error("Failed to generate session ID", "error", err)
					return c.JSON(http.StatusInternalServerError, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInternalError), "Internal error", nil))
				}
				s.sessionManager.CreateSession(sessionID)
				logger.Debug("Generated new MCP session")
			} else if !s.sessionManager.TouchSession(sessionID) {
				return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
			}
		}

		if !hasInitialize || hasNonInitialize {
			if sessionID == "" {
				return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing MCP-Session-Id header", nil))
			}
			if !s.sessionManager.TouchSession(sessionID) {
				return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
			}
		}
	}
	if acceptedOneWay {
		if sessionID == "" {
			return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing MCP-Session-Id header", nil))
		}
	}
	if sessionID != "" && (len(requests) == 0 || acceptedOneWay) {
		if !s.sessionManager.TouchSession(sessionID) {
			return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
		}
	}

	requireProtocolHeader := hasNonInitialize || acceptedOneWay
	if !s.isProtocolVersionAccepted(sessionID, requestedProtocolVersion, requireProtocolHeader) {
		if requestedProtocolVersion == "" && requireProtocolHeader {
			return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing MCP-Protocol-Version header", nil))
		}
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid MCP-Protocol-Version header", nil))
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
		c.Response().Header().Set(headerSessionID, sessionID)
	}

	if len(requests) == 0 && len(prebuiltResponses) > 0 {
		return c.JSON(http.StatusBadRequest, prebuiltResponses[0])
	}

	if len(responses) == 0 {
		return c.NoContent(http.StatusAccepted)
	}
	return c.JSON(http.StatusOK, responses[0])
}

func (s *Server) handleStreamableHTTPGet(c echo.Context) error {
	logger.Info("Streamable HTTP GET request is not supported", "remote_addr", c.RealIP())
	return c.JSON(http.StatusMethodNotAllowed, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "SSE stream via GET is not supported; use POST /mcp", nil))
}

func (s *Server) handleStreamableHTTPDelete(c echo.Context) error {
	logger.Info("Streamable HTTP DELETE request", "remote_addr", c.RealIP())
	sessionID := c.Request().Header.Get(headerSessionID)
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing MCP-Session-Id header", nil))
	}
	if !s.sessionManager.HasSession(sessionID) {
		return c.JSON(http.StatusNotFound, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Unknown MCP session", nil))
	}
	requestedProtocolVersion := strings.TrimSpace(c.Request().Header.Get(headerProtocolVersion))
	if !s.isProtocolVersionAccepted(sessionID, requestedProtocolVersion, true) {
		if requestedProtocolVersion == "" {
			return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Missing MCP-Protocol-Version header", nil))
		}
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid MCP-Protocol-Version header", nil))
	}
	s.sessionManager.RemoveSession(sessionID)
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) handleMessage(msg jsonrpc.Request, sessionID string) (any, error) {
	switch msg.Method {
	case "initialize":
		logger.Debug("Handling initialize message", "request_id", msg.ID)
		return s.handleInit(msg, sessionID)
	case "initialized", "notifications/initialized":
		if msg.ID != nil {
			return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil), nil
		}
		logger.Debug("Handling initialized notification")
		if sessionID != "" {
			s.sessionManager.MarkInitialized(sessionID)
		}
		return nil, nil
	default:
		logger.Debug("Handling standard/unknown message", "method", msg.Method)
		return shared.DispatchStandardMethod(msg, s.toolManager, s.handleGodotResource), nil
	}
}

func (s *Server) handleInit(msg jsonrpc.Request, sessionID string) (*jsonrpc.Response, error) {
	logger.Debug("Handling init message", "request_id", msg.ID)
	if sessionID != "" {
		s.sessionManager.CreateSession(sessionID)
	}

	tools, err := s.registry.GetServerTools("default")
	if err != nil {
		logger.Error("Failed to get server tools", "error", err, "server_id", "default")
		return nil, err
	}

	negotiatedVersion := negotiateProtocolVersion(msg.Params)
	if sessionID != "" {
		s.sessionManager.SetProtocolVersion(sessionID, negotiatedVersion)
	}
	result := map[string]any{
		"type":            string(mcp.TypeInit),
		"version":         "0.1.0",
		"server_id":       "default",
		"tools":           tools,
		"protocolVersion": negotiatedVersion,
		"capabilities":    shared.ServerCapabilities(),
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

func generateSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to read cryptographic random bytes: %w", err)
	}
	return "session_" + hex.EncodeToString(buf), nil
}

func (s *Server) isProtocolVersionAccepted(sessionID string, requestedVersion string, requireHeader bool) bool {
	if requestedVersion != "" {
		if !isSupportedProtocolVersion(requestedVersion) {
			return false
		}

		if sessionID != "" {
			if negotiatedVersion, ok := s.sessionManager.GetProtocolVersion(sessionID); ok && negotiatedVersion != "" && negotiatedVersion != requestedVersion {
				return false
			}
		}
		return true
	}

	return !requireHeader
}

func isSupportedProtocolVersion(version string) bool {
	if version == "" {
		return false
	}
	if version == mcp.ProtocolVersion {
		return true
	}
	_, ok := supportedProtocolVersions[version]
	return ok
}

func negotiateProtocolVersion(paramsRaw json.RawMessage) string {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	preferred := mcp.ProtocolVersion
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return preferred
	}

	if isSupportedProtocolVersion(params.ProtocolVersion) {
		return params.ProtocolVersion
	}
	return preferred
}
