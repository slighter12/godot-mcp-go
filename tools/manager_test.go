package tools

import (
	"encoding/json"
	"fmt"
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
		name:        "testTool",
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
	result, err := manager.CallTool("testTool", map[string]any{})
	if err != nil {
		t.Errorf("CallTool failed: %v", err)
	}
	if result != "test result" {
		t.Errorf("Expected 'test result', got %v", result)
	}

	// Test non-existent tool
	_, err = manager.CallTool("nonExistentTool", map[string]any{})
	if err == nil {
		t.Error("Expected error for non-existent tool")
	}

	// Test tool error handling
	errorTool := &TestTool{
		name:        "errorTool",
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

	_, err = manager.CallTool("errorTool", map[string]any{})
	if err == nil {
		t.Error("Expected error from errorTool")
	}
}

func TestConcurrentToolExecution(t *testing.T) {
	manager := NewManager()

	// Register a tool that takes some time to execute
	slowTool := &TestTool{
		name:        "slowTool",
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := manager.CallTool("slowTool", map[string]any{})
			if err != nil {
				t.Errorf("Concurrent CallTool failed: %v", err)
			}
			if result != "slow result" {
				t.Errorf("Expected 'slow result', got %v", result)
			}
		}()
	}
	wg.Wait()
}
