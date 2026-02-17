package http

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/slighter12/godot-mcp-go/logger"
	"github.com/slighter12/godot-mcp-go/mcp/jsonrpc"
	"github.com/slighter12/godot-mcp-go/promptcatalog"
	"github.com/slighter12/godot-mcp-go/tools/utility"
)

func (s *Server) registerRuntimeTools() error {
	return s.toolManager.RegisterTool(utility.NewReloadPromptCatalogTool(s.reloadPromptCatalog))
}

func (s *Server) reloadPromptCatalog() map[string]any {
	result := map[string]any{
		"changed":        false,
		"promptCount":    0,
		"loadErrorCount": 0,
		"status":         "disabled",
	}

	if s.promptCatalog == nil || !s.promptCatalog.Enabled() {
		return result
	}

	beforePrompts := s.promptCatalog.ListPrompts()
	beforeFingerprint := promptListFingerprint(beforePrompts)

	loadErr := s.promptCatalog.LoadFromPaths(s.config.PromptCatalog.Paths)

	afterPrompts := s.promptCatalog.ListPrompts()
	afterFingerprint := promptListFingerprint(afterPrompts)
	loadErrors := s.promptCatalog.LoadErrors()
	changed := beforeFingerprint != afterFingerprint

	status := "ok"
	if len(loadErrors) > 0 {
		status = "warning"
	}

	result = map[string]any{
		"changed":        changed,
		"promptCount":    len(afterPrompts),
		"loadErrorCount": len(loadErrors),
		"status":         status,
	}

	if len(loadErrors) > 0 {
		result["warnings"] = summarizeLoadErrors(loadErrors, 5)
	}
	if loadErr != nil {
		logger.Warn("Prompt catalog reloaded with warnings", "error", loadErr)
	}

	if changed {
		notified := s.BroadcastPromptListChanged()
		result["notifiedSessions"] = notified
	}
	return result
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
