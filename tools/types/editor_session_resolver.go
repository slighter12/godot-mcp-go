package types

import (
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

const defaultEditorSnapshotUnavailableMessage = "Editor snapshot is unavailable until it is healthy"

// ParseOptionalEditorSessionID parses optional editor_session_id from tool arguments.
// Empty string is treated as not provided.
func ParseOptionalEditorSessionID(arguments map[string]any, toolName string) (string, bool, *SemanticError) {
	if arguments == nil {
		return "", false, nil
	}
	raw, exists := arguments["editor_session_id"]
	if !exists {
		return "", false, nil
	}
	asString, ok := raw.(string)
	if !ok {
		return "", false, NewRuntimeInvalidParamsError("editor_session_id must be a string", toolName, "editor_session_missing", map[string]any{
			"reason": "invalid_editor_session_id_type",
		})
	}
	asString = strings.TrimSpace(asString)
	if asString == "" {
		return "", false, nil
	}
	return asString, true, nil
}

// ResolveFreshEditorSnapshot resolves one healthy editor snapshot owner for the current tool call.
//
// Resolution order:
// 1. explicit arguments.editor_session_id (when provided)
// 2. caller MCP session id (when fresh)
// 3. latest fresh editor snapshot session
func ResolveFreshEditorSnapshot(arguments map[string]any, ctx MCPContext, toolName string, unavailableMessage string) (runtimebridge.StoredEditorSnapshot, *SemanticError) {
	if strings.TrimSpace(unavailableMessage) == "" {
		unavailableMessage = defaultEditorSnapshotUnavailableMessage
	}
	now := time.Now().UTC()

	explicitEditorSessionID, explicitProvided, semErr := ParseOptionalEditorSessionID(arguments, toolName)
	if semErr != nil {
		return runtimebridge.StoredEditorSnapshot{}, semErr
	}
	if explicitProvided {
		stored, ok, reason := runtimebridge.DefaultEditorStore().FreshForSession(explicitEditorSessionID, now)
		if !ok {
			return runtimebridge.StoredEditorSnapshot{}, NewRuntimeNotAvailableError(unavailableMessage, toolName, reason, map[string]any{
				"editor_session_id": explicitEditorSessionID,
				"resolution":        "explicit_editor_session_id",
			})
		}
		return stored, nil
	}

	callerSessionID := strings.TrimSpace(ctx.SessionID)
	if callerSessionID != "" {
		if stored, ok, _ := runtimebridge.DefaultEditorStore().FreshForSession(callerSessionID, now); ok {
			return stored, nil
		}
	}

	stored, ok, reason := runtimebridge.DefaultEditorStore().LatestFresh(now)
	if !ok {
		return runtimebridge.StoredEditorSnapshot{}, NewRuntimeNotAvailableError(unavailableMessage, toolName, reason, map[string]any{
			"caller_session_id": callerSessionID,
		})
	}
	return stored, nil
}

// ResolveFreshEditorSessionID resolves one healthy editor session id for the current tool call.
func ResolveFreshEditorSessionID(arguments map[string]any, ctx MCPContext, toolName string, unavailableMessage string) (string, *SemanticError) {
	stored, semErr := ResolveFreshEditorSnapshot(arguments, ctx, toolName, unavailableMessage)
	if semErr != nil {
		return "", semErr
	}
	return strings.TrimSpace(stored.SessionID), nil
}
