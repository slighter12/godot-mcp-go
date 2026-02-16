package shared

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
)

func TestBuildPromptsListResponse_NotSupported(t *testing.T) {
	req := mustRequest(t, "prompts/list", map[string]any{})
	resp := BuildPromptsListResponse(req, nil)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != int(jsonrpc.ErrMethodNotFound) {
		t.Fatalf("expected %d, got %d", int(jsonrpc.ErrMethodNotFound), resp.Error.Code)
	}
	assertErrorKindFeature(t, resp.Error.Data, "not_supported", "prompt_catalog")
}

func TestServerCapabilities_PromptsCapabilityToggle(t *testing.T) {
	withPrompts := ServerCapabilities(true)
	if _, ok := withPrompts["prompts"]; !ok {
		t.Fatal("expected prompts capability when prompt catalog is enabled")
	}

	withoutPrompts := ServerCapabilities(false)
	if _, ok := withoutPrompts["prompts"]; ok {
		t.Fatal("did not expect prompts capability when prompt catalog is disabled")
	}
}

func TestBuildPromptsListResponse_NotAvailable(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	missing := filepath.Join(t.TempDir(), "missing", "SKILL.md")
	if err := catalog.LoadFromPaths([]string{missing}); err == nil {
		t.Fatal("expected loading error")
	}

	req := mustRequest(t, "prompts/list", map[string]any{})
	resp := BuildPromptsListResponse(req, catalog)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != int(jsonrpc.ErrServerError) {
		t.Fatalf("expected %d, got %d", int(jsonrpc.ErrServerError), resp.Error.Code)
	}
	assertErrorKindFeature(t, resp.Error.Data, "not_available", "prompt_catalog")
}

func TestBuildPromptsListResponse_Pagination(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	for i := range pageSize + 1 {
		catalog.RegisterPrompt(promptcatalog.Prompt{
			Name:        fmt.Sprintf("prompt-%02d", i),
			Description: "desc",
			Template:    "body",
		})
	}

	first := BuildPromptsListResponse(mustRequest(t, "prompts/list", map[string]any{}), catalog)
	firstResult := mustResultMap(t, first)
	firstPrompts := mustPromptList(t, firstResult)
	if len(firstPrompts) != pageSize {
		t.Fatalf("expected %d prompts, got %d", pageSize, len(firstPrompts))
	}
	if firstResult["nextCursor"] != fmt.Sprintf("%d", pageSize) {
		t.Fatalf("expected nextCursor=%d, got %v", pageSize, firstResult["nextCursor"])
	}

	second := BuildPromptsListResponse(mustRequest(t, "prompts/list", map[string]any{"cursor": fmt.Sprintf("%d", pageSize)}), catalog)
	secondResult := mustResultMap(t, second)
	secondPrompts := mustPromptList(t, secondResult)
	if len(secondPrompts) != 1 {
		t.Fatalf("expected 1 prompt on second page, got %d", len(secondPrompts))
	}
	if _, hasNext := secondResult["nextCursor"]; hasNext {
		t.Fatal("did not expect nextCursor on last page")
	}
}

func TestBuildPromptsGetResponse_RenderTemplate(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "scene-review",
		Description: "desc",
		Template:    "Review {{scene_path}}",
	})

	req := mustRequest(t, "prompts/get", map[string]any{
		"name":      "scene-review",
		"arguments": map[string]any{"scene_path": "res://Main.tscn"},
	})
	resp := BuildPromptsGetResponse(req, catalog)
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected success response, got %+v", resp)
	}
	result := mustResultMap(t, resp)
	if result["name"] != "scene-review" {
		t.Fatalf("expected name scene-review, got %v", result["name"])
	}
	messages, ok := result["messages"].([]map[string]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message, got %T %v", result["messages"], result["messages"])
	}
	content, ok := messages[0]["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected message content map, got %T", messages[0]["content"])
	}
	if content["text"] != "Review res://Main.tscn" {
		t.Fatalf("expected rendered prompt text, got %v", content["text"])
	}
}

func TestBuildPromptsGetResponse_NoRecursivePlaceholderExpansion(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "chain-render",
		Description: "desc",
		Template:    "A={{a}};B={{b}}",
	})

	req := mustRequest(t, "prompts/get", map[string]any{
		"name": "chain-render",
		"arguments": map[string]any{
			"a": "{{b}}",
			"b": "SAFE",
		},
	})
	resp := BuildPromptsGetResponse(req, catalog)
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected success response, got %+v", resp)
	}
	result := mustResultMap(t, resp)
	messages, ok := result["messages"].([]map[string]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message, got %T %v", result["messages"], result["messages"])
	}
	content, ok := messages[0]["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected message content map, got %T", messages[0]["content"])
	}
	if content["text"] != "A={{b}};B=SAFE" {
		t.Fatalf("expected one-pass rendering without recursive expansion, got %v", content["text"])
	}
}

func TestBuildPromptsGetResponse_RenderNonStringArgumentsAsJSON(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "meta",
		Description: "desc",
		Template:    "Meta={{meta}}",
	})

	req := mustRequest(t, "prompts/get", map[string]any{
		"name": "meta",
		"arguments": map[string]any{
			"meta": map[string]any{"path": "res://Main.tscn", "line": float64(12)},
		},
	})
	resp := BuildPromptsGetResponse(req, catalog)
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected success response, got %+v", resp)
	}
	result := mustResultMap(t, resp)
	messages, ok := result["messages"].([]map[string]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message, got %T %v", result["messages"], result["messages"])
	}
	content, ok := messages[0]["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected message content map, got %T", messages[0]["content"])
	}
	text, ok := content["text"].(string)
	if !ok {
		t.Fatalf("expected rendered content text to be string, got %T", content["text"])
	}
	if !strings.Contains(text, "\"path\":\"res://Main.tscn\"") || !strings.Contains(text, "\"line\":12") {
		t.Fatalf("expected JSON-rendered map argument, got %v", text)
	}
}

func TestBuildPromptsGetResponse_RenderedPromptTooLarge(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	catalog.RegisterPrompt(promptcatalog.Prompt{
		Name:        "oversized",
		Description: "desc",
		Template:    "Payload={{payload}}",
	})

	oversized := strings.Repeat("a", maxRenderedPromptBytes)
	req := mustRequest(t, "prompts/get", map[string]any{
		"name":      "oversized",
		"arguments": map[string]any{"payload": oversized},
	})
	resp := BuildPromptsGetResponse(req, catalog)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected %d, got %d", int(jsonrpc.ErrInvalidParams), resp.Error.Code)
	}

	m, ok := resp.Error.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected error data map, got %T", resp.Error.Data)
	}
	if m["kind"] != "invalid_params" {
		t.Fatalf("expected kind invalid_params, got %v", m["kind"])
	}
	if m["problem"] != "rendered_prompt_too_large" {
		t.Fatalf("expected rendered_prompt_too_large, got %v", m["problem"])
	}
}

func TestBuildPromptsGetResponse_UnknownPrompt(t *testing.T) {
	catalog := promptcatalog.NewRegistry(true)
	req := mustRequest(t, "prompts/get", map[string]any{"name": "missing"})
	resp := BuildPromptsGetResponse(req, catalog)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != int(jsonrpc.ErrInvalidParams) {
		t.Fatalf("expected %d, got %d", int(jsonrpc.ErrInvalidParams), resp.Error.Code)
	}
	assertErrorKindFeature(t, resp.Error.Data, "invalid_params", "")
}

func TestBuildPingResponse_EmptyResultObject(t *testing.T) {
	resp := BuildPingResponse(jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  "ping",
	})
	result := mustResultMap(t, resp)
	if len(result) != 0 {
		t.Fatalf("expected empty result object, got %#v", result)
	}
}

func mustRequest(t *testing.T, method string, params map[string]any) jsonrpc.Request {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		ID:      1,
		Method:  method,
		Params:  raw,
	}
}

func mustResultMap(t *testing.T, resp *jsonrpc.Response) map[string]any {
	t.Helper()
	if resp == nil || resp.Result == nil {
		t.Fatal("expected response result")
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T", resp.Result)
	}
	return result
}

func mustPromptList(t *testing.T, result map[string]any) []map[string]any {
	t.Helper()
	raw, ok := result["prompts"]
	if !ok {
		t.Fatal("missing prompts field")
	}
	list, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", raw)
	}
	return list
}

func assertErrorKindFeature(t *testing.T, data any, expectedKind string, expectedFeature string) {
	t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("expected error data map, got %T", data)
	}
	if m["kind"] != expectedKind {
		t.Fatalf("expected kind %q, got %v", expectedKind, m["kind"])
	}
	if expectedFeature != "" && m["feature"] != expectedFeature {
		t.Fatalf("expected feature %q, got %v", expectedFeature, m["feature"])
	}
}
