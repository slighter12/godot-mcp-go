package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slighter12/godot-mcp-go/mcp"
)

// Config represents the MCP server configuration
type Config struct {
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Description   string        `json:"description"`
	Server        Server        `json:"server"`
	Transports    []Transport   `json:"transports"`
	Logging       Logging       `json:"logging"`
	PromptCatalog PromptCatalog `json:"prompt_catalog"`
}

// Server represents server configuration
type Server struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Debug bool   `json:"debug"`
}

// Transport represents a transport configuration
type Transport struct {
	Type    string            `json:"type"`
	Enabled bool              `json:"enabled"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Logging represents logging configuration
type Logging struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	Path   string `json:"path"`
}

// PromptCatalog represents prompt catalog runtime configuration.
type PromptCatalog struct {
	Enabled      bool                    `json:"enabled"`
	Paths        []string                `json:"paths"`
	AllowedRoots []string                `json:"allowed_roots"`
	AutoReload   PromptCatalogAutoReload `json:"auto_reload"`
	Rendering    PromptCatalogRendering  `json:"rendering"`
}

// PromptCatalogAutoReload controls polling-based prompt catalog reload behavior.
type PromptCatalogAutoReload struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
}

// PromptCatalogRendering controls prompt template rendering validation behavior.
type PromptCatalogRendering struct {
	Mode                   string `json:"mode"`
	RejectUnknownArguments bool   `json:"reject_unknown_arguments"`
}

// NewConfig creates a new Config with default values
func NewConfig() *Config {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return &Config{
		Name:        "godot-mcp-go",
		Version:     "0.1.0",
		Description: "Go-based Model Context Protocol server for Godot",
		Server: Server{
			Host:  "localhost",
			Port:  9080,
			Debug: false,
		},
		Transports: []Transport{
			{
				Type:    "stdio",
				Enabled: true,
			},
			{
				Type:    "streamable_http",
				Enabled: true,
				URL:     "http://localhost:9080/mcp",
				Headers: map[string]string{
					"Accept":               "application/json, text/event-stream",
					"Content-Type":         "application/json",
					"MCP-Protocol-Version": mcp.ProtocolVersion,
				},
			},
		},
		Logging: Logging{
			Level:  "info",
			Format: "json",
			Path:   filepath.Join(home, ".godot-mcp", "logs", "mcp.log"),
		},
		PromptCatalog: PromptCatalog{
			Enabled:      true,
			Paths:        []string{},
			AllowedRoots: []string{},
			AutoReload: PromptCatalogAutoReload{
				Enabled:         false,
				IntervalSeconds: 5,
			},
			Rendering: PromptCatalogRendering{
				Mode:                   "legacy",
				RejectUnknownArguments: false,
			},
		},
	}
}

// LoadConfig loads the configuration from a file
func LoadConfig(path string) (*Config, error) {
	cfg := NewConfig()

	// Read config file if it exists
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("config file not found: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Override with environment variables (highest priority).
	applyEnvOverrides(cfg)
	cfg.Normalize()

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(cfg *Config, path string) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

func applyEnvOverrides(cfg *Config) {
	if portStr := os.Getenv("MCP_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = port
		} else {
			log.Printf("warning: ignoring invalid MCP_PORT value %q: %v", portStr, err)
		}
	}

	if host := os.Getenv("MCP_HOST"); host != "" {
		cfg.Server.Host = host
	}

	if debug := os.Getenv("MCP_DEBUG"); debug != "" {
		if parsed, err := strconv.ParseBool(debug); err == nil {
			cfg.Server.Debug = parsed
		} else {
			log.Printf("warning: ignoring invalid MCP_DEBUG value %q: %v", debug, err)
		}
	}

	if logLevel := os.Getenv("MCP_LOG_LEVEL"); logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	if logPath := os.Getenv("MCP_LOG_PATH"); logPath != "" {
		cfg.Logging.Path = logPath
	}

	if promptCatalogEnabled := os.Getenv("MCP_PROMPT_CATALOG_ENABLED"); promptCatalogEnabled != "" {
		if parsed, err := strconv.ParseBool(promptCatalogEnabled); err == nil {
			cfg.PromptCatalog.Enabled = parsed
		} else {
			log.Printf("warning: ignoring invalid MCP_PROMPT_CATALOG_ENABLED value %q: %v", promptCatalogEnabled, err)
		}
	}

	if promptCatalogPaths := os.Getenv("MCP_PROMPT_CATALOG_PATHS"); promptCatalogPaths != "" {
		cfg.PromptCatalog.Paths = parseCSV(promptCatalogPaths)
	}

	if promptCatalogAllowedRoots := os.Getenv("MCP_PROMPT_CATALOG_ALLOWED_ROOTS"); promptCatalogAllowedRoots != "" {
		cfg.PromptCatalog.AllowedRoots = parseCSV(promptCatalogAllowedRoots)
	}

	if autoReloadEnabled := os.Getenv("MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED"); autoReloadEnabled != "" {
		if parsed, err := strconv.ParseBool(autoReloadEnabled); err == nil {
			cfg.PromptCatalog.AutoReload.Enabled = parsed
		} else {
			log.Printf("warning: ignoring invalid MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED value %q: %v", autoReloadEnabled, err)
		}
	}

	if autoReloadInterval := os.Getenv("MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS"); autoReloadInterval != "" {
		if parsed, err := strconv.Atoi(autoReloadInterval); err == nil {
			cfg.PromptCatalog.AutoReload.IntervalSeconds = parsed
		} else {
			log.Printf("warning: ignoring invalid MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS value %q: %v", autoReloadInterval, err)
		}
	}

	if renderingMode := os.Getenv("MCP_PROMPT_CATALOG_RENDERING_MODE"); renderingMode != "" {
		cfg.PromptCatalog.Rendering.Mode = renderingMode
	}

	if rejectUnknownArgs := os.Getenv("MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS"); rejectUnknownArgs != "" {
		if parsed, err := strconv.ParseBool(rejectUnknownArgs); err == nil {
			cfg.PromptCatalog.Rendering.RejectUnknownArguments = parsed
		} else {
			log.Printf("warning: ignoring invalid MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS value %q: %v", rejectUnknownArgs, err)
		}
	}
}

// Normalize canonicalizes config values so downstream validation and runtime
// logic operate on stable representations.
func (c *Config) Normalize() {
	c.Server.Host = strings.TrimSpace(c.Server.Host)
	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))
	c.Logging.Format = strings.ToLower(strings.TrimSpace(c.Logging.Format))
	c.Logging.Path = strings.TrimSpace(c.Logging.Path)
	c.PromptCatalog.Paths = normalizePaths(c.PromptCatalog.Paths)
	c.PromptCatalog.AllowedRoots = normalizePaths(c.PromptCatalog.AllowedRoots)
	c.PromptCatalog.Rendering.Mode = strings.ToLower(strings.TrimSpace(c.PromptCatalog.Rendering.Mode))
	if c.PromptCatalog.Rendering.Mode == "" {
		c.PromptCatalog.Rendering.Mode = "legacy"
	}
	if c.PromptCatalog.AutoReload.IntervalSeconds == 0 {
		c.PromptCatalog.AutoReload.IntervalSeconds = 5
	}
	for i := range c.Transports {
		c.Transports[i].Type = strings.ToLower(strings.TrimSpace(c.Transports[i].Type))
		c.Transports[i].URL = strings.TrimSpace(c.Transports[i].URL)
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate server configuration
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("invalid port number")
	}

	if c.Server.Host == "" {
		return errors.New("host cannot be empty")
	}

	// Validate logging configuration
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return errors.New("invalid log level")
	}

	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return errors.New("invalid log format")
	}

	if c.Logging.Path == "" {
		return errors.New("log path cannot be empty")
	}

	// Validate transports
	if len(c.Transports) == 0 {
		return errors.New("at least one transport must be enabled")
	}

	validTransportTypes := map[string]bool{
		"stdio":           true,
		"streamable_http": true,
	}

	enabledTransports := 0
	for _, t := range c.Transports {
		if !validTransportTypes[t.Type] {
			return fmt.Errorf("invalid transport type: %s", t.Type)
		}
		if t.Enabled {
			enabledTransports++
		}
	}

	if enabledTransports == 0 {
		return errors.New("at least one transport must be enabled")
	}

	validRenderingModes := map[string]bool{
		"legacy": true,
		"strict": true,
	}
	if !validRenderingModes[c.PromptCatalog.Rendering.Mode] {
		return fmt.Errorf("invalid prompt catalog rendering mode %q: expected one of [legacy strict]", c.PromptCatalog.Rendering.Mode)
	}

	if c.PromptCatalog.AutoReload.IntervalSeconds < 2 || c.PromptCatalog.AutoReload.IntervalSeconds > 300 {
		return fmt.Errorf(
			"invalid prompt catalog auto reload interval seconds %d: expected range 2..300",
			c.PromptCatalog.AutoReload.IntervalSeconds,
		)
	}

	return nil
}

// ResolveConfigPath returns the path that should be used for configuration.
func ResolveConfigPath() (string, error) {
	// First check environment variable
	if path := strings.TrimSpace(os.Getenv("MCP_CONFIG_PATH")); path != "" {
		return path, nil
	}

	// Then check config/mcp_config.json in current directory
	if _, err := os.Stat("config/mcp_config.json"); err == nil {
		return "config/mcp_config.json", nil
	}

	// Finally check home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, ".godot-mcp", "config", "mcp_config.json"), nil
}

// EnsureDefaultConfig creates a default config file if one does not exist.
func EnsureDefaultConfig(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("config path cannot be empty")
	}

	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	defaultConfig := NewConfig()
	defaultConfig.Normalize()
	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	return nil
}

// GetConfigPath returns the resolved config path.
// Deprecated: use ResolveConfigPath and EnsureDefaultConfig.
func GetConfigPath() (string, error) {
	return ResolveConfigPath()
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
