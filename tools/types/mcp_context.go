package types

import "strings"

// MCPContext carries injected transport/session metadata for internal bridge tools.
type MCPContext struct {
	SessionID          string
	SessionInitialized bool
}

func ExtractMCPContext(arguments map[string]any) MCPContext {
	if arguments == nil {
		return MCPContext{}
	}
	rawContext, _ := arguments["_mcp"].(map[string]any)
	ctx := MCPContext{}
	if rawContext == nil {
		return ctx
	}
	if sessionID, ok := rawContext["session_id"].(string); ok {
		ctx.SessionID = strings.TrimSpace(sessionID)
	}
	if initialized, ok := rawContext["session_initialized"].(bool); ok {
		ctx.SessionInitialized = initialized
	}
	return ctx
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
