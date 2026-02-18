package config

import (
	"fmt"
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

	if cfg.PromptCatalog.AutoReload.Enabled {
		t.Errorf("Expected prompt catalog auto reload to be disabled by default")
	}
	if cfg.PromptCatalog.AutoReload.IntervalSeconds != 5 {
		t.Errorf("Expected auto reload interval 5, got %d", cfg.PromptCatalog.AutoReload.IntervalSeconds)
	}
	if cfg.PromptCatalog.Rendering.Mode != "legacy" {
		t.Errorf("Expected rendering mode legacy, got %q", cfg.PromptCatalog.Rendering.Mode)
	}
	if cfg.PromptCatalog.Rendering.RejectUnknownArguments {
		t.Errorf("Expected reject_unknown_arguments to be false by default")
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")
	logPath := filepath.Join(tempDir, "test.log")

	testConfig := fmt.Sprintf(`{
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
			"path": %q
		}
	}`, logPath)

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

	if cfg.Logging.Path != logPath {
		t.Errorf("Expected logging path '%s', got '%s'", logPath, cfg.Logging.Path)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error when loading non-existent config file")
	}
}

func TestLoadConfigPartialJSON(t *testing.T) {
	// Create a temporary config file with partial but valid JSON.
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "partial_config.json")
	logPath := filepath.Join(tempDir, "partial_test.log")

	invalidConfig := fmt.Sprintf(`{
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
			"path": %q
		}
	}`, logPath)

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

func TestLoadConfigMalformedJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "malformed_config.json")

	malformedConfig := `{
		"name": "test-server",
		"version": "1.0.0",
		"server": {
			"host": "localhost",
			"port": 8080,
		}
	}`

	err := os.WriteFile(configPath, []byte(malformedConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write malformed config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error when loading malformed JSON config")
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

func TestGetConfigPathNoSideEffects(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nested", "missing_config.json")
	t.Setenv("MCP_CONFIG_PATH", missingPath)

	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("Expected no error resolving config path, got %v", err)
	}
	if path != missingPath {
		t.Fatalf("Expected MCP_CONFIG_PATH '%s', got '%s'", missingPath, path)
	}

	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("Expected GetConfigPath to have no side effects, stat err=%v", err)
	}
}

func TestEnsureDefaultConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nested", "mcp_config.json")
	if err := EnsureDefaultConfig(configPath); err != nil {
		t.Fatalf("EnsureDefaultConfig failed: %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Expected default config file to exist, stat err=%v", err)
	}
}

func TestValidateHasNoFilesystemSideEffects(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "logs", "app.log")

	cfg := NewConfig()
	cfg.Logging.Path = logPath
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate should succeed, got: %v", err)
	}

	logDir := filepath.Dir(logPath)
	if _, err := os.Stat(logDir); !os.IsNotExist(err) {
		t.Fatalf("Validate should not create log directory, stat err=%v", err)
	}
}

func TestSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	saveLogPath := filepath.Join(tempDir, "save_test.log")

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
			Path:   saveLogPath,
		},
	}

	// Create a temporary directory for the test
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

	if loadedCfg.PromptCatalog.AutoReload.IntervalSeconds != 5 {
		t.Errorf("Expected normalized default auto reload interval 5, got %d", loadedCfg.PromptCatalog.AutoReload.IntervalSeconds)
	}
}

func TestLoadConfigPromptCatalogEnvOverrides(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "env_overrides_config.json")
	logPath := filepath.Join(tempDir, "test.log")

	testConfig := fmt.Sprintf(`{
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
			}
		],
		"logging": {
			"level": "debug",
			"format": "text",
			"path": %q
		},
		"prompt_catalog": {
			"enabled": true,
			"paths": [" /a ", "/a", "/b"],
			"allowed_roots": ["/base"]
		}
	}`, logPath)

	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	t.Setenv("MCP_PROMPT_CATALOG_ALLOWED_ROOTS", "/override-a, /override-b")
	t.Setenv("MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED", "true")
	t.Setenv("MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS", "9")
	t.Setenv("MCP_PROMPT_CATALOG_RENDERING_MODE", "STRICT")
	t.Setenv("MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS", "true")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.PromptCatalog.Paths) != 2 || cfg.PromptCatalog.Paths[0] != "/a" || cfg.PromptCatalog.Paths[1] != "/b" {
		t.Fatalf("Unexpected normalized prompt catalog paths: %#v", cfg.PromptCatalog.Paths)
	}
	if len(cfg.PromptCatalog.AllowedRoots) != 2 || cfg.PromptCatalog.AllowedRoots[0] != "/override-a" || cfg.PromptCatalog.AllowedRoots[1] != "/override-b" {
		t.Fatalf("Unexpected allowed roots: %#v", cfg.PromptCatalog.AllowedRoots)
	}
	if !cfg.PromptCatalog.AutoReload.Enabled {
		t.Fatalf("Expected auto reload enabled")
	}
	if cfg.PromptCatalog.AutoReload.IntervalSeconds != 9 {
		t.Fatalf("Expected interval 9, got %d", cfg.PromptCatalog.AutoReload.IntervalSeconds)
	}
	if cfg.PromptCatalog.Rendering.Mode != "strict" {
		t.Fatalf("Expected rendering mode strict, got %q", cfg.PromptCatalog.Rendering.Mode)
	}
	if !cfg.PromptCatalog.Rendering.RejectUnknownArguments {
		t.Fatalf("Expected reject unknown arguments true")
	}
}

func TestPromptCatalogNormalizeAndValidate(t *testing.T) {
	cfg := NewConfig()
	cfg.PromptCatalog.AllowedRoots = []string{" /a ", "/a", "", "/b "}
	cfg.PromptCatalog.Rendering.Mode = " STRICT "
	cfg.PromptCatalog.AutoReload.IntervalSeconds = 0
	cfg.Normalize()
	if len(cfg.PromptCatalog.AllowedRoots) != 2 || cfg.PromptCatalog.AllowedRoots[0] != "/a" || cfg.PromptCatalog.AllowedRoots[1] != "/b" {
		t.Fatalf("Unexpected normalized allowed roots: %#v", cfg.PromptCatalog.AllowedRoots)
	}
	if cfg.PromptCatalog.Rendering.Mode != "strict" {
		t.Fatalf("Expected strict mode after normalize, got %q", cfg.PromptCatalog.Rendering.Mode)
	}
	if cfg.PromptCatalog.AutoReload.IntervalSeconds != 5 {
		t.Fatalf("Expected default auto reload interval 5 after normalize, got %d", cfg.PromptCatalog.AutoReload.IntervalSeconds)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Expected prompt catalog config to validate, got %v", err)
	}
}

func TestSaveConfigRejectsNilConfig(t *testing.T) {
	if err := SaveConfig(nil, filepath.Join(t.TempDir(), "config.json")); err == nil {
		t.Fatal("expected nil config error")
	}
}

func TestValidateRejectsInvalidPromptCatalogRenderingMode(t *testing.T) {
	cfg := NewConfig()
	cfg.PromptCatalog.Rendering.Mode = "unknown"
	cfg.Normalize()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Expected validation error for invalid rendering mode")
	}
}

func TestValidateRejectsInvalidPromptCatalogAutoReloadInterval(t *testing.T) {
	cfg := NewConfig()
	cfg.PromptCatalog.AutoReload.IntervalSeconds = 1
	cfg.Normalize()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Expected validation error for invalid auto reload interval")
	}

	cfg = NewConfig()
	cfg.PromptCatalog.AutoReload.IntervalSeconds = 301
	cfg.Normalize()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Expected validation error for invalid auto reload interval")
	}
}
