package main

import (
	"log"
	"os"

	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/transport/http"
)

func main() {
	// Load configuration
	configPath, err := config.GetConfigPath()
	if err != nil {
		log.Fatalf("Failed to resolve config path: %+v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %+v", err)
	}

	// Check for debug mode
	if os.Getenv("MCP_DEBUG") == "true" {
		cfg.Server.Debug = true
		log.Println("Debug mode enabled via MCP_DEBUG environment variable")
	}

	// Initialize logger
	if err := logger.Init(logger.GetLevelFromString(cfg.Logging.Level), logger.Format(cfg.Logging.Format), cfg.Logging.Path); err != nil {
		log.Fatalf("Failed to initialize logger: %+v", err)
	}

	// Create and start server
	server := http.NewServer(cfg)
	if err := server.Start(); err != nil {
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}
