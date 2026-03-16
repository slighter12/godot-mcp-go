package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/transport/shared"
	"github.com/slighter12/godot-mcp-go/transport/stdio"
)

type Server struct {
	registry       *mcp.Registry
	promptCatalog  *promptcatalog.Registry
	toolManager    *tools.Manager
	sessionManager *SessionManager
	config         *config.Config
	echo           *echo.Echo

	promptCatalogReloadMu                   sync.Mutex
	promptCatalogFileFingerprint            string
	promptCatalogSnapshotWarningFingerprint string
	promptCatalogSnapshotWarningLastLogged  time.Time

	promptCatalogAutoReloadMu     sync.Mutex
	promptCatalogAutoReloadCancel context.CancelFunc
	promptCatalogAutoReloadDone   chan struct{}

	promptCatalogEventWatchMu     sync.Mutex
	promptCatalogEventWatchCancel context.CancelFunc
	promptCatalogEventWatchDone   chan struct{}
}

func NewServer(cfg *config.Config) *Server {
	server := &Server{
		registry:       mcp.NewRegistry(),
		toolManager:    tools.NewManager(),
		sessionManager: NewSessionManager(),
		config:         cfg,
		echo:           echo.New(),
	}
	runtimebridge.DefaultStore().ConfigureFreshness(
		time.Duration(cfg.RuntimeBridge.StaleAfterSeconds)*time.Second,
		time.Duration(cfg.RuntimeBridge.StaleGraceMS)*time.Millisecond,
	)
	runtimebridge.SetNotificationSender(server.SendJSONRPCNotificationToSession)
	tooltypes.SetRuntimeCommandProgressNotifier(server.SendRuntimeCommandProgressNotification)
	return server
}

func (s *Server) Start() error {
	s.stopPromptCatalogWatchers()
	s.initializePromptCatalog()
	s.startPromptCatalogWatchers()
	defer s.stopPromptCatalogWatchers()
	useStdio := os.Getenv("MCP_USE_STDIO") == "true"
	if useStdio {
		if err := s.registerStdioBaseTools(); err != nil {
			logger.Error("Failed to register stdio base tools", "error", err)
			return err
		}
	} else {
		s.toolManager.RegisterDefaultTools()
	}
	if err := s.registerRuntimeTools(); err != nil {
		logger.Error("Failed to register runtime tools", "error", err)
		return err
	}
	if err := s.registry.RegisterServer("default", s.toolManager.GetTools()); err != nil {
		logger.Error("Failed to register default server", "error", err)
		return err
	}
	// Mark default server as persistent so it's not cleaned up
	if err := s.registry.SetPersistence("default", true); err != nil {
		logger.Error("Failed to set default server persistence", "error", err)
		return err
	}
	logger.Info("Default server registered successfully", "server_id", "default")
	go s.startCleanupGoroutine()
	s.setupEcho()
	if useStdio {
		return s.startStdioServer()
	} else {
		return s.startStreamableHTTPServer()
	}
}

func (s *Server) registerStdioBaseTools() error {
	s.toolManager = tools.NewManager()
	for _, tool := range tools.GetStdioTools() {
		if err := s.toolManager.RegisterTool(tool); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) setupEcho() {
	s.echo.Use(middleware.Logger())
	s.echo.Use(middleware.Recover())
	s.echo.Use(s.originValidationMiddleware())
	s.echo.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOriginFunc: func(origin string) (bool, error) {
			return s.isAllowedOrigin(origin), nil
		},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			"MCP-Session-Id",
			"MCP-Protocol-Version",
			"Last-Event-ID",
		},
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
	server.AttachPromptCatalog(s.promptCatalog)
	server.AttachPromptRenderOptions(s.promptRenderOptions())
	server.AttachToolCallOptions(s.toolCallOptions())
	return server.Start()
}

func (s *Server) startStreamableHTTPServer() error {
	logger.Info("Starting MCP server in Streamable HTTP mode", "port", s.config.Server.Port)
	logger.Debug("Streamable HTTP server configuration", "config", s.config)
	host := strings.TrimSpace(s.config.Server.Host)
	if host == "" {
		host = "localhost"
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", s.config.Server.Port))
	logger.Info("Streamable HTTP server starting to listen", "address", addr)
	return s.echo.Start(addr)
}

func (s *Server) originValidationMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			origin := c.Request().Header.Get(echo.HeaderOrigin)
			if origin == "" {
				return next(c)
			}
			if !s.isAllowedOrigin(origin) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden origin"})
			}
			return next(c)
		}
	}
}

func (s *Server) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	allowed := map[string]struct{}{
		"localhost": {},
		"127.0.0.1": {},
		"::1":       {},
	}

	cfgHost := strings.ToLower(strings.TrimSpace(s.config.Server.Host))
	if cfgHost != "" && cfgHost != "0.0.0.0" && cfgHost != "::" {
		allowed[cfgHost] = struct{}{}
	}

	_, ok := allowed[host]
	return ok
}

func (s *Server) GetRegistry() *mcp.Registry {
	return s.registry
}

func (s *Server) initializePromptCatalog() {
	s.promptCatalog = promptcatalog.NewRegistry(s.config.PromptCatalog.Enabled)
	if !s.promptCatalog.Enabled() {
		s.promptCatalogReloadMu.Lock()
		s.promptCatalogFileFingerprint = ""
		s.promptCatalogSnapshotWarningFingerprint = ""
		s.promptCatalogSnapshotWarningLastLogged = time.Time{}
		s.promptCatalogReloadMu.Unlock()
		logger.Info("Prompt catalog runtime disabled")
		return
	}

	fingerprint, snapshotErrors, err := s.loadPromptCatalogWithStableSnapshot(nil)
	if err != nil {
		logger.Warn("Prompt catalog loaded with warnings", "error", err)
	}

	s.promptCatalogReloadMu.Lock()
	s.promptCatalogFileFingerprint = fingerprint
	s.logPromptCatalogSnapshotWarningsLocked(snapshotErrors)
	s.promptCatalogReloadMu.Unlock()

	logger.Info("Prompt catalog runtime initialized",
		"enabled", s.promptCatalog.Enabled(),
		"paths", len(s.config.PromptCatalog.Paths),
		"prompts", s.promptCatalog.PromptCount(),
		"load_errors", len(s.promptCatalog.LoadErrors()),
	)
}

func (s *Server) GetToolManager() *tools.Manager {
	return s.toolManager
}

func (s *Server) GetPromptCatalog() *promptcatalog.Registry {
	return s.promptCatalog
}

func (s *Server) GetSessionManager() *SessionManager {
	return s.sessionManager
}
func (s *Server) GetConfig() *config.Config {
	return s.config
}

func (s *Server) promptRenderOptions() shared.PromptRenderOptions {
	if s == nil || s.config == nil {
		return shared.DefaultPromptRenderOptions()
	}
	governanceRoots := make([]shared.PromptGovernanceRoot, 0, len(s.config.PromptCatalog.Governance.Roots))
	for _, root := range s.config.PromptCatalog.Governance.Roots {
		governanceRoots = append(governanceRoots, shared.PromptGovernanceRoot{
			Path: root.Path,
			Tier: root.Tier,
		})
	}
	return shared.PromptRenderOptions{
		Mode:                   s.config.PromptCatalog.Rendering.Mode,
		RejectUnknownArguments: s.config.PromptCatalog.Rendering.RejectUnknownArguments,
		GovernanceRoots:        governanceRoots,
	}
}

func (s *Server) toolCallOptions() shared.ToolCallOptions {
	if s == nil || s.config == nil {
		return shared.DefaultToolCallOptions()
	}
	return shared.ToolCallOptions{
		SchemaValidationEnabled:   s.config.ToolControls.SchemaValidationEnabled,
		RejectUnknownArguments:    s.config.ToolControls.RejectUnknownArguments,
		PermissionMode:            s.config.ToolControls.PermissionMode,
		AllowedTools:              s.config.ToolControls.AllowedTools,
		EmitProgressNotifications: s.config.ToolControls.EmitProgressNotifications,
	}
}

func (s *Server) toolCallContext(sessionID string) shared.ToolCallContext {
	callerSessionID := strings.TrimSpace(sessionID)
	return shared.ToolCallContext{
		SessionID:               callerSessionID,
		RuntimeSessionID:        s.resolveRuntimeReadSessionID(callerSessionID),
		RuntimeCommandSessionID: s.resolveRuntimeCommandSessionID(callerSessionID),
		SessionInitialized:      s.sessionManager.IsInitialized(callerSessionID),
		MutatingAllowed:         s.isMutatingAllowedForSession(callerSessionID),
	}
}

func (s *Server) isMutatingAllowedForSession(sessionID string) bool {
	if s == nil {
		return false
	}
	if s.sessionManager.IsMutatingAllowed(sessionID) {
		return true
	}
	return s.config != nil && s.config.ToolControls.AllowMutatingWithoutCapability
}

func (s *Server) resolveRuntimeReadSessionID(callerSessionID string) string {
	callerSessionID = strings.TrimSpace(callerSessionID)
	if callerSessionID == "" {
		return ""
	}

	now := time.Now().UTC()
	if _, ok, _ := runtimebridge.DefaultStore().FreshForSession(callerSessionID, now); ok {
		return callerSessionID
	}
	if s == nil || s.config == nil || !s.config.RuntimeBridge.AllowLatestSessionFallback {
		return callerSessionID
	}

	latest, ok, _ := runtimebridge.DefaultStore().LatestFresh(now)
	if !ok || strings.TrimSpace(latest.SessionID) == "" {
		return callerSessionID
	}
	return strings.TrimSpace(latest.SessionID)
}

func (s *Server) resolveRuntimeCommandSessionID(callerSessionID string) string {
	callerSessionID = strings.TrimSpace(callerSessionID)
	if callerSessionID == "" {
		return ""
	}
	if s == nil || s.config == nil || !s.config.RuntimeBridge.AllowLatestSessionFallback {
		if _, ok := s.sessionManager.GetTransport(callerSessionID); ok {
			return callerSessionID
		}
		return callerSessionID
	}

	now := time.Now().UTC()
	if _, ok, _ := runtimebridge.DefaultStore().FreshForSession(callerSessionID, now); ok {
		if _, hasTransport := s.sessionManager.GetTransport(callerSessionID); hasTransport {
			return callerSessionID
		}
	}

	if latest, ok, _ := runtimebridge.DefaultStore().LatestFresh(now); ok {
		if runtimeSessionID := strings.TrimSpace(latest.SessionID); runtimeSessionID != "" {
			if _, hasTransport := s.sessionManager.GetTransport(runtimeSessionID); hasTransport {
				return runtimeSessionID
			}
		}
	}

	if _, ok := s.sessionManager.GetTransport(callerSessionID); ok {
		return callerSessionID
	}
	if latestTransportSessionID, ok := s.sessionManager.LatestSessionIDWithTransport(); ok {
		return latestTransportSessionID
	}
	return callerSessionID
}

func (s *Server) SendRuntimeCommandProgressNotification(event tooltypes.RuntimeCommandProgressEvent) {
	if s == nil || s.config == nil || !s.config.ToolControls.EmitProgressNotifications {
		return
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return
	}
	if !notifications.IsValidProgressToken(event.ProgressToken) {
		return
	}
	_ = s.SendJSONRPCNotificationToSession(event.SessionID, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/progress",
		"params":  notifications.ProgressParams(event.ProgressToken, event.Progress, event.Message),
	})
}
