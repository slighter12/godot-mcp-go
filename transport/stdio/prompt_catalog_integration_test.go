package stdio

import (
	"testing"

	"github.com/slighter12/godot-mcp-go/internal/protocol/mcpv20260728"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/transport/shared"
)

func TestModernStdioPromptsFlow(t *testing.T) {
	server := newTestStdioServer(true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "Review a scene",
		Template:    "Review {{scene_path}}",
	})
	options := modernStdioClientOptions{EditorSessionID: "editor-stdio"}

	list := dispatchModernTest(t, server, 1, "prompts/list", map[string]any{}, options)
	if list.Error != nil {
		t.Fatalf("prompts/list error: %#v", list.Error)
	}
	listResult := mustMap(t, list.Result)
	if listResult["resultType"] != "complete" || listResult["cacheScope"] != "public" {
		t.Fatalf("unexpected prompts/list result: %#v", listResult)
	}

	get := dispatchModernTest(t, server, 2, "prompts/get", map[string]any{
		"name":      "scene-review",
		"arguments": map[string]any{"scene_path": "res://Main.tscn"},
	}, options)
	if get.Error != nil {
		t.Fatalf("prompts/get error: %#v", get.Error)
	}
	getResult := mustMap(t, get.Result)
	if getResult["resultType"] != "complete" || getResult["cacheScope"] != "public" {
		t.Fatalf("unexpected prompts/get result: %#v", getResult)
	}
}

func TestModernStdioPromptValidation(t *testing.T) {
	server := newTestStdioServer(true)
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:     "scene-review",
		Template: "Review {{scene_path}}",
	})
	response := dispatchModernTest(t, server, 1, "prompts/get", map[string]any{
		"name":      "scene-review",
		"arguments": map[string]any{"scene_path": 42},
	}, modernStdioClientOptions{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if data["field"] != "arguments" || data["problem"] != "invalid_type" {
		t.Fatalf("unexpected validation data: %#v", data)
	}
}

func TestModernStdioPromptsDisabledUsesMethodNotFound(t *testing.T) {
	server := newTestStdioServer(false)
	response := dispatchModernTest(t, server, 1, "prompts/list", map[string]any{}, modernStdioClientOptions{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected method-not-found error, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if data["kind"] != "not_supported" {
		t.Fatalf("unexpected disabled prompt data: %#v", data)
	}
}

func TestModernStdioStrictPromptValidation(t *testing.T) {
	server := newTestStdioServer(true)
	server.AttachPromptRenderOptions(shared.PromptRenderOptions{Mode: shared.PromptRenderingModeStrict})
	server.promptCatalog.RegisterPrompt(promptcatalog.Prompt{
		Name:     "scene-review",
		Template: "Review {{scene_path}} and {{line}}",
	})
	response := dispatchModernTest(t, server, 1, "prompts/get", map[string]any{
		"name":      "scene-review",
		"arguments": map[string]any{"scene_path": "res://Main.tscn"},
	}, modernStdioClientOptions{})
	if response.Error == nil || response.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected invalid params, got %#v", response)
	}
	data := mustMap(t, response.Error.Data)
	if data["problem"] != "missing_required_arguments" {
		t.Fatalf("unexpected strict validation data: %#v", data)
	}
}

var _ = mcpv20260728.ProtocolVersion
