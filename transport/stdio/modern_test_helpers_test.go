package stdio

import (
	"encoding/json"
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
)

type modernStdioClientOptions struct {
	EditorSessionID string
	Mutating        bool
}

func newTestStdioServer(promptCatalogEnabled bool) *StdioServer {
	manager := tools.NewManager()
	manager.RegisterDefaultTools()
	server := NewStdioServer(manager)
	server.AttachPromptCatalog(promptcatalog.NewRegistry(promptCatalogEnabled))
	return server
}

func dispatchModernTest(t *testing.T, server *StdioServer, id any, method string, params map[string]any, options modernStdioClientOptions) *jsonrpc.Response {
	t.Helper()
	params = modernStdioParams(params, options)
	request := jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      id,
		Method:  method,
		Params:  mustRaw(t, params),
	}
	meta, err := mcpv20260728.ParseRequestMeta(request.Params)
	if err != nil {
		t.Fatalf("parse modern metadata: %v", err)
	}
	response, dispatchErr := server.dispatch(request, meta)
	if dispatchErr != nil {
		t.Fatalf("dispatch %s: %v", method, dispatchErr)
	}
	if response == nil {
		t.Fatalf("dispatch %s returned nil response", method)
	}
	rpcResponse, ok := response.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("dispatch %s returned %T", method, response)
	}
	return rpcResponse
}

func modernStdioParams(params map[string]any, options modernStdioClientOptions) map[string]any {
	result := map[string]any{}
	for key, value := range params {
		result[key] = value
	}
	settings := map[string]any{
		"version":  "1",
		"mutating": options.Mutating,
	}
	if options.EditorSessionID != "" {
		settings["editor_session_id"] = options.EditorSessionID
	}
	result["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion": mcpv20260728.ProtocolVersion,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name":    "stdio-modern-test",
			"version": "0.3.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{
			"extensions": map[string]any{
				mcpv20260728.GodotExtensionID: settings,
			},
		},
	}
	return result
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return raw
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return result
}
