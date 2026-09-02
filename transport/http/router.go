package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slighter12/godot-mcp-go/internal/domain/toolspec"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

const subscriptionKeepAliveInterval = 15 * time.Second

const (
	headerProtocolVersion = "MCP-Protocol-Version"
	headerMethod          = "Mcp-Method"
	headerName            = "Mcp-Name"
)

// DispatchHook is retained as an alias for the transport-neutral shared seam.
type DispatchHook = shared.DispatchHook

// ProgressDispatchHook provides fixture-only progress frames for one request.
type ProgressDispatchHook func(context.Context, jsonrpc.Request, mcpv20260728.RequestMeta) ([]*jsonrpc.Notification, any, bool)

func RegisterRoutes(e *echo.Echo, s *Server) {
	e.GET("/", s.handleHTTPInfo)
	e.POST("/mcp", s.handleStreamableHTTPPost)
	e.GET("/mcp", s.handleStreamableHTTPRemoved)
	e.DELETE("/mcp", s.handleStreamableHTTPRemoved)
	e.OPTIONS("/mcp", s.handleOptions)
}

func (s *Server) handleHTTPInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"version": mcp.ServerVersion,
		"type":    "godot-mcp",
		"protocol": map[string]any{
			"version": mcpv20260728.ProtocolVersion,
		},
		"capabilities": map[string]any{
			"stdio":           true,
			"streamable_http": true,
		},
		"streamable_http_endpoint": "/mcp",
	})
}

func (s *Server) handleOptions(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

func (s *Server) handleStreamableHTTPRemoved(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{
		"method": c.Request().Method,
	}))
}

func (s *Server) handleStreamableHTTPPost(c echo.Context) error {
	limitedBody := http.MaxBytesReader(c.Response(), c.Request().Body, shared.MaxJSONRPCFrameBytes)
	defer limitedBody.Close()
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return c.JSON(http.StatusRequestEntityTooLarge, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Request body too large", nil))
		}
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}

	requests, prebuiltResponses, acceptedOneWay, err := shared.ParseJSONRPCFrame(body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
	}
	if len(prebuiltResponses) > 0 {
		return c.JSON(http.StatusBadRequest, prebuiltResponses[0])
	}
	if len(requests) != 1 {
		if acceptedOneWay {
			return c.NoContent(http.StatusAccepted)
		}
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
	}

	request := requests[0]
	meta, protocolResponse := s.validateModernHTTPRequest(c, request)
	if protocolResponse != nil {
		return c.JSON(http.StatusBadRequest, protocolResponse)
	}

	if request.Method == "subscriptions/listen" {
		return s.handleSubscription(c, request, meta)
	}
	if capabilityResponse := s.missingMutatingCapability(request, meta); capabilityResponse != nil {
		return c.JSON(http.StatusBadRequest, capabilityResponse)
	}
	if request.ID != nil && request.Method == "tools/call" && meta.ProgressToken != nil {
		return s.handleProgressToolCall(c, request, meta)
	}
	if request.ID == nil {
		if response, handleErr := s.dispatchModernMessage(c.Request().Context(), request, meta, ""); handleErr != nil {
			return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Notification rejected", nil))
		} else if response != nil {
			if responseObj, ok := response.(*jsonrpc.Response); ok && responseObj.Error != nil {
				return c.JSON(http.StatusBadRequest, responseObj)
			}
		}
		return c.NoContent(http.StatusAccepted)
	}

	response, handleErr := s.dispatchModernMessage(c.Request().Context(), request, meta, "")
	if handleErr != nil {
		logger.Error("Error handling modern MCP message", "method", request.Method, "error", handleErr)
		response = jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "Internal error", nil)
	}
	if response == nil {
		return c.NoContent(http.StatusAccepted)
	}
	if rpcResponse, ok := response.(*jsonrpc.Response); ok && rpcResponse.Error != nil {
		return c.JSON(modernResponseStatus(rpcResponse), rpcResponse)
	}
	return c.JSON(http.StatusOK, response)
}

func (s *Server) missingMutatingCapability(request jsonrpc.Request, meta mcpv20260728.RequestMeta) *jsonrpc.Response {
	if s == nil || request.Method != "tools/call" || mcpv20260728.MutatingCapability(meta) {
		return nil
	}
	if s.config != nil && s.config.ToolControls.AllowMutatingWithoutCapability {
		return nil
	}
	var payload struct {
		Name string `json:"name"`
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(request.Params, &payload); err != nil {
		return nil
	}
	toolName := strings.TrimSpace(payload.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(payload.Tool)
	}
	tool, found := s.toolManager.GetTool(toolName)
	if !found || tool == nil || !toolspec.IsMutatingTool(tool.Name()) {
		return nil
	}
	return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrMissingRequiredClientCapability), "Missing required client capability", map[string]any{
		"requiredCapabilities": []string{"extensions.com.slighter12/godot-mcp.mutating"},
		"tool":                 tool.Name(),
	})
}

func (s *Server) validateModernHTTPRequest(c echo.Context, request jsonrpc.Request) (mcpv20260728.RequestMeta, *jsonrpc.Response) {
	meta, err := mcpv20260728.ParseRequestMeta(request.Params)
	if err != nil {
		switch {
		case errors.Is(err, mcpv20260728.ErrInvalidProtocolVersion):
			headerVersion, headerOK := singleHeaderValue(c.Request().Header, headerProtocolVersion)
			headerVersion = strings.TrimSpace(headerVersion)
			if headerOK && headerVersion != "" && headerVersion != meta.ProtocolVersion {
				return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Protocol version header does not match request metadata", mcpv20260728.HeaderMismatchData(headerProtocolVersion, headerVersion, meta.ProtocolVersion))
			}
			return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrUnsupportedProtocolVersion), "Unsupported protocol version", mcpv20260728.UnsupportedVersionData(meta.ProtocolVersion))
		case errors.Is(err, mcpv20260728.ErrMissingProtocolVersion), errors.Is(err, mcpv20260728.ErrMissingClientCapabilities), errors.Is(err, mcpv20260728.ErrInvalidRequestMeta):
			data := mcpv20260728.AddSupportedVersionsForInitialize(request.Method, map[string]any{"reason": err.Error()})
			return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid request metadata", data)
		default:
			return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid request metadata", nil)
		}
	}

	headerVersion, versionHeaderOK := singleHeaderValue(c.Request().Header, headerProtocolVersion)
	headerVersion = strings.TrimSpace(headerVersion)
	if !versionHeaderOK || headerVersion == "" || headerVersion != meta.ProtocolVersion {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Protocol version header does not match request metadata", mcpv20260728.HeaderMismatchData(headerProtocolVersion, headerVersion, meta.ProtocolVersion))
	}

	methodHeader, methodHeaderOK := singleHeaderValue(c.Request().Header, headerMethod)
	methodHeader = strings.TrimSpace(methodHeader)
	if !methodHeaderOK {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Mcp-Method header does not match request", mcpv20260728.HeaderMismatchData(headerMethod, methodHeader, request.Method))
	}
	if err := mcpv20260728.ValidateMethodHeader(request.Method, methodHeader); err != nil {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Mcp-Method header does not match request", mcpv20260728.HeaderMismatchData(headerMethod, methodHeader, request.Method))
	}
	if request.Method == "tools/call" {
		if _, err := mcpv20260728.DecodeToolCallParams(request.Params, true); err != nil {
			return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
		}
	}

	name := requestName(request)
	nameHeader, nameHeaderOK := optionalSingleHeaderValue(c.Request().Header, headerName)
	nameHeader = strings.TrimSpace(nameHeader)
	if !nameHeaderOK {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Mcp-Name header does not match request", mcpv20260728.HeaderMismatchData(headerName, nameHeader, name))
	}
	if err := mcpv20260728.ValidateNameHeader(request.Method, name, nameHeader); err != nil {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrHeaderMismatch), "Mcp-Name header does not match request", mcpv20260728.HeaderMismatchData(headerName, nameHeader, name))
	}

	if !acceptsJSONAndEventStream(strings.Join(c.Request().Header.Values(echo.HeaderAccept), ",")) {
		return meta, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidRequest), "Accept header must include application/json and text/event-stream", nil)
	}
	if response := s.validateCustomParameterHeaders(c.Request().Header, request); response != nil {
		return meta, response
	}
	return meta, nil
}

func (s *Server) validateCustomParameterHeaders(headers http.Header, request jsonrpc.Request) *jsonrpc.Response {
	if s == nil || s.toolManager == nil || request.Method != "tools/call" {
		return nil
	}
	params, err := mcpv20260728.DecodeToolCallParams(request.Params, true)
	if err != nil {
		return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid tool call payload", nil)
	}
	tool, ok := s.toolManager.GetTool(params.Name)
	if !ok || tool == nil {
		return nil
	}

	if err := mcpv20260728.ValidateParameterHeaders(tool.InputSchema(), params.Arguments, headers); err != nil {
		var headerErr *mcpv20260728.ParameterHeaderError
		if errors.As(err, &headerErr) {
			return headerParameterMismatch(request.ID, headerErr.Path, headerErr.Reason)
		}
		return headerParameterMismatch(request.ID, "", "invalid x-mcp-header schema")
	}
	return nil
}

func headerParameterMismatch(id any, argumentName, reason string) *jsonrpc.Response {
	return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrHeaderMismatch), "Mcp-Param header does not match request", map[string]any{
		"argument": argumentName,
		"reason":   reason,
	})
}

func singleHeaderValue(headers http.Header, name string) (string, bool) {
	values := headers.Values(name)
	if len(values) != 1 {
		return strings.Join(values, ","), false
	}
	return values[0], true
}

func optionalSingleHeaderValue(headers http.Header, name string) (string, bool) {
	values := headers.Values(name)
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 {
		return strings.Join(values, ","), false
	}
	return values[0], true
}

func requestName(request jsonrpc.Request) string {
	var params map[string]any
	if err := json.Unmarshal(request.Params, &params); err != nil || params == nil {
		return ""
	}
	switch request.Method {
	case "tools/call", "prompts/get":
		name, _ := params["name"].(string)
		return name
	case "resources/read":
		uri, _ := params["uri"].(string)
		return uri
	default:
		return ""
	}
}

func acceptsJSONAndEventStream(acceptHeader string) bool {
	hasJSON := false
	hasSSE := false
	for _, part := range strings.Split(acceptHeader, ",") {
		mime := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		switch strings.ToLower(mime) {
		case "application/json":
			hasJSON = true
		case "text/event-stream":
			hasSSE = true
		}
	}
	return hasJSON && hasSSE
}

func (s *Server) dispatchModernMessage(ctx context.Context, msg jsonrpc.Request, meta mcpv20260728.RequestMeta, progressRouteKey string) (any, error) {
	if s != nil {
		if response, handled := shared.DispatchWithHook(ctx, msg, meta, s.dispatchHook); handled {
			return response, nil
		}
	}
	if msg.Method == "initialize" || msg.Method == "initialized" || msg.Method == "notifications/initialized" || msg.Method == "ping" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", mcpv20260728.AddSupportedVersionsForInitialize(msg.Method, map[string]any{"method": msg.Method})), nil
	}
	if msg.Method == "notifications/cancelled" {
		return jsonrpc.NewErrorResponse(msg.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", map[string]any{
			"method": msg.Method,
		}), nil
	}
	if msg.Method == "tools/call" {
		return shared.BuildToolCallResponseWithContextAndOptions(msg, s.toolManager, s.handleGodotResource, s.modernToolCallContext(ctx, meta, requestIDKey(msg.ID), progressRouteKey), s.toolCallOptions()), nil
	}
	return shared.DispatchStandardMethodWithContextAndProviders(msg, s.toolManager, s.promptCatalog, s.handleGodotResource, s.promptRenderOptions(), s.toolCallOptions(), s.dispatchProviders, shared.DispatchContext{
		Context: ctx, RequestMeta: meta, RequestStateCodec: s.requestStateCodec,
	}), nil
}

func (s *Server) modernToolCallContext(ctx context.Context, meta mcpv20260728.RequestMeta, requestID string, progressRouteKey string) shared.ToolCallContext {
	editorSessionID := ""
	if settings := mcpv20260728.ExtensionSettings(meta.ClientCapabilities, mcpv20260728.GodotExtensionID); settings != nil {
		editorSessionID, _ = settings["editor_session_id"].(string)
	}
	if strings.TrimSpace(editorSessionID) == "" {
		if stored, ok, _ := runtimebridge.DefaultEditorStore().LatestFresh(time.Now().UTC()); ok {
			editorSessionID = strings.TrimSpace(stored.SessionID)
		}
	}
	return shared.ToolCallContext{
		Context:                 ctx,
		RequestID:               requestID,
		ProgressRouteKey:        strings.TrimSpace(progressRouteKey),
		SessionID:               strings.TrimSpace(editorSessionID),
		EditorSessionID:         strings.TrimSpace(editorSessionID),
		RuntimeSessionID:        strings.TrimSpace(editorSessionID),
		RuntimeCommandSessionID: strings.TrimSpace(editorSessionID),
		SessionInitialized:      true,
		MutatingAllowed:         mcpv20260728.MutatingCapability(meta) || (s.config != nil && s.config.ToolControls.AllowMutatingWithoutCapability),
		Modern:                  true,
		ClientInfo:              meta.ClientInfo,
		ClientCapabilities:      meta.ClientCapabilities,
		RequestStateCodec:       s.requestStateCodec,
	}
}

func (s *Server) handleProgressToolCall(c echo.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) error {
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "SSE stream is not available", nil))
	}
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	flusher.Flush()

	progressRouteKey := s.nextStreamRouteKey("progress")
	transport := NewStreamableHTTPTransport(c.Response().Writer, flusher)
	s.registerProgressStream(progressRouteKey, transport)
	defer s.unregisterProgressStream(progressRouteKey, transport)
	defer transport.Close()
	if s.progressDispatchHook != nil {
		if notifications, response, handled := s.progressDispatchHook(c.Request().Context(), request, meta); handled {
			for _, notification := range notifications {
				if c.Request().Context().Err() != nil {
					return nil
				}
				if err := transport.SendSSEWithTimeout("message", notification, progressWriteTimeout); err != nil {
					return nil
				}
			}
			if response != nil {
				_ = transport.SendSSEWithTimeout("message", response, progressWriteTimeout)
			}
			return nil
		}
	}

	type dispatchResult struct {
		response any
		err      error
	}
	resultCh := make(chan dispatchResult, 1)
	go func() {
		response, handleErr := s.dispatchModernMessage(c.Request().Context(), request, meta, progressRouteKey)
		resultCh <- dispatchResult{response: response, err: handleErr}
	}()

	var result dispatchResult
	select {
	case result = <-resultCh:
	case <-c.Request().Context().Done():
		s.markCanceledProgressRequest(progressRouteKey)
		return nil
	}
	response, handleErr := result.response, result.err
	if handleErr != nil {
		logger.Error("Error handling modern MCP progress request", "method", request.Method, "error", handleErr)
		response = jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "Internal error", nil)
	}
	if response != nil {
		if err := transport.SendSSEWithTimeout("message", response, progressWriteTimeout); err != nil {
			return nil
		}
	}
	return nil
}

func requestIDKey(id any) string {
	if id == nil {
		return ""
	}
	return fmt.Sprint(id)
}

func (s *Server) handleSubscription(c echo.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) error {
	var params map[string]any
	if err := json.Unmarshal(request.Params, &params); err != nil || params == nil {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid subscriptions/listen payload", nil))
	}
	promptsEnabled := s.promptCatalog != nil && s.promptCatalog.Enabled()
	editorSessionID, commandEnabled, acceptedNotifications, err := commandSubscriptionRequest(meta, params, promptsEnabled)
	if err != nil {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil))
	}
	if request.ID == nil {
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "subscriptions/listen requires a request id", nil))
	}

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "SSE stream is not available", nil))
	}
	subscriptionID := request.ID
	subscriptionRouteKey := s.nextStreamRouteKey("subscription")
	streamCtx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()
	transport := NewStreamableHTTPTransport(c.Response().Writer, flusher, cancel)
	if s.subscriptionManager == nil {
		_ = transport.Close()
		return c.JSON(http.StatusInternalServerError, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "Subscription manager is not configured", nil))
	}
	if err := s.subscriptionManager.Open(subscriptionRouteKey, subscriptionID, strings.TrimSpace(editorSessionID), transport, cancel, commandEnabled, acceptedNotifications); err != nil {
		_ = transport.Close()
		return c.JSON(http.StatusBadRequest, jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), err.Error(), nil))
	}
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	flusher.Flush()
	defer s.subscriptionManager.Remove(subscriptionRouteKey, transport)
	if err := transport.SendSSE("message", acknowledgedNotification(subscriptionID, acceptedNotifications)); err != nil {
		return nil
	}

	keepAlive := time.NewTicker(subscriptionKeepAliveInterval)
	defer keepAlive.Stop()
	for {
		select {
		case <-streamCtx.Done():
			return nil
		case <-keepAlive.C:
			if err := transport.SendComment(""); err != nil {
				return nil
			}
		}
	}
}

func modernResponseStatus(response *jsonrpc.Response) int {
	if response == nil || response.Error == nil {
		return http.StatusOK
	}
	switch response.Error.Code {
	case int(jsonrpc.ErrMethodNotFound):
		return http.StatusNotFound
	case int(jsonrpc.ErrHeaderMismatch), int(jsonrpc.ErrMissingRequiredClientCapability), int(jsonrpc.ErrUnsupportedProtocolVersion):
		return http.StatusBadRequest
	default:
		return http.StatusOK
	}
}
