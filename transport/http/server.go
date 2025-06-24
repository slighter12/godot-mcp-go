package http

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/tools"
	"github.com/slighter12/godot-mcp-go/transport/stdio"
)

type Server struct {
	registry       *mcp.Registry
	toolManager    *tools.Manager
	sessionManager *SessionManager
	config         *config.Config
	echo           *echo.Echo
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		registry:       mcp.NewRegistry(),
		toolManager:    tools.NewManager(),
		sessionManager: NewSessionManager(),
		config:         cfg,
		echo:           echo.New(),
	}
}

func (s *Server) Start() error {
	s.toolManager.RegisterDefaultTools()
	if err := s.registry.RegisterServer("default", s.toolManager.GetTools()); err != nil {
		logger.Error("Failed to register default server", "error", err)
		return err
	}
	logger.Info("Default server registered successfully", "server_id", "default")
	go s.startCleanupGoroutine()
	s.setupEcho()
	useStdio := os.Getenv("MCP_USE_STDIO") == "true"
	if useStdio {
		return s.startStdioServer()
	} else {
		return s.startStreamableHTTPServer()
	}
}

func (s *Server) setupEcho() {
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
	s.echo.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, "Mcp-Session-Id", "Last-Event-ID"},
	}))
	RegisterRoutes(s.echo, s)
}

func (s *Server) startCleanupGoroutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.registry.Cleanup(10 * time.Minute)
		s.sessionManager.CleanupSessions(10 * time.Minute)
	}
}

func (s *Server) startStdioServer() error {
	logger.Info("Starting MCP server in stdio mode", "config", s.config)
	server := stdio.NewStdioServer(s.toolManager)
	return server.Start()
}

func (s *Server) startStreamableHTTPServer() error {
	logger.Info("Starting MCP server in Streamable HTTP mode", "port", s.config.Server.Port)
	logger.Debug("Streamable HTTP server configuration", "config", s.config)
	addr := fmt.Sprintf("[::]:%d", s.config.Server.Port)
	logger.Info("Streamable HTTP server starting to listen", "address", addr)
	return s.echo.Start(addr)
}

func (s *Server) GetRegistry() *mcp.Registry {
	return s.registry
}
func (s *Server) GetToolManager() *tools.Manager {
	return s.toolManager
}
func (s *Server) GetSessionManager() *SessionManager {
	return s.sessionManager
}
func (s *Server) GetConfig() *config.Config {
	return s.config
}
