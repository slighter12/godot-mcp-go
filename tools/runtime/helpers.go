package runtime

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const (
	defaultRuntimeCommandTimeout = 6 * time.Second
	defaultAwaitSnapshotTimeout  = 3 * time.Second
)

func decodeArgs(args json.RawMessage) (map[string]any, tooltypes.MCPContext, error) {
	arguments := map[string]any{}
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, tooltypes.MCPContext{}, err
	}
	ctx := tooltypes.ExtractMCPContext(arguments)
	return arguments, ctx, nil
}

func requireInitializedContext(ctx tooltypes.MCPContext, toolName string) *tooltypes.SemanticError {
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return tooltypes.NewRuntimeNotAvailableError(
			"Runtime tool requires an initialized MCP HTTP session",
			toolName,
			"editor_session_missing",
			map[string]any{"reason": "session_not_initialized"},
		)
	}
	return nil
}

func requireGameSessionID(arguments map[string]any, toolName string) (string, *tooltypes.SemanticError) {
	raw, ok := arguments["session_id"]
	if !ok {
		return "", tooltypes.NewRuntimeInvalidParamsError("session_id is required", toolName, "game_session_missing", nil)
	}
	value, ok := raw.(string)
	if !ok {
		return "", tooltypes.NewRuntimeInvalidParamsError("session_id must be a string", toolName, "game_session_missing", nil)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", tooltypes.NewRuntimeInvalidParamsError("session_id is required", toolName, "game_session_missing", nil)
	}
	return value, nil
}

func runtimeMetadata(stored runtimebridge.StoredRuntimeSnapshot) map[string]any {
	return map[string]any{
		"source":      "runtime",
		"session_id":  stored.SessionID,
		"snapshot_id": stored.Snapshot.SnapshotID,
		"frame":       stored.Snapshot.Frame,
		"updated_at":  stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapCommandFailureCode(reason string) string {
	switch strings.TrimSpace(reason) {
	case "command_ack_timeout":
		return "command_timeout"
	case "session_missing":
		return "game_session_missing"
	case "command_transport_unavailable":
		return "capability_not_enabled"
	default:
		return "capability_not_enabled"
	}
}

func mapAckFailureCode(ack runtimebridge.CommandAck) string {
	reason := strings.TrimSpace(ack.Error)
	if fromResult, ok := ack.Reason(); ok {
		reason = strings.TrimSpace(fromResult)
	}
	switch reason {
	case "game_not_running":
		return "game_not_running"
	case "runtime_snapshot_missing":
		return "runtime_snapshot_missing"
	case "runtime_snapshot_stale":
		return "runtime_snapshot_stale"
	case "node_not_found":
		return "node_not_found"
	case "property_not_supported":
		return "property_not_supported"
	case "input_not_supported":
		return "input_not_supported"
	case "capability_not_enabled":
		return "capability_not_enabled"
	case "command_timeout":
		return "command_timeout"
	default:
		if reason == "" {
			return "capability_not_enabled"
		}
		return reason
	}
}

func dispatchToRuntimeSession(gameSessionID string, commandName string, commandArgs map[string]any, timeout time.Duration) (runtimebridge.CommandAck, *tooltypes.SemanticError) {
	if timeout <= 0 {
		timeout = defaultRuntimeCommandTimeout
	}

	session, ok := runtimebridge.DefaultGameSessionRegistry().Session(gameSessionID)
	if !ok {
		return runtimebridge.CommandAck{}, tooltypes.NewRuntimeNotAvailableError(
			"Game session is unavailable",
			commandName,
			"game_session_missing",
			map[string]any{"session_id": gameSessionID},
		)
	}
	if !session.Running {
		return runtimebridge.CommandAck{}, tooltypes.NewRuntimeNotAvailableError(
			"Game session is not running",
			commandName,
			"game_not_running",
			map[string]any{"session_id": gameSessionID},
		)
	}

	runtimeSessionID, ok := runtimebridge.DefaultGameSessionRegistry().RuntimeSessionID(gameSessionID)
	if !ok {
		return runtimebridge.CommandAck{}, tooltypes.NewRuntimeNotAvailableError(
			"Runtime transport session is unavailable",
			commandName,
			"game_session_missing",
			map[string]any{"session_id": gameSessionID},
		)
	}
	if commandArgs == nil {
		commandArgs = map[string]any{}
	}
	commandArgs["session_id"] = gameSessionID

	ack, dispatched, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(runtimeSessionID, commandName, commandArgs, timeout)
	if !dispatched {
		return runtimebridge.CommandAck{}, tooltypes.NewRuntimeNotAvailableError(
			"Runtime command is unavailable",
			commandName,
			mapCommandFailureCode(reason),
			map[string]any{"session_id": gameSessionID, "reason": reason},
		)
	}
	if !ack.Success {
		return runtimebridge.CommandAck{}, tooltypes.NewRuntimeNotAvailableError(
			"Runtime command failed",
			commandName,
			mapAckFailureCode(ack),
			map[string]any{"session_id": gameSessionID, "reason": strings.TrimSpace(ack.Error)},
		)
	}
	return ack, nil
}
