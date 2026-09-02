package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
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
	dispatchProviders   shared.DispatchProviders
	requestStateCodec   *mcpv20260728.RequestStateCodec

	writeMu                   sync.Mutex
	output                    io.Writer
	pendingMu                 sync.Mutex
	pending                   map[string]context.CancelFunc
	subscriptionsMu           sync.Mutex
	subscriptions             map[string]*stdioSubscription
	subscriptionRouteSequence atomic.Uint64
}

type stdioSubscription struct {
	key             string
	id              any
	idKey           string
	editorSessionID string
	commandEnabled  bool
	notifications   map[string]any
	cancel          context.CancelFunc
}

func NewStdioServer(toolManager *tools.Manager) *StdioServer {
	return &StdioServer{
		toolManager:         toolManager,
		promptRenderOptions: shared.DefaultPromptRenderOptions(),
		toolCallOptions:     shared.DefaultToolCallOptions(),
		output:              os.Stdout,
		pending:             make(map[string]context.CancelFunc),
		subscriptions:       make(map[string]*stdioSubscription),
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

// AttachDispatchProviders enables optional production capabilities through
// the same shared dispatch providers used by Streamable HTTP.
func (s *StdioServer) AttachDispatchProviders(providers shared.DispatchProviders) {
	s.dispatchProviders = providers
}

func (s *StdioServer) AttachRequestStateCodec(codec *mcpv20260728.RequestStateCodec) {
	s.requestStateCodec = codec
}

func (s *StdioServer) Start() error {
	return s.serve(os.Stdin)
}

func (s *StdioServer) serve(input io.Reader) error {
	reader := bufio.NewReader(input)
	logger.Debug("Stdio server started and waiting for messages")

	for {
		raw, oversized, err := readStdioFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.cancelAll()
				return nil
			}
			if oversized {
				s.write(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrInvalidRequest), "Request body too large", nil))
				s.cancelAll()
				return errors.New("request body too large")
			}
			logger.Error("Error decoding message", "error", err)
			s.write(jsonrpc.NewErrorResponse(nil, int(jsonrpc.ErrParseError), "Parse error", nil))
			s.cancelAll()
			return fmt.Errorf("decode stdio message: %w", err)
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

func readStdioFrame(reader *bufio.Reader) (json.RawMessage, bool, error) {
	frame := make([]byte, 0, 4096)
	for {
		if len(frame) == shared.MaxJSONRPCFrameBytes {
			boundary, err := reader.ReadByte()
			switch {
			case err == nil && boundary == '\n':
				frame = append(frame, boundary)
				return json.RawMessage(frame), false, nil
			case err == nil:
				return nil, true, errors.New("request body too large")
			case errors.Is(err, io.EOF):
				return json.RawMessage(frame), false, nil
			default:
				return nil, false, err
			}
		}
		chunk, err := reader.ReadSlice('\n')
		frameBytes := len(frame) + len(chunk)
		if len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
			frameBytes--
		}
		if frameBytes > shared.MaxJSONRPCFrameBytes {
			return nil, true, errors.New("request body too large")
		}
		frame = append(frame, chunk...)
		switch {
		case err == nil:
			return json.RawMessage(frame), false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(frame) > 0:
			return json.RawMessage(frame), false, nil
		default:
			return nil, false, err
		}
	}
}

func (s *StdioServer) handleRequest(request jsonrpc.Request) {
	requestKey := requestIDKey(request.ID)
	tracksPending := requestKey != "" && request.Method != "subscriptions/listen"
	ctx, cancel := context.WithCancel(context.Background())
	if tracksPending {
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
		s.handleSubscription(ctx, cancel, request, meta)
		return
	}

	response, handleErr := s.dispatchContext(ctx, request, meta)
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
	return s.dispatchContext(context.Background(), request, meta)
}

func (s *StdioServer) dispatchContext(ctx context.Context, request jsonrpc.Request, meta mcpv20260728.RequestMeta) (any, error) {
	if request.Method == "initialize" || request.Method == "initialized" || request.Method == "notifications/initialized" || request.Method == "ping" {
		return jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrMethodNotFound), "Method not found", mcpv20260728.AddSupportedVersionsForInitialize(request.Method, map[string]any{"method": request.Method})), nil
	}
	if request.Method == "tools/call" {
		callContext := modernToolCallContext(meta, requestIDKey(request.ID))
		callContext.Context = ctx
		callContext.RequestStateCodec = s.requestStateCodec
		return shared.BuildToolCallResponseWithContextAndOptions(request, s.toolManager, readGodotResource, callContext, s.toolCallOptions), nil
	}
	return shared.DispatchStandardMethodWithContextAndProviders(request, s.toolManager, s.promptCatalog, readGodotResource, s.promptRenderOptions, s.toolCallOptions, s.dispatchProviders, shared.DispatchContext{
		Context: ctx, RequestMeta: meta, RequestStateCodec: s.requestStateCodec,
	}), nil
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
		ClientInfo:              meta.ClientInfo,
		ClientCapabilities:      meta.ClientCapabilities,
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
	decoder := json.NewDecoder(bytes.NewReader(request.Params))
	decoder.UseNumber()
	if err := decoder.Decode(&params); err != nil {
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
	subscriptions := make([]*stdioSubscription, 0)
	for subscriptionKey, subscription := range s.subscriptions {
		if subscription != nil && subscription.idKey == key {
			delete(s.subscriptions, subscriptionKey)
			subscriptions = append(subscriptions, subscription)
		}
	}
	s.subscriptionsMu.Unlock()
	for _, subscription := range subscriptions {
		if subscription.cancel != nil {
			subscription.cancel()
		}
	}
}

func (s *StdioServer) handleSubscription(ctx context.Context, cancel context.CancelFunc, request jsonrpc.Request, meta mcpv20260728.RequestMeta) {
	if request.ID == nil {
		return
	}
	var params map[string]any
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Invalid subscriptions/listen payload", nil))
		return
	}
	subscriptionID := request.ID
	subscriptionKey := s.nextSubscriptionRouteKey()
	notifications := map[string]any{}
	requested, _ := params["notifications"].(map[string]any)
	for _, key := range []string{"promptsListChanged", "resourcesListChanged", "toolsListChanged", "resourceSubscriptions"} {
		if value, exists := requested[key]; exists {
			if key != "promptsListChanged" || (s.promptCatalog != nil && s.promptCatalog.Enabled()) {
				notifications[key] = value
			}
		}
	}
	editorSessionID := ""
	commandEnabled := false
	if filter, exists := requested[mcpv20260728.CommandStreamExtensionID]; exists {
		if mcpv20260728.ExtensionSettings(meta.ClientCapabilities, mcpv20260728.CommandStreamExtensionID) == nil {
			s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "Command stream extension is not advertised", nil))
			return
		}
		settings, _ := meta.Raw[mcpv20260728.CommandStreamExtensionID].(map[string]any)
		if settings == nil {
			s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "command stream subscription settings are required", nil))
			return
		}
		editorSessionID, _ = settings["editor_session_id"].(string)
		editorSessionID = strings.TrimSpace(editorSessionID)
		filterSettings, _ := filter.(map[string]any)
		commandEnabled, _ = filterSettings["command"].(bool)
		if commandEnabled && editorSessionID == "" {
			s.write(jsonrpc.NewErrorResponse(request.ID, int(jsonrpc.ErrInvalidParams), "editor_session_id is required for command subscriptions", nil))
			return
		}
		if commandEnabled {
			notifications[mcpv20260728.CommandStreamExtensionID] = map[string]any{"command": true}
		}
	}
	subscription := &stdioSubscription{
		key:             subscriptionKey,
		id:              subscriptionID,
		idKey:           requestIDKey(subscriptionID),
		editorSessionID: editorSessionID,
		commandEnabled:  commandEnabled,
		notifications:   cloneStdioMap(notifications),
		cancel:          cancel,
	}
	s.subscriptionsMu.Lock()
	s.subscriptions[subscriptionKey] = subscription
	s.subscriptionsMu.Unlock()
	s.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/subscriptions/acknowledged",
		"params": map[string]any{
			"_meta":         map[string]any{"io.modelcontextprotocol/subscriptionId": subscriptionID},
			"notifications": notifications,
		},
	})
	<-ctx.Done()
	s.subscriptionsMu.Lock()
	if current := s.subscriptions[subscriptionKey]; current == subscription {
		delete(s.subscriptions, subscriptionKey)
	}
	s.subscriptionsMu.Unlock()
}

func (s *StdioServer) nextSubscriptionRouteKey() string {
	return fmt.Sprintf("subscription-%d", s.subscriptionRouteSequence.Add(1))
}

func (s *StdioServer) cancelAll() {
	s.pendingMu.Lock()
	for key, cancel := range s.pending {
		cancel()
		delete(s.pending, key)
	}
	s.pendingMu.Unlock()
	s.subscriptionsMu.Lock()
	subscriptions := make([]*stdioSubscription, 0, len(s.subscriptions))
	for key, subscription := range s.subscriptions {
		subscriptions = append(subscriptions, subscription)
		delete(s.subscriptions, key)
	}
	s.subscriptionsMu.Unlock()
	for _, subscription := range subscriptions {
		if subscription == nil {
			continue
		}
		s.write(gracefulStdioSubscriptionResponse(subscription.id))
		if subscription.cancel != nil {
			subscription.cancel()
		}
	}
}

func (s *StdioServer) SendJSONRPCNotificationToEditor(editorSessionID string, message map[string]any) bool {
	if s == nil || strings.TrimSpace(editorSessionID) == "" {
		return false
	}
	editorSessionID = strings.TrimSpace(editorSessionID)
	s.subscriptionsMu.Lock()
	targets := make([]*stdioSubscription, 0)
	for _, subscription := range s.subscriptions {
		if subscription != nil && subscription.commandEnabled && subscription.editorSessionID == editorSessionID {
			targets = append(targets, subscription)
		}
	}
	s.subscriptionsMu.Unlock()
	for _, subscription := range targets {
		s.write(withStdioSubscriptionID(message, subscription.id))
	}
	return len(targets) > 0
}

func (s *StdioServer) SendNotification(filterKey string, message map[string]any) int {
	if s == nil || strings.TrimSpace(filterKey) == "" {
		return 0
	}
	s.subscriptionsMu.Lock()
	targets := make([]*stdioSubscription, 0)
	for _, subscription := range s.subscriptions {
		if enabled, _ := subscription.notifications[filterKey].(bool); enabled {
			targets = append(targets, subscription)
		}
	}
	s.subscriptionsMu.Unlock()
	for _, subscription := range targets {
		s.write(withStdioSubscriptionID(message, subscription.id))
	}
	return len(targets)
}

func cloneStdioMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func withStdioSubscriptionID(message map[string]any, subscriptionID any) map[string]any {
	copyMessage := cloneStdioMap(message)
	params, _ := copyMessage["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	} else {
		params = cloneStdioMap(params)
	}
	meta, _ := params["_meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	} else {
		meta = cloneStdioMap(meta)
	}
	meta["io.modelcontextprotocol/subscriptionId"] = subscriptionID
	params["_meta"] = meta
	copyMessage["params"] = params
	return copyMessage
}

func gracefulStdioSubscriptionResponse(subscriptionID any) *jsonrpc.Response {
	return jsonrpc.NewResponse(subscriptionID, map[string]any{
		"resultType": "complete",
		"_meta": map[string]any{
			"io.modelcontextprotocol/subscriptionId": subscriptionID,
		},
	})
}

func (s *StdioServer) write(value any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	output := s.output
	if output == nil {
		output = os.Stdout
	}
	if err := json.NewEncoder(output).Encode(value); err != nil {
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
	encoded, err := json.Marshal(id)
	if err != nil {
		return fmt.Sprintf("%T:%v", id, id)
	}
	return string(encoded)
}

func readGodotResource(path string) (any, error) {
	switch path {
	case "godot://script/current":
		return map[string]any{"type": "script", "path": "current"}, nil
	case "godot://scene/current":
		return map[string]any{"type": "scene", "path": "current"}, nil
	case "godot://project/info":
		return map[string]any{"name": "godot-mcp", "version": mcp.ServerVersion, "type": "godot"}, nil
	case "godot://policy/godot-checks":
		return map[string]any{"policy": "policy-godot", "checks": promptcatalog.GodotPolicyChecks()}, nil
	case "godot://runtime/metrics":
		return runtimebridge.HealthSnapshot(time.Now().UTC()), nil
	default:
		return nil, fmt.Errorf("unknown resource path: %s", path)
	}
}
