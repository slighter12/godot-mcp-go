package types

import "strings"

// MCPContext carries injected transport/session metadata for internal bridge tools.
type MCPContext struct {
	RequestID               string
	ProgressRouteKey        string
	SessionID               string
	EditorSessionID         string
	RuntimeSessionID        string
	RuntimeCommandSessionID string
	SessionInitialized      bool
	EmitProgress            bool
	ProgressToken           any
}

func ExtractMCPContext(arguments map[string]any) MCPContext {
	if arguments == nil {
		return MCPContext{EmitProgress: true}
	}
	rawContext, _ := arguments["_mcp"].(map[string]any)
	ctx := MCPContext{EmitProgress: true}
	if rawContext == nil {
		return ctx
	}
	if requestID, ok := rawContext["request_id"].(string); ok {
		ctx.RequestID = strings.TrimSpace(requestID)
	}
	if progressRouteKey, ok := rawContext["progress_route_key"].(string); ok {
		ctx.ProgressRouteKey = strings.TrimSpace(progressRouteKey)
	}
	if sessionID, ok := rawContext["session_id"].(string); ok {
		ctx.SessionID = strings.TrimSpace(sessionID)
	}
	if editorSessionID, ok := rawContext["editor_session_id"].(string); ok {
		ctx.EditorSessionID = strings.TrimSpace(editorSessionID)
		if ctx.SessionID == "" {
			ctx.SessionID = ctx.EditorSessionID
		}
	}
	if runtimeSessionID, ok := rawContext["runtime_session_id"].(string); ok {
		ctx.RuntimeSessionID = strings.TrimSpace(runtimeSessionID)
	}
	if runtimeCommandSessionID, ok := rawContext["runtime_command_session_id"].(string); ok {
		ctx.RuntimeCommandSessionID = strings.TrimSpace(runtimeCommandSessionID)
	}
	if initialized, ok := rawContext["session_initialized"].(bool); ok {
		ctx.SessionInitialized = initialized
	}
	if emitProgress, ok := rawContext["emit_progress_notifications"].(bool); ok {
		ctx.EmitProgress = emitProgress
	}
	switch token := rawContext["progress_token"].(type) {
	case string:
		token = strings.TrimSpace(token)
		if token != "" {
			ctx.ProgressToken = token
		}
	case float64:
		ctx.ProgressToken = token
	}
	return ctx
}

func (c MCPContext) EffectiveRuntimeSessionID() string {
	if strings.TrimSpace(c.RuntimeSessionID) != "" {
		return strings.TrimSpace(c.RuntimeSessionID)
	}
	return strings.TrimSpace(c.SessionID)
}

func (c MCPContext) EffectiveRuntimeCommandSessionID() string {
	if strings.TrimSpace(c.RuntimeCommandSessionID) != "" {
		return strings.TrimSpace(c.RuntimeCommandSessionID)
	}
	return c.EffectiveRuntimeSessionID()
}

func StripMCPContext(arguments map[string]any) map[string]any {
	if len(arguments) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if key == "_mcp" {
			continue
		}
		out[key] = value
	}
	return out
}
