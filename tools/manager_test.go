package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
)

func TestMain(m *testing.M) {
	// Initialize logger for tests
	logger.Init(logger.GetLevelFromString("debug"), logger.FormatJSON, "logs/tools_test.log")

	// Run tests
	m.Run()
}

// TestTool implements Tool interface for testing
type TestTool struct {
	name        string
	description string
	schema      mcp.InputSchema
	executor    func(args json.RawMessage) ([]byte, error)
}

type outputSchemaTestTool struct {
	*TestTool
	outputSchema map[string]any
}

func (t *outputSchemaTestTool) OutputSchema() map[string]any { return t.outputSchema }

func (t *TestTool) Name() string {
	return t.name
}

func (t *TestTool) Description() string {
	return t.description
}

func (t *TestTool) InputSchema() mcp.InputSchema {
	return t.schema
}

func (t *TestTool) Execute(args json.RawMessage) ([]byte, error) {
	return t.executor(args)
}

func TestToolManager(t *testing.T) {
	// Create a tool manager
	manager := NewManager()

	// Test tool registration
	testTool := &TestTool{
		name:        "godot.test.echo",
		description: "Test tool",
		schema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		},
		executor: func(args json.RawMessage) ([]byte, error) {
			result := "test result"
			return json.Marshal(result)
		},
	}
	manager.RegisterTool(testTool)

	// Test tool execution
	result, err := manager.CallTool("godot.test.echo", map[string]any{})
	if err != nil {
		t.Errorf("CallTool failed: %v", err)
	}
	if result != "test result" {
		t.Errorf("Expected 'test result', got %v", result)
	}

	// Test non-existent tool
	_, err = manager.CallTool("godot.test.missing", map[string]any{})
	if err == nil {
		t.Error("Expected error for non-existent tool")
	}

	// Test tool error handling
	errorTool := &TestTool{
		name:        "godot.test.error",
		description: "Error tool",
		schema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		},
		executor: func(args json.RawMessage) ([]byte, error) {
			return nil, fmt.Errorf("test error")
		},
	}
	manager.RegisterTool(errorTool)

	_, err = manager.CallTool("godot.test.error", map[string]any{})
	if err == nil {
		t.Error("Expected error from errorTool")
	}
}

func TestGetToolsPreservesOptionalOutputSchema(t *testing.T) {
	manager := NewManager()
	want := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
	}
	tool := &outputSchemaTestTool{
		TestTool: &TestTool{
			name:        "godot.test.output",
			description: "Output schema tool",
			schema:      mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}},
			executor:    func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
		},
		outputSchema: want,
	}
	if err := manager.RegisterTool(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	got := manager.GetTools()
	if len(got) != 1 {
		t.Fatalf("expected one tool, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].OutputSchema, want) {
		t.Fatalf("outputSchema changed: got %#v want %#v", got[0].OutputSchema, want)
	}
}

func TestManagerWithNameValidatorKeepsFixtureNamesOptIn(t *testing.T) {
	fixtureManager := NewManagerWithNameValidator(func(name string) bool { return strings.HasPrefix(name, "test_") })
	fixtureTool := &TestTool{
		name:        "test_simple_text",
		description: "Fixture only",
		schema:      mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}},
		executor:    func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
	}
	if err := fixtureManager.RegisterTool(fixtureTool); err != nil {
		t.Fatalf("fixture manager rejected fixture name: %v", err)
	}
	if err := NewManager().RegisterTool(fixtureTool); err == nil {
		t.Fatal("production manager accepted fixture name")
	}
}

func TestRegisterToolRejectsInvalidHeaderAnnotations(t *testing.T) {
	for _, test := range []struct {
		name       string
		properties map[string]any
	}{
		{name: "number is not an allowed header primitive", properties: map[string]any{"value": map[string]any{"type": "number", "x-mcp-header": "Value"}}},
		{name: "header suffix must be tchar", properties: map[string]any{"value": map[string]any{"type": "string", "x-mcp-header": "Bad Header"}}},
		{name: "header suffixes are case-insensitively unique", properties: map[string]any{
			"first":  map[string]any{"type": "string", "x-mcp-header": "Value"},
			"second": map[string]any{"type": "string", "x-mcp-header": "value"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := &TestTool{
				name:        "godot.test.invalid.header",
				description: "Invalid header schema",
				schema:      mcp.InputSchema{Type: "object", Properties: test.properties, Required: []string{}},
				executor:    func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
			}
			if err := NewManager().RegisterTool(tool); err == nil {
				t.Fatal("expected invalid x-mcp-header schema to be rejected")
			}
		})
	}
}

func TestRegisterToolAcceptsHeaderAnnotationAlongNestedPropertiesPath(t *testing.T) {
	tool := &TestTool{
		name:        "godot.test.nested.header",
		description: "Nested header schema",
		schema: mcp.InputSchema{Type: "object", Properties: map[string]any{
			"outer": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"inner": map[string]any{"type": "string", "x-mcp-header": "Inner"},
				},
			},
		}},
		executor: func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
	}
	if err := NewManager().RegisterTool(tool); err != nil {
		t.Fatalf("expected statically reachable nested annotation to be accepted: %v", err)
	}
}

func TestRegisterToolRejectsHeaderAnnotationOutsidePropertiesPath(t *testing.T) {
	tool := &TestTool{
		name: "godot.test.invalid.nested.header", description: "Invalid nested header schema",
		schema: mcp.InputSchema{Type: "object", Properties: map[string]any{
			"values": map[string]any{"type": "array", "items": map[string]any{"type": "string", "x-mcp-header": "Value"}},
		}},
		executor: func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
	}
	if err := NewManager().RegisterTool(tool); err == nil {
		t.Fatal("expected annotation under items to be rejected")
	}
}

func TestRegisterToolRejectsDuplicateHeaderSuffixAcrossNestedProperties(t *testing.T) {
	tool := &TestTool{
		name: "godot.test.duplicate.nested.header", description: "Duplicate nested header schema",
		schema: mcp.InputSchema{Type: "object", Properties: map[string]any{
			"top": map[string]any{"type": "string", "x-mcp-header": "City"},
			"address": map[string]any{"type": "object", "properties": map[string]any{
				"city": map[string]any{"type": "string", "x-mcp-header": "city"},
			}},
		}},
		executor: func(json.RawMessage) ([]byte, error) { return []byte(`{}`), nil },
	}
	if err := NewManager().RegisterTool(tool); err == nil {
		t.Fatal("expected globally duplicated header suffix to be rejected")
	}
}

func TestConcurrentToolExecution(t *testing.T) {
	manager := NewManager()

	// Register a tool that takes some time to execute
	slowTool := &TestTool{
		name:        "godot.test.slow",
		description: "Slow tool",
		schema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		},
		executor: func(args json.RawMessage) ([]byte, error) {
			time.Sleep(100 * time.Millisecond)
			result := "slow result"
			return json.Marshal(result)
		},
	}
	manager.RegisterTool(slowTool)

	// Test concurrent execution
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			result, err := manager.CallTool("godot.test.slow", map[string]any{})
			if err != nil {
				t.Errorf("Concurrent CallTool failed: %v", err)
			}
			if result != "slow result" {
				t.Errorf("Expected 'slow result', got %v", result)
			}
		})
	}
	wg.Wait()
}

func TestSummarizeToolArgsForLog(t *testing.T) {
	if got := summarizeToolArgsForLog("godot.bridge.editor.ping", json.RawMessage(`{"k":"v"}`)); got == `{"k":"v"}` {
		t.Fatalf("expected ping args to be summarized, got raw payload")
	}

	if got := summarizeToolArgsForLog("other-tool", json.RawMessage(`{"k":"v"}`)); got != `{"k":"v"}` {
		t.Fatalf("expected small payload to stay inline, got %q", got)
	}

	large := make([]byte, 513)
	for i := range large {
		large[i] = 'a'
	}
	if got := summarizeToolArgsForLog("other-tool", json.RawMessage(large)); !strings.HasPrefix(got, "<omitted:") {
		t.Fatalf("expected large payload summary, got %q", got)
	}
}
