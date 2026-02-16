package stdio

import (
	"encoding/json"
	"testing"

	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools"
)

func TestInitializeCapabilitiesReflectPromptCatalog(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "enabled", enabled: true},
		{name: "disabled", enabled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestStdioServer(tc.enabled)
			respAny, err := server.handleMessage(jsonrpc.Request{ID: 1, Method: "initialize"})
			if err != nil {
				t.Fatalf("handle initialize: %v", err)
			}
			resp, ok := respAny.(*jsonrpc.Response)
			if !ok {
				t.Fatalf("expected jsonrpc response, got %T", respAny)
			}
			result := mustMap(t, resp.Result)
			capabilities := mustMap(t, result["capabilities"])
			_, hasPrompts := capabilities["prompts"]
			if tc.enabled && !hasPrompts {
				t.Fatal("expected prompts capability when prompt catalog is enabled")
			}
			if !tc.enabled && hasPrompts {
				t.Fatal("did not expect prompts capability when prompt catalog is disabled")
			}
		})
	}
}

func TestStdioPromptsFlow(t *testing.T) {
	server := newTestStdioServer(true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "desc",
		Template:    "Review {{scene_path}}",
	})

	listRespAny, err := server.handleMessage(jsonrpc.Request{ID: 1, Method: "prompts/list", Params: mustRaw(t, map[string]any{})})
	if err != nil {
		t.Fatalf("prompts/list failed: %v", err)
	}
	listResp, ok := listRespAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", listRespAny)
	}
	listResult := mustMap(t, listResp.Result)
	promptsRaw, ok := listResult["prompts"].([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", listResult["prompts"])
	}
	if len(promptsRaw) != 1 || promptsRaw[0]["name"] != "scene-review" {
		t.Fatalf("unexpected prompts list: %+v", promptsRaw)
	}

	getRespAny, err := server.handleMessage(jsonrpc.Request{
		ID:     2,
		Method: "prompts/get",
		Params: mustRaw(t, map[string]any{
			"name":      "scene-review",
			"arguments": map[string]any{"scene_path": "res://Main.tscn"},
		}),
	})
	if err != nil {
		t.Fatalf("prompts/get failed: %v", err)
	}
	getResp, ok := getRespAny.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", getRespAny)
	}
	getResult := mustMap(t, getResp.Result)
	messages, ok := getResult["messages"].([]map[string]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message, got %T %v", getResult["messages"], getResult["messages"])
	}
	content := mustMap(t, messages[0]["content"])
	if content["text"] != "Review <user_input name=\"scene_path\">\nres://Main.tscn\n</user_input>" {
		t.Fatalf("expected rendered prompt, got %v", content["text"])
	}
}

func TestStdioPromptsNotSupportedWhenCatalogDisabled(t *testing.T) {
	server := newTestStdioServer(false)

	listRespAny, err := server.handleMessage(jsonrpc.Request{
		ID:     1,
		Method: "prompts/list",
		Params: mustRaw(t, map[string]any{}),
	})
	if err != nil {
		t.Fatalf("prompts/list failed: %v", err)
	}
	assertNotSupportedError(t, listRespAny)

	getRespAny, err := server.handleMessage(jsonrpc.Request{
		ID:     2,
		Method: "prompts/get",
		Params: mustRaw(t, map[string]any{
			"name": "scene-review",
		}),
	})
	if err != nil {
		t.Fatalf("prompts/get failed: %v", err)
	}
	assertNotSupportedError(t, getRespAny)
}

func newTestStdioServer(promptCatalogEnabled bool) *StdioServer {
	toolManager := tools.NewManager()
	toolManager.RegisterDefaultTools()
	server := NewStdioServer(toolManager)
	server.AttachPromptCatalog(promptcatalog.NewRegistry(promptCatalogEnabled))
	return server
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
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return out
}

func assertNotSupportedError(t *testing.T, response any) {
	t.Helper()
	resp, ok := response.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected jsonrpc response, got %T", response)
	}
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error response")
	}
	if resp.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected code %d, got %d", int(jsonrpc.ErrMethodNotFound), resp.Error.Code)
	}
	data := mustMap(t, resp.Error.Data)
	if data["kind"] != "not_supported" {
		t.Fatalf("expected kind not_supported, got %v", data["kind"])
	}
}
