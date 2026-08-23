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
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/slighter12/godot-mcp-go/config"
	"github.com/slighter12/godot-mcp-go/internal/infra/notifications"
	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	"github.com/slighter12/godot-mcp-go/tools"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
	"github.com/slighter12/godot-mcp-go/transport/shared"
	"github.com/slighter12/godot-mcp-go/transport/stdio"
)

type Server struct {
	promptCatalog       *promptcatalog.Registry
	toolManager         *tools.Manager
	subscriptionManager *SubscriptionManager
	config              *config.Config
	echo                *echo.Echo
	progressMu          sync.RWMutex
	progressStreams     map[string]*progressStreamRecord
	streamRouteSequence atomic.Uint64

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

	releaseNotificationSender func()
	releaseProgressNotifier   func()
	stdioServer               *stdio.StdioServer
}

func NewServer(cfg *config.Config) *Server {
	server := &Server{
		toolManager:         tools.NewManager(),
		subscriptionManager: NewSubscriptionManager(),
		config:              cfg,
		echo:                echo.New(),
		progressStreams:     make(map[string]*progressStreamRecord),
	}
	runtimebridge.DefaultEditorStore().ConfigureFreshness(
		time.Duration(cfg.RuntimeBridge.StaleAfterSeconds)*time.Second,
		time.Duration(cfg.RuntimeBridge.StaleGraceMS)*time.Millisecond,
	)
	runtimebridge.DefaultRuntimeSnapshotStore().ConfigureFreshness(
		time.Duration(cfg.RuntimeBridge.StaleAfterSeconds)*time.Second,
		time.Duration(cfg.RuntimeBridge.StaleGraceMS)*time.Millisecond,
	)
	server.releaseNotificationSender = runtimebridge.RegisterNotificationSender(server.SendJSONRPCNotificationToEditor)
	server.releaseProgressNotifier = tooltypes.RegisterRuntimeCommandProgressNotifier(server.SendRuntimeCommandProgressNotification)
	return server
}

// nextStreamRouteKey returns a server-owned key for an HTTP stream. Client
// JSON-RPC ids are scoped to the client connection and must not be used as
// keys in the server-wide progress or subscription maps.
func (s *Server) nextStreamRouteKey(kind string) string {
	if s == nil || strings.TrimSpace(kind) == "" {
		return ""
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(kind), s.streamRouteSequence.Add(1))
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
		AllowMethods: []string{http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			"MCP-Protocol-Version",
			"Mcp-Method",
			"Mcp-Name",
		},
	}))
	RegisterRoutes(s.echo, s)
}

func (s *Server) startStdioServer() error {
	logger.Info("Starting MCP server in stdio mode", "config", s.config)
	server := stdio.NewStdioServer(s.toolManager)
	s.stdioServer = server
	server.AttachPromptCatalog(s.promptCatalog)
	server.AttachPromptRenderOptions(s.promptRenderOptions())
	server.AttachToolCallOptions(s.toolCallOptions())
	releaseNotificationSender := runtimebridge.RegisterNotificationSender(server.SendJSONRPCNotificationToEditor)
	defer releaseNotificationSender()
	releaseProgressNotifier := tooltypes.RegisterRuntimeCommandProgressNotifier(server.SendRuntimeCommandProgressNotification)
	defer releaseProgressNotifier()
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
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/progress",
		"params":  notifications.ProgressParams(event.ProgressToken, event.Progress, event.Message),
	}
	progressRouteKey := strings.TrimSpace(event.ProgressRouteKey)
	if progressRouteKey != "" {
		if s.isCanceledProgressRequest(progressRouteKey) || s.sendRequestProgress(progressRouteKey, notification) {
			return
		}
		// A non-empty route key belongs to a request-scoped HTTP progress
		// stream. If that stream is gone, do not leak late progress to an
		// editor subscription.
		return
	}
	_ = s.SendJSONRPCNotificationToEditor(event.SessionID, notification)
}

// SendJSONRPCNotificationToEditor delivers an extension notification to the
// subscription owned by an explicit Godot editor session handle.
func (s *Server) SendJSONRPCNotificationToEditor(editorSessionID string, message map[string]any) bool {
	if s == nil || s.subscriptionManager == nil {
		return false
	}
	editorSessionID = strings.TrimSpace(editorSessionID)
	if s.subscriptionManager.SendToEditor(editorSessionID, message) {
		return true
	}
	return false
}

// Shutdown stops background prompt watchers, closes active subscriptions with
// a terminal result, and shuts down the HTTP listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopPromptCatalogWatchers()
	if s.releaseNotificationSender != nil {
		s.releaseNotificationSender()
	}
	if s.releaseProgressNotifier != nil {
		s.releaseProgressNotifier()
	}
	if s.subscriptionManager != nil {
		s.subscriptionManager.closeAll(ctx)
	}
	s.progressMu.Lock()
	progressStreams := make([]*StreamableHTTPTransport, 0, len(s.progressStreams))
	for routeKey, record := range s.progressStreams {
		if record != nil && record.transport != nil {
			progressStreams = append(progressStreams, record.transport)
		}
		delete(s.progressStreams, routeKey)
	}
	s.progressMu.Unlock()
	for _, transport := range progressStreams {
		_ = transport.Close()
	}
	if s.echo == nil {
		return nil
	}
	return s.echo.Shutdown(ctx)
}
