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
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Server      Server      `json:"server"`
	Transports  []Transport `json:"transports"`
	Logging     Logging     `json:"logging"`
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

	// Override with environment variables (highest priority)
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
		cfg.Server.Debug = debug == "true"
	}

	if logLevel := os.Getenv("MCP_LOG_LEVEL"); logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	if logPath := os.Getenv("MCP_LOG_PATH"); logPath != "" {
		cfg.Logging.Path = logPath
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(cfg *Config, path string) error {
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
	logLevel := strings.ToLower(strings.TrimSpace(c.Logging.Level))
	if !validLogLevels[logLevel] {
		return errors.New("invalid log level")
	}
	c.Logging.Level = logLevel

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

	// Ensure log directory exists
	logDir := filepath.Dir(c.Logging.Path)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
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

	return nil
}

// GetConfigPath returns the path to the configuration file
func GetConfigPath() (string, error) {
	// First check environment variable
	if path := os.Getenv("MCP_CONFIG_PATH"); path != "" {
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

	configPath := filepath.Join(home, ".godot-mcp", "config/mcp_config.json")

	// Create default config if it doesn't exist
	if _, err := os.Stat(configPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat config file: %w", err)
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create config directory: %w", err)
		}

		// Create default config
		defaultConfig := NewConfig()

		// Write default config
		data, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal default config: %w", err)
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to write default config: %w", err)
		}
	}

	return configPath, nil
}
