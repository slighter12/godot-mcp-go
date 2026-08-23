package http

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
)

const subscriptionWriteTimeout = 5 * time.Second

// SubscriptionManager owns long-lived subscriptions independently from MCP
// request processing.  The editor session handle is application state; it is
// deliberately not an MCP protocol session identifier.
type SubscriptionManager struct {
	mu            sync.RWMutex
	subscriptions map[string]*commandSubscription
}

type commandSubscription struct {
	key             string
	id              any
	editorSessionID string
	transport       *StreamableHTTPTransport
	close           func()
	commandEnabled  bool
	notifications   map[string]any
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{subscriptions: make(map[string]*commandSubscription)}
}

func (m *SubscriptionManager) Open(key string, id any, editorSessionID string, transport *StreamableHTTPTransport, closeFn func(), commandEnabled bool, notifications map[string]any) error {
	// key is server-generated; id is the original client JSON-RPC id exposed
	// back to the client as subscriptionId.
	if m == nil || transport == nil || key == "" || id == nil {
		return fmt.Errorf("invalid subscription")
	}
	if commandEnabled && editorSessionID == "" {
		return fmt.Errorf("editor_session_id is required for command subscriptions")
	}

	m.mu.Lock()
	existing := m.subscriptions[key]
	m.subscriptions[key] = &commandSubscription{
		key:             key,
		id:              id,
		editorSessionID: editorSessionID,
		transport:       transport,
		close:           closeFn,
		commandEnabled:  commandEnabled,
		notifications:   cloneMap(notifications),
	}
	m.mu.Unlock()
	if existing != nil {
		existing.transport.Close()
		if existing.close != nil {
			existing.close()
		}
	}
	return nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (m *SubscriptionManager) Remove(key string, expected ...*StreamableHTTPTransport) {
	if m == nil {
		return
	}
	m.mu.Lock()
	subscription := m.subscriptions[key]
	if subscription != nil && len(expected) > 0 && subscription.transport != expected[0] {
		subscription = nil
	} else if subscription != nil {
		delete(m.subscriptions, key)
	}
	m.mu.Unlock()
	if subscription != nil {
		subscription.transport.Close()
		if subscription.close != nil {
			subscription.close()
		}
	}
}

// CloseAll sends a terminal result to each active subscription before closing
// its stream. This gives clients a protocol-level distinction between a
// graceful server shutdown and an unexpected connection failure.
func (m *SubscriptionManager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	subscriptions := make([]*commandSubscription, 0, len(m.subscriptions))
	for key, subscription := range m.subscriptions {
		subscriptions = append(subscriptions, subscription)
		delete(m.subscriptions, key)
	}
	m.mu.Unlock()

	for _, subscription := range subscriptions {
		_ = subscription.transport.SendSSEWithTimeout("message", gracefulSubscriptionResponse(subscription.id), subscriptionWriteTimeout)
		if subscription.close != nil {
			subscription.close()
		}
		_ = subscription.transport.Close()
	}
}

func (m *SubscriptionManager) SendToEditor(editorSessionID string, message map[string]any) bool {
	if m == nil || strings.TrimSpace(editorSessionID) == "" {
		return false
	}
	return m.send(editorSessionID, message)
}

func (m *SubscriptionManager) SendNotification(filterKey string, message map[string]any) int {
	if m == nil || strings.TrimSpace(filterKey) == "" {
		return 0
	}
	m.mu.RLock()
	targets := make([]*commandSubscription, 0)
	for _, subscription := range m.subscriptions {
		if enabled, _ := subscription.notifications[filterKey].(bool); enabled {
			targets = append(targets, subscription)
		}
	}
	m.mu.RUnlock()

	results := make(chan bool, len(targets))
	for _, subscription := range targets {
		go func(subscription *commandSubscription) {
			if err := subscription.transport.SendSSEWithTimeout("message", withSubscriptionID(message, subscription.id), subscriptionWriteTimeout); err != nil {
				m.Remove(subscription.key, subscription.transport)
				results <- false
				return
			}
			results <- true
		}(subscription)
	}
	sent := 0
	for range targets {
		if <-results {
			sent++
		}
	}
	return sent
}

func (m *SubscriptionManager) send(editorSessionID string, message map[string]any) bool {
	if m == nil || editorSessionID == "" {
		return false
	}
	m.mu.RLock()
	targets := make([]*commandSubscription, 0)
	for _, subscription := range m.subscriptions {
		if subscription.commandEnabled && subscription.editorSessionID == editorSessionID {
			targets = append(targets, subscription)
		}
	}
	m.mu.RUnlock()

	sent := false
	results := make(chan bool, len(targets))
	for _, subscription := range targets {
		go func(subscription *commandSubscription) {
			if err := subscription.transport.SendSSEWithTimeout("message", withSubscriptionID(message, subscription.id), subscriptionWriteTimeout); err != nil {
				m.Remove(subscription.key, subscription.transport)
				results <- false
				return
			}
			results <- true
		}(subscription)
	}
	for range targets {
		if <-results {
			sent = true
		}
	}
	return sent
}

func withSubscriptionID(message map[string]any, subscriptionID any) map[string]any {
	copyMessage := make(map[string]any, len(message))
	for key, value := range message {
		copyMessage[key] = value
	}
	params, _ := copyMessage["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	} else {
		paramsCopy := make(map[string]any, len(params))
		for key, value := range params {
			paramsCopy[key] = value
		}
		params = paramsCopy
	}
	meta, _ := params["_meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	} else {
		metaCopy := make(map[string]any, len(meta))
		for key, value := range meta {
			metaCopy[key] = value
		}
		meta = metaCopy
	}
	meta["io.modelcontextprotocol/subscriptionId"] = subscriptionID
	params["_meta"] = meta
	copyMessage["params"] = params
	return copyMessage
}

func acknowledgedNotification(subscriptionID any, notifications map[string]any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/subscriptions/acknowledged",
		"params": map[string]any{
			"_meta": map[string]any{
				"io.modelcontextprotocol/subscriptionId": subscriptionID,
			},
			"notifications": notifications,
		},
	}
}

func gracefulSubscriptionResponse(subscriptionID any) *jsonrpc.Response {
	return jsonrpc.NewResponse(subscriptionID, map[string]any{
		"resultType": "complete",
		"_meta": map[string]any{
			"io.modelcontextprotocol/subscriptionId": subscriptionID,
		},
	})
}

func commandSubscriptionRequest(meta mcpv20260728.RequestMeta, params map[string]any, promptsEnabled bool) (string, bool, map[string]any, error) {
	notifications, _ := params["notifications"].(map[string]any)
	_, customFilterRequested := notifications[mcpv20260728.CommandStreamExtensionID]
	acceptedNotifications := map[string]any{}
	if requested, _ := notifications["promptsListChanged"].(bool); promptsEnabled && requested {
		acceptedNotifications["promptsListChanged"] = true
	}
	if !customFilterRequested {
		return "", false, acceptedNotifications, nil
	}
	capability := mcpv20260728.ExtensionSettings(meta.ClientCapabilities, mcpv20260728.CommandStreamExtensionID)
	if capability == nil {
		return "", false, nil, fmt.Errorf("client does not advertise %s", mcpv20260728.CommandStreamExtensionID)
	}
	settings, _ := meta.Raw[mcpv20260728.CommandStreamExtensionID].(map[string]any)
	if settings == nil {
		return "", false, nil, fmt.Errorf("%s subscription settings are required", mcpv20260728.CommandStreamExtensionID)
	}
	editorSessionID, _ := settings["editor_session_id"].(string)
	if editorSessionID == "" {
		return "", false, nil, fmt.Errorf("editor_session_id is required for command subscriptions")
	}
	commandFilter, _ := notifications[mcpv20260728.CommandStreamExtensionID].(map[string]any)
	commandEnabled, _ := commandFilter["command"].(bool)
	if commandEnabled {
		acceptedNotifications[mcpv20260728.CommandStreamExtensionID] = map[string]any{"command": true}
	}
	return editorSessionID, commandEnabled, acceptedNotifications, nil
}
