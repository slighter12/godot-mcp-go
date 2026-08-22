package main

import (
	"context"
	"errors"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	mcphttp "github.com/slighter12/godot-mcp-go/transport/http"
)

func main() {
	// Load configuration
	configPath, err := config.ResolveConfigPath()
	if err != nil {
		log.Fatalf("Failed to resolve config path: %+v", err)
	}
	if err := config.EnsureDefaultConfig(configPath); err != nil {
		log.Fatalf("Failed to prepare default config: %+v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %+v", err)
	}

	// Initialize logger
	if err := logger.Init(logger.GetLevelFromString(cfg.Logging.Level), logger.Format(cfg.Logging.Format), cfg.Logging.Path); err != nil {
		log.Fatalf("Failed to initialize logger: %+v", err)
	}

	// Create and start server
	server := mcphttp.NewServer(cfg)
	if os.Getenv("MCP_USE_STDIO") == "true" {
		if err := server.Start(); err != nil {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
		return
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
	case sig := <-signals:
		logger.Info("Shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			logger.Error("Server shutdown error", "error", err)
		}
		if err := <-serverErr; err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			logger.Error("Server error during shutdown", "error", err)
			os.Exit(1)
		}
	}
}
