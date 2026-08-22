package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

// StdioServer handles MCP 2026-07-28 communication over newline-delimited
// JSON-RPC.  Protocol state is per request; no initialize handshake exists.
type StdioServer struct {
	toolManager         *tools.Manager
	promptCatalog       *promptcatalog.Registry
	promptRenderOptions shared.PromptRenderOptions
	toolCallOptions     shared.ToolCallOptions

	writeMu         sync.Mutex
	pendingMu       sync.Mutex
	pending         map[string]context.CancelFunc
	subscriptionsMu sync.Mutex
	subscriptions   map[string]context.CancelFunc
}

func NewStdioServer(toolManager *tools.Manager) *StdioServer {
	return &StdioServer{
		toolManager:         toolManager,
		promptRenderOptions: shared.DefaultPromptRenderOptions(),
		toolCallOptions:     shared.DefaultToolCallOptions(),
		pending:             make(map[string]context.CancelFunc),
		subscriptions:       make(map[string]context.CancelFunc),
	}
}

func (s *StdioServer) AttachPromptCatalog(registry *promptcatalog.Registry) {
	s.promptCatalog = registry
}

func (s *StdioServer) AttachPromptRenderOptions(options shared.PromptRenderOptions) {
	s.promptRenderOptions = options
}

func (s *StdioServer) AttachToolCallOptions(options shared.ToolCallOptions) {
	s.toolCallOptions = options
}

func (s *StdioServer) Start() error {
	decoder := json.NewDecoder(os.Stdin)
	logger.Debug("Stdio server started and waiting for messages")

	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				s.cancelAll()
				return nil
			}
			logger.Error("Error decoding message", "error", err)
			s.write(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
			continue
		}

		requests, prebuiltResponses, acceptedOneWay, parseErr := shared.ParseJSONRPCFrame(raw)
		if parseErr != nil {
			s.write(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
			continue
		}
		for _, response := range prebuiltResponses {
			s.write(response)
		}
		if len(requests) == 0 && len(prebuiltResponses) == 0 && !acceptedOneWay {
			s.write(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Invalid request", nil))
			continue
		}

		for _, request := range requests {
			if request.Method == "notifications/cancelled" {
				s.handleCancellation(request)
				continue
			}
			go s.handleRequest(request)
		}
	}
}

func (s *StdioServer) handleRequest(request jsonrpc.Request) {
	requestKey := requestIDKey(request.ID)
	ctx, cancel := context.WithCancel(context.Background())
	if requestKey != "" {
		s.pendingMu.Lock()
		s.pending[requestKey] = cancel
		s.pendingMu.Unlock()
		defer func() {
			s.pendingMu.Lock()
			delete(s.pending, requestKey)
			s.pendingMu.Unlock()
		}()
	}
	defer cancel()

	meta, err := mcpv20260728.ParseRequestMeta(request.Params)
	if err != nil {
		if request.ID != nil {
			s.write(protocolErrorResponse(request.ID, request.Method, err))
		}
		return
	}

	if request.Method == "subscriptions/listen" {
		s.handleSubscription(ctx, request, meta)
		return
	}

	response, handleErr := s.dispatch(request, meta)
	if ctx.Err() != nil || request.ID == nil {
		return
	}
	if handleErr != nil {
		s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInternalError), "Internal error", nil))
		return
	}
	if response != nil {
		s.write(response)
	}
}

func (s *StdioServer) dispatch(request jsonrpc.Request, meta mcpv20260728.RequestMeta) (any, error) {
	if request.Method == "initialize" || request.Method == "initialized" || request.Method == "notifications/initialized" || request.Method == "ping" {
		return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", mcpv20260728.AddSupportedVersionsForInitialize(request.Method, map[string]any{"method": request.Method})), nil
	}
	if request.Method == "tools/call" {
		return shared.BuildToolCallResponseWithContextAndOptions(request, s.toolManager, readGodotResource, modernToolCallContext(meta, requestIDKey(request.ID)), s.toolCallOptions), nil
	}
	return shared.DispatchStandardMethodWithOptions(request, s.toolManager, s.promptCatalog, readGodotResource, s.promptRenderOptions, s.toolCallOptions), nil
}

func modernToolCallContext(meta mcpv20260728.RequestMeta, requestID string) shared.ToolCallContext {
	editorSessionID := ""
	if settings := mcpv20260728.ExtensionSettings(meta.ClientCapabilities, mcpv20260728.GodotExtensionID); settings != nil {
		editorSessionID, _ = settings["editor_session_id"].(string)
	}
	return shared.ToolCallContext{
		RequestID:               requestID,
		SessionID:               strings.TrimSpace(editorSessionID),
		EditorSessionID:         strings.TrimSpace(editorSessionID),
		RuntimeSessionID:        strings.TrimSpace(editorSessionID),
		RuntimeCommandSessionID: strings.TrimSpace(editorSessionID),
		SessionInitialized:      true,
		MutatingAllowed:         mcpv20260728.MutatingCapability(meta),
		Modern:                  true,
	}
}

// SendRuntimeCommandProgressNotification writes request-scoped progress on
// the stdio response channel. The request id is used for server-side routing;
// the MCP notification itself carries the progress token.
func (s *StdioServer) SendRuntimeCommandProgressNotification(event tooltypes.RuntimeCommandProgressEvent) {
	if s == nil || !notifications.IsValidProgressToken(event.ProgressToken) {
		return
	}
	s.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/progress",
		"params":  notifications.ProgressParams(event.ProgressToken, event.Progress, event.Message),
	})
}

func (s *StdioServer) handleCancellation(request jsonrpc.Request) {
	var params struct {
		RequestID any `json:"requestId"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return
	}
	key := requestIDKey(params.RequestID)
	s.pendingMu.Lock()
	cancel := s.pending[key]
	delete(s.pending, key)
	s.pendingMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.subscriptionsMu.Lock()
	if subscriptionCancel := s.subscriptions[key]; subscriptionCancel != nil {
		delete(s.subscriptions, key)
		subscriptionCancel()
	}
	s.subscriptionsMu.Unlock()
}

func (s *StdioServer) handleSubscription(ctx context.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) {
	if request.ID == nil {
		return
	}
	var params map[string]any
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid subscriptions/listen payload", nil))
		return
	}
	subscriptionID := request.ID
	subscriptionKey := requestIDKey(subscriptionID)
	notifications := map[string]any{}
	requested, _ := params["notifications"].(map[string]any)
	for _, key := range []string{"promptsListChanged", "resourcesListChanged", "toolsListChanged", "resourceSubscriptions"} {
		if value, exists := requested[key]; exists {
			notifications[key] = value
		}
	}
	if filter, exists := requested[mcpv20260728.CommandStreamExtensionID]; exists {
		if mcpv20260728.ExtensionSettings(meta.ClientCapabilities, mcpv20260728.CommandStreamExtensionID) == nil {
			s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Command stream extension is not advertised", nil))
			return
		}
		notifications[mcpv20260728.CommandStreamExtensionID] = filter
	}
	s.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/subscriptions/acknowledged",
		"params": map[string]any{
			"_meta":         map[string]any{"io.modelcontextprotocol/subscriptionId": subscriptionID},
			"notifications": notifications,
		},
	})
	s.subscriptionsMu.Lock()
	s.subscriptions[subscriptionKey] = func() {}
	s.subscriptionsMu.Unlock()
	<-ctx.Done()
	s.subscriptionsMu.Lock()
	delete(s.subscriptions, subscriptionKey)
	s.subscriptionsMu.Unlock()
}

func (s *StdioServer) cancelAll() {
	s.pendingMu.Lock()
	for key, cancel := range s.pending {
		cancel()
		delete(s.pending, key)
	}
	s.pendingMu.Unlock()
	s.subscriptionsMu.Lock()
	for key, cancel := range s.subscriptions {
		cancel()
		delete(s.subscriptions, key)
	}
	s.subscriptionsMu.Unlock()
}

func (s *StdioServer) write(value any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		logger.Error("Error encoding stdio response", "error", err)
	}
}

func protocolErrorResponse(id any, method string, err error) *jsonrpc.Response {
	switch {
	case errors.Is(err, mcpv20260728.ErrInvalidProtocolVersion):
		data := mcpv20260728.UnsupportedVersionData(extractProtocolVersion(err))
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrUnsupportedProtocolVersion), "Unsupported protocol version", mcpv20260728.AddSupportedVersionsForInitialize(method, data))
	case errors.Is(err, mcpv20260728.ErrMissingProtocolVersion), errors.Is(err, mcpv20260728.ErrMissingClientCapabilities), errors.Is(err, mcpv20260728.ErrInvalidRequestMeta):
		data := mcpv20260728.AddSupportedVersionsForInitialize(method, map[string]any{"reason": err.Error()})
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrInvalidParams), "Invalid request metadata", data)
	default:
		return jsonrpc.NewErrorResponse(id, int(jsonrpc.ErrInvalidParams), "Invalid request metadata", mcpv20260728.AddSupportedVersionsForInitialize(method, nil))
	}
}

func extractProtocolVersion(err error) string {
	var unsupported *mcpv20260728.UnsupportedProtocolVersionError
	if errors.As(err, &unsupported) {
		return unsupported.Requested
	}
	return ""
}

func requestIDKey(id any) string {
	if id == nil {
		return ""
	}
	return fmt.Sprint(id)
}

func readGodotResource(path string) (any, error) {
	switch path {
	case "godot://script/current":
		return map[string]any{"type": "script", "path": "current"}, nil
	case "godot://scene/current":
		return map[string]any{"type": "scene", "path": "current"}, nil
	case "godot://project/info":
		return map[string]any{"name": "godot-mcp", "version": "0.3.0", "type": "godot"}, nil
	case "godot://policy/godot-checks":
		return map[string]any{"policy": "policy-godot", "checks": promptcatalog.GodotPolicyChecks()}, nil
	case "godot://runtime/metrics":
		return runtimebridge.HealthSnapshot(time.Now().UTC()), nil
	default:
		return nil, fmt.Errorf("unknown resource path: %s", path)
	}
}
