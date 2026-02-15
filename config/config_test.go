package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg.Name != "godot-mcp-go" {
		t.Errorf("Expected name 'godot-mcp-go', got '%s'", cfg.Name)
	}

	if cfg.Version != "0.1.0" {
		t.Errorf("Expected version '0.1.0', got '%s'", cfg.Version)
	}

	if cfg.Server.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 9080 {
		t.Errorf("Expected port 9080, got %d", cfg.Server.Port)
	}

	if len(cfg.Transports) != 2 {
		t.Errorf("Expected 2 transports, got %d", len(cfg.Transports))
	}

	// Check stdio transport
	if cfg.Transports[0].Type != "stdio" {
		t.Errorf("Expected first transport type 'stdio', got '%s'", cfg.Transports[0].Type)
	}

	if !cfg.Transports[0].Enabled {
		t.Errorf("Expected stdio transport to be enabled")
	}

	// Check streamable_http transport
	if cfg.Transports[1].Type != "streamable_http" {
		t.Errorf("Expected second transport type 'streamable_http', got '%s'", cfg.Transports[1].Type)
	}

	if !cfg.Transports[1].Enabled {
		t.Errorf("Expected streamable_http transport to be enabled")
	}

	if cfg.Transports[1].URL != "http://localhost:9080/mcp" {
		t.Errorf("Expected streamable_http URL 'http://localhost:9080/mcp', got '%s'", cfg.Transports[1].URL)
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	testConfig := `{
		"name": "test-server",
		"version": "1.0.0",
		"description": "Test server",
		"server": {
			"host": "127.0.0.1",
			"port": 8080,
			"debug": true
		},
		"transports": [
			{
				"type": "stdio",
				"enabled": true
			},
			{
				"type": "streamable_http",
				"enabled": true,
				"url": "http://localhost:8080/mcp",
				"headers": {
					"Accept": "application/json, text/event-stream",
					"Content-Type": "application/json"
				}
			}
		],
		"logging": {
			"level": "debug",
			"format": "text",
			"path": "/tmp/test.log"
		}
	}`

	err := os.WriteFile(configPath, []byte(testConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Load the config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config
	if cfg.Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", cfg.Name)
	}

	if cfg.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", cfg.Version)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host '127.0.0.1', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}

	if !cfg.Server.Debug {
		t.Errorf("Expected debug to be true")
	}

	if len(cfg.Transports) != 2 {
		t.Errorf("Expected 2 transports, got %d", len(cfg.Transports))
	}

	// Check stdio transport
	if cfg.Transports[0].Type != "stdio" {
		t.Errorf("Expected first transport type 'stdio', got '%s'", cfg.Transports[0].Type)
	}

	// Check streamable_http transport
	if cfg.Transports[1].Type != "streamable_http" {
		t.Errorf("Expected second transport type 'streamable_http', got '%s'", cfg.Transports[1].Type)
	}

	if cfg.Transports[1].URL != "http://localhost:8080/mcp" {
		t.Errorf("Expected streamable_http URL 'http://localhost:8080/mcp', got '%s'", cfg.Transports[1].URL)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected logging level 'debug', got '%s'", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "text" {
		t.Errorf("Expected logging format 'text', got '%s'", cfg.Logging.Format)
	}

	if cfg.Logging.Path != "/tmp/test.log" {
		t.Errorf("Expected logging path '/tmp/test.log', got '%s'", cfg.Logging.Path)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error when loading non-existent config file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	// Create a temporary config file with invalid JSON
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid_config.json")

	invalidConfig := `{
		"name": "test-server",
		"version": "1.0.0",
		"server": {
			"host": "localhost",
			"port": 8080
		},
		"transports": [
			{
				"type": "stdio",
				"enabled": true
			}
		],
		"logging": {
			"level": "info",
			"format": "json",
			"path": "/tmp/test.log"
		}
	}`

	err := os.WriteFile(configPath, []byte(invalidConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Load the config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config
	if cfg.Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", cfg.Name)
	}

	if cfg.Server.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestGetConfigPath(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("Expected no error resolving config path, got %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty config path")
	}

	// Check if the path ends with the expected filename
	expectedFilename := "mcp_config.json"
	if filepath.Base(path) != expectedFilename {
		t.Errorf("Expected config filename '%s', got '%s'", expectedFilename, filepath.Base(path))
	}
}

func TestSaveConfig(t *testing.T) {
	// Create a test config
	cfg := &Config{
		Name:        "test-save",
		Version:     "2.0.0",
		Description: "Test save config",
		Server: Server{
			Host:  "localhost",
			Port:  9090,
			Debug: true,
		},
		Transports: []Transport{
			{
				Type:    "stdio",
				Enabled: true,
			},
			{
				Type:    "streamable_http",
				Enabled: true,
				URL:     "http://localhost:9090/mcp",
				Headers: map[string]string{
					"Accept":       "application/json, text/event-stream",
					"Content-Type": "application/json",
				},
			},
		},
		Logging: Logging{
			Level:  "debug",
			Format: "json",
			Path:   "/tmp/save_test.log",
		},
	}

	// Create a temporary directory for the test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "save_test_config.json")

	// Save the config
	err := SaveConfig(cfg, configPath)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load the saved config to verify
	loadedCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	// Verify the loaded config matches the original
	if loadedCfg.Name != cfg.Name {
		t.Errorf("Expected name '%s', got '%s'", cfg.Name, loadedCfg.Name)
	}

	if loadedCfg.Version != cfg.Version {
		t.Errorf("Expected version '%s', got '%s'", cfg.Version, loadedCfg.Version)
	}

	if loadedCfg.Server.Host != cfg.Server.Host {
		t.Errorf("Expected host '%s', got '%s'", cfg.Server.Host, loadedCfg.Server.Host)
	}

	if loadedCfg.Server.Port != cfg.Server.Port {
		t.Errorf("Expected port %d, got %d", cfg.Server.Port, loadedCfg.Server.Port)
	}

	if loadedCfg.Server.Debug != cfg.Server.Debug {
		t.Errorf("Expected debug %v, got %v", cfg.Server.Debug, loadedCfg.Server.Debug)
	}

	if len(loadedCfg.Transports) != len(cfg.Transports) {
		t.Errorf("Expected %d transports, got %d", len(cfg.Transports), len(loadedCfg.Transports))
	}

	// Check stdio transport
	if loadedCfg.Transports[0].Type != cfg.Transports[0].Type {
		t.Errorf("Expected first transport type '%s', got '%s'", cfg.Transports[0].Type, loadedCfg.Transports[0].Type)
	}

	// Check streamable_http transport
	if loadedCfg.Transports[1].Type != cfg.Transports[1].Type {
		t.Errorf("Expected second transport type '%s', got '%s'", cfg.Transports[1].Type, loadedCfg.Transports[1].Type)
	}

	if loadedCfg.Transports[1].URL != cfg.Transports[1].URL {
		t.Errorf("Expected streamable_http URL '%s', got '%s'", cfg.Transports[1].URL, loadedCfg.Transports[1].URL)
	}

	if loadedCfg.Logging.Level != cfg.Logging.Level {
		t.Errorf("Expected logging level '%s', got '%s'", cfg.Logging.Level, loadedCfg.Logging.Level)
	}

	if loadedCfg.Logging.Format != cfg.Logging.Format {
		t.Errorf("Expected logging format '%s', got '%s'", cfg.Logging.Format, loadedCfg.Logging.Format)
	}

	if loadedCfg.Logging.Path != cfg.Logging.Path {
		t.Errorf("Expected logging path '%s', got '%s'", cfg.Logging.Path, loadedCfg.Logging.Path)
	}
}
