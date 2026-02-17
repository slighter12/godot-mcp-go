package http

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools/utility"
)

const snapshotWarningHeartbeatInterval = 10 * time.Minute

type promptCatalogSourceSnapshot struct {
	fingerprint string
	warnings    []string
}

func (s *Server) registerRuntimeTools() error {
	return s.toolManager.RegisterTool(utility.NewReloadPromptCatalogTool(s.reloadPromptCatalog))
}

func (s *Server) reloadPromptCatalog() map[string]any {
	s.promptCatalogReloadMu.Lock()
	defer s.promptCatalogReloadMu.Unlock()
	return s.reloadPromptCatalogLocked(nil)
}

func (s *Server) reloadPromptCatalogLocked(snapshot *promptCatalogSourceSnapshot) map[string]any {
	result := map[string]any{
		"changed":        false,
		"promptCount":    0,
		"loadErrorCount": 0,
		"status":         "disabled",
	}

	if s.promptCatalog == nil || !s.promptCatalog.Enabled() {
		s.promptCatalogFileFingerprint = ""
		return result
	}

	beforePrompts := s.promptCatalog.ListPrompts()
	beforeFingerprint := promptListFingerprint(beforePrompts)

	loadErr := s.promptCatalog.LoadFromPathsWithAllowedRoots(s.config.PromptCatalog.Paths, s.config.PromptCatalog.AllowedRoots)

	afterPrompts := s.promptCatalog.ListPrompts()
	afterFingerprint := promptListFingerprint(afterPrompts)
	loadErrors := s.promptCatalog.LoadErrors()

	sourceFingerprint := ""
	sourceFingerprintWarnings := []string(nil)
	if snapshot == nil {
		sourceFingerprint, sourceFingerprintWarnings = promptcatalog.SnapshotFingerprint(s.config.PromptCatalog.Paths, s.config.PromptCatalog.AllowedRoots)
	} else {
		sourceFingerprint = snapshot.fingerprint
		sourceFingerprintWarnings = append([]string(nil), snapshot.warnings...)
	}
	s.promptCatalogFileFingerprint = sourceFingerprint
	changed := beforeFingerprint != afterFingerprint

	status := "ok"
	if len(loadErrors) > 0 || len(sourceFingerprintWarnings) > 0 {
		status = "warning"
	}

	result = map[string]any{
		"changed":        changed,
		"promptCount":    len(afterPrompts),
		"loadErrorCount": len(loadErrors),
		"status":         status,
	}

	allWarnings := append([]string{}, loadErrors...)
	allWarnings = append(allWarnings, sourceFingerprintWarnings...)
	if len(allWarnings) > 0 {
		result["warnings"] = summarizeLoadErrors(allWarnings, 5)
	}
	if loadErr != nil {
		logger.Warn("Prompt catalog reloaded with warnings", "error", loadErr)
	}
	s.logPromptCatalogSnapshotWarningsLocked(sourceFingerprintWarnings)

	if changed {
		notified := s.BroadcastPromptListChanged()
		result["notifiedSessions"] = notified
	}
	return result
}

func (s *Server) startPromptCatalogAutoReload() {
	if s == nil || s.config == nil || s.promptCatalog == nil || !s.promptCatalog.Enabled() {
		return
	}
	if !s.config.PromptCatalog.AutoReload.Enabled {
		return
	}

	intervalSeconds := s.config.PromptCatalog.AutoReload.IntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = 5
	}

	logger.Info("Prompt catalog auto reload enabled",
		"interval_seconds", intervalSeconds,
		"paths", len(s.config.PromptCatalog.Paths),
		"allowed_roots", len(s.config.PromptCatalog.AllowedRoots),
	)

	s.stopPromptCatalogAutoReload()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.promptCatalogAutoReloadMu.Lock()
	s.promptCatalogAutoReloadCancel = cancel
	s.promptCatalogAutoReloadDone = done
	s.promptCatalogAutoReloadMu.Unlock()

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if result, reloaded := s.reloadPromptCatalogIfSourcesChanged(); reloaded {
					logger.Info("Prompt catalog auto reload completed",
						"changed", result["changed"],
						"prompt_count", result["promptCount"],
						"status", result["status"],
					)
				}
			}
		}
	}()
}

func (s *Server) stopPromptCatalogAutoReload() {
	if s == nil {
		return
	}

	s.promptCatalogAutoReloadMu.Lock()
	cancel := s.promptCatalogAutoReloadCancel
	done := s.promptCatalogAutoReloadDone
	s.promptCatalogAutoReloadCancel = nil
	s.promptCatalogAutoReloadDone = nil
	s.promptCatalogAutoReloadMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Server) reloadPromptCatalogIfSourcesChanged() (map[string]any, bool) {
	if s == nil || s.config == nil || s.promptCatalog == nil || !s.promptCatalog.Enabled() {
		return nil, false
	}

	sourceFingerprint, warnings := promptcatalog.SnapshotFingerprint(s.config.PromptCatalog.Paths, s.config.PromptCatalog.AllowedRoots)

	s.promptCatalogReloadMu.Lock()
	defer s.promptCatalogReloadMu.Unlock()
	s.logPromptCatalogSnapshotWarningsLocked(warnings)

	if sourceFingerprint == s.promptCatalogFileFingerprint {
		return nil, false
	}

	return s.reloadPromptCatalogLocked(&promptCatalogSourceSnapshot{
		fingerprint: sourceFingerprint,
		warnings:    warnings,
	}), true
}

func (s *Server) logPromptCatalogSnapshotWarningsLocked(warnings []string) {
	fingerprint := promptCatalogWarningFingerprint(warnings)
	if fingerprint == "" {
		if s.promptCatalogSnapshotWarningFingerprint != "" {
			logger.Info("Prompt catalog source snapshot warnings recovered")
		}
		s.promptCatalogSnapshotWarningFingerprint = ""
		s.promptCatalogSnapshotWarningLastLogged = time.Time{}
		return
	}

	now := time.Now()
	shouldLog := false
	if fingerprint != s.promptCatalogSnapshotWarningFingerprint {
		shouldLog = true
	} else if s.promptCatalogSnapshotWarningLastLogged.IsZero() || now.Sub(s.promptCatalogSnapshotWarningLastLogged) >= snapshotWarningHeartbeatInterval {
		shouldLog = true
	}

	s.promptCatalogSnapshotWarningFingerprint = fingerprint
	if shouldLog {
		s.promptCatalogSnapshotWarningLastLogged = now
		logger.Warn("Prompt catalog source snapshot collected with warnings", "warnings", summarizeLoadErrors(warnings, 5))
	}
}

func promptCatalogWarningFingerprint(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return ""
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\n")
}

func summarizeLoadErrors(loadErrors []string, limit int) []string {
	if len(loadErrors) == 0 {
		return nil
	}
	if limit <= 0 || len(loadErrors) <= limit {
		return append([]string(nil), loadErrors...)
	}
	out := append([]string(nil), loadErrors[:limit]...)
	out = append(out, fmt.Sprintf("... %d more warning(s)", len(loadErrors)-limit))
	return out
}

func (s *Server) BroadcastPromptListChanged() int {
	sessionIDs := s.sessionManager.SessionIDsWithTransport()
	if len(sessionIDs) == 0 {
		return 0
	}

	notification := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"method":  "notifications/prompts/list_changed",
	}

	sent := 0
	for _, sessionID := range sessionIDs {
		if s.SendJSONRPCNotificationToSession(sessionID, notification) {
			sent++
		}
	}
	return sent
}

func (s *Server) SendJSONRPCNotificationToSession(sessionID string, message map[string]any) bool {
	transport, ok := s.sessionManager.GetTransport(sessionID)
	if !ok || transport == nil {
		return false
	}

	if err := transport.SendSSE("message", message); err != nil {
		logger.Warn("Failed to send SSE notification", "session_id", sessionID, "error", err)
		s.sessionManager.ClearTransportIfMatch(sessionID, transport)
		return false
	}
	return true
}

type listPromptDigest struct {
	Name        string                         `json:"name"`
	Title       string                         `json:"title,omitempty"`
	Description string                         `json:"description"`
	Arguments   []promptcatalog.PromptArgument `json:"arguments,omitempty"`
}

func promptListFingerprint(prompts []promptcatalog.Prompt) string {
	if len(prompts) == 0 {
		return "[]"
	}

	digest := make([]listPromptDigest, 0, len(prompts))
	for _, prompt := range prompts {
		args := append([]promptcatalog.PromptArgument(nil), prompt.Arguments...)
		slices.SortFunc(args, func(a, b promptcatalog.PromptArgument) int {
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})

		digest = append(digest, listPromptDigest{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   args,
		})
	}

	data, err := json.Marshal(digest)
	if err != nil {
		return ""
	}
	return string(data)
}
