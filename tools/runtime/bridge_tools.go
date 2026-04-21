package runtime

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type BridgeEditorSyncTool struct{}

func (t *BridgeEditorSyncTool) Name() string { return "godot.bridge.editor.sync" }
func (t *BridgeEditorSyncTool) Description() string {
	return "Synchronizes Godot editor snapshot (internal bridge tool)"
}
func (t *BridgeEditorSyncTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(false),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *BridgeEditorSyncTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"snapshot": map[string]any{"type": "object"},
		},
		Required: []string{"snapshot"},
		Title:    "Bridge Editor Sync",
	}
}
func (t *BridgeEditorSyncTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Snapshot runtimebridge.EditorSnapshot `json:"snapshot"`
		Context  struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Editor sync requires initialized session", t.Name(), "editor_session_missing", nil)
	}

	now := time.Now().UTC()
	runtimebridge.DefaultEditorStore().Upsert(strings.TrimSpace(payload.Context.SessionID), payload.Snapshot, now)
	result := map[string]any{
		"source":     "editor",
		"synced":     true,
		"session_id": strings.TrimSpace(payload.Context.SessionID),
		"updated_at": now.Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type BridgeEditorPingTool struct{}

func (t *BridgeEditorPingTool) Name() string { return "godot.bridge.editor.ping" }
func (t *BridgeEditorPingTool) Description() string {
	return "Refreshes editor snapshot freshness (internal bridge tool)"
}
func (t *BridgeEditorPingTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(false),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *BridgeEditorPingTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Bridge Editor Ping"}
}
func (t *BridgeEditorPingTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Context struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Editor ping requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	now := time.Now().UTC()
	if touched := runtimebridge.DefaultEditorStore().Touch(strings.TrimSpace(payload.Context.SessionID), now); !touched {
		return nil, tooltypes.NewRuntimeNotAvailableError("Editor snapshot is missing", t.Name(), "runtime_snapshot_missing", nil)
	}
	return json.Marshal(map[string]any{
		"source":     "editor",
		"pong":       true,
		"session_id": strings.TrimSpace(payload.Context.SessionID),
		"updated_at": now.Format(time.RFC3339Nano),
	})
}

type BridgeRuntimeRegisterTool struct{}

func (t *BridgeRuntimeRegisterTool) Name() string { return "godot.bridge.runtime.register" }
func (t *BridgeRuntimeRegisterTool) Description() string {
	return "Registers runtime transport for a game session (internal bridge tool)"
}
func (t *BridgeRuntimeRegisterTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *BridgeRuntimeRegisterTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":        map[string]any{"type": "string"},
			"editor_session_id": map[string]any{"type": "string"},
			"scene_path":        map[string]any{"type": "string"},
			"launch_token":      map[string]any{"type": "string"},
			"started_at":        map[string]any{"type": "string"},
		},
		Required: []string{"session_id"},
		Title:    "Bridge Runtime Register",
	}
}
func (t *BridgeRuntimeRegisterTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		SessionID       string `json:"session_id"`
		EditorSessionID string `json:"editor_session_id"`
		ScenePath       string `json:"scene_path"`
		LaunchToken     string `json:"launch_token"`
		StartedAt       string `json:"started_at"`
		Context         struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	log.Printf(
		"godot-mcp runtime.register received: game_session_id=%q runtime_session_id=%q editor_session_id=%q launch_token=%q scene_path=%q started_at=%q",
		strings.TrimSpace(payload.SessionID),
		strings.TrimSpace(payload.Context.SessionID),
		strings.TrimSpace(payload.EditorSessionID),
		strings.TrimSpace(payload.LaunchToken),
		strings.TrimSpace(payload.ScenePath),
		strings.TrimSpace(payload.StartedAt),
	)
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		log.Printf("godot-mcp runtime register rejected: reason=editor_session_missing session_id=%q runtime_session_id=%q", strings.TrimSpace(payload.SessionID), strings.TrimSpace(payload.Context.SessionID))
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime register requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		log.Printf("godot-mcp runtime register rejected: reason=game_session_missing runtime_session_id=%q", strings.TrimSpace(payload.Context.SessionID))
		return nil, tooltypes.NewRuntimeInvalidParamsError("session_id is required", t.Name(), "game_session_missing", nil)
	}
	registeredSession, ok := runtimebridge.DefaultGameSessionRegistry().Session(sessionID)
	if !ok {
		log.Printf("godot-mcp runtime register rejected: reason=game_session_missing session_id=%q runtime_session_id=%q", sessionID, strings.TrimSpace(payload.Context.SessionID))
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime register requires an active game session", t.Name(), "game_session_missing", map[string]any{
			"session_id": sessionID,
			"reason":     "game_session_missing",
		})
	}
	if !runtimebridge.DefaultGameSessionRegistry().MatchesLaunchToken(sessionID, payload.LaunchToken) {
		log.Printf("godot-mcp runtime register rejected: reason=launch_token_mismatch session_id=%q runtime_session_id=%q payload_launch_token_present=%t", sessionID, strings.TrimSpace(payload.Context.SessionID), strings.TrimSpace(payload.LaunchToken) != "")
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime register launch token mismatch", t.Name(), "game_session_missing", map[string]any{
			"session_id": sessionID,
			"reason":     "launch_token_mismatch",
		})
	}
	editorSessionID := strings.TrimSpace(payload.EditorSessionID)
	if editorSessionID == "" {
		editorSessionID = strings.TrimSpace(registeredSession.EditorSessionID)
	}
	startedAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.StartedAt)); err == nil {
		startedAt = parsed.UTC()
	}

	runtimebridge.DefaultGameSessionRegistry().RegisterRuntimeTransport(
		sessionID,
		strings.TrimSpace(payload.Context.SessionID),
		editorSessionID,
		strings.TrimSpace(payload.ScenePath),
		startedAt,
		strings.TrimSpace(payload.LaunchToken),
	)
	log.Printf("godot-mcp runtime register accepted: game_session_id=%q runtime_session_id=%q editor_session_id=%q launch_token=%q", sessionID, strings.TrimSpace(payload.Context.SessionID), editorSessionID, strings.TrimSpace(payload.LaunchToken))
	return json.Marshal(map[string]any{
		"source":             "runtime",
		"registered":         true,
		"session_id":         sessionID,
		"runtime_session_id": strings.TrimSpace(payload.Context.SessionID),
		"editor_session_id":  editorSessionID,
		"started_at":         startedAt.Format(time.RFC3339Nano),
	})
}

type BridgeRuntimeSnapshotPushTool struct{}

func (t *BridgeRuntimeSnapshotPushTool) Name() string { return "godot.bridge.runtime.snapshot.push" }
func (t *BridgeRuntimeSnapshotPushTool) Description() string {
	return "Pushes runtime snapshot for a game session (internal bridge tool)"
}
func (t *BridgeRuntimeSnapshotPushTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *BridgeRuntimeSnapshotPushTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"snapshot":   map[string]any{"type": "object"},
		},
		Required: []string{"session_id", "snapshot"},
		Title:    "Bridge Runtime Snapshot Push",
	}
}
func (t *BridgeRuntimeSnapshotPushTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		SessionID string                        `json:"session_id"`
		Snapshot  runtimebridge.RuntimeSnapshot `json:"snapshot"`
		Context   struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	log.Printf(
		"godot-mcp runtime.snapshot.push received: game_session_id=%q runtime_session_id=%q snapshot_id=%q frame=%d running=%t",
		strings.TrimSpace(payload.SessionID),
		strings.TrimSpace(payload.Context.SessionID),
		strings.TrimSpace(payload.Snapshot.SnapshotID),
		payload.Snapshot.Frame,
		payload.Snapshot.Running,
	)
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		log.Printf("godot-mcp runtime snapshot rejected: reason=editor_session_missing session_id=%q runtime_session_id=%q", strings.TrimSpace(payload.SessionID), strings.TrimSpace(payload.Context.SessionID))
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime snapshot push requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		log.Printf("godot-mcp runtime snapshot rejected: reason=game_session_missing runtime_session_id=%q", strings.TrimSpace(payload.Context.SessionID))
		return nil, tooltypes.NewRuntimeInvalidParamsError("session_id is required", t.Name(), "game_session_missing", nil)
	}
	if !runtimebridge.DefaultGameSessionRegistry().RuntimeSessionMatches(sessionID, payload.Context.SessionID) {
		expectedRuntimeSessionID, _ := runtimebridge.DefaultGameSessionRegistry().RuntimeSessionID(sessionID)
		log.Printf("godot-mcp runtime snapshot rejected: reason=runtime_session_mismatch session_id=%q runtime_session_id=%q expected_runtime_session_id=%q", sessionID, strings.TrimSpace(payload.Context.SessionID), strings.TrimSpace(expectedRuntimeSessionID))
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime snapshot push session mismatch", t.Name(), "game_session_missing", map[string]any{
			"session_id": sessionID,
			"reason":     "runtime_session_mismatch",
		})
	}
	sessionBefore, _ := runtimebridge.DefaultGameSessionRegistry().Session(sessionID)
	now := time.Now().UTC()
	runtimebridge.DefaultRuntimeSnapshotStore().Upsert(sessionID, payload.Snapshot, now)
	runtimebridge.DefaultGameSessionRegistry().MarkSnapshotReceived(sessionID, now)
	if !sessionBefore.HasSnapshot {
		log.Printf("godot-mcp runtime snapshot accepted: first_snapshot=true session_id=%q runtime_session_id=%q snapshot_id=%q frame=%d", sessionID, strings.TrimSpace(payload.Context.SessionID), strings.TrimSpace(payload.Snapshot.SnapshotID), payload.Snapshot.Frame)
	}
	if sessionBefore.HasSnapshot {
		log.Printf("godot-mcp runtime snapshot accepted: first_snapshot=false session_id=%q runtime_session_id=%q snapshot_id=%q frame=%d", sessionID, strings.TrimSpace(payload.Context.SessionID), strings.TrimSpace(payload.Snapshot.SnapshotID), payload.Snapshot.Frame)
	}
	return json.Marshal(map[string]any{
		"source":      "runtime",
		"synced":      true,
		"session_id":  sessionID,
		"snapshot_id": strings.TrimSpace(payload.Snapshot.SnapshotID),
		"frame":       payload.Snapshot.Frame,
		"updated_at":  now.Format(time.RFC3339Nano),
	})
}

type BridgeRuntimeLogPushTool struct{}

func (t *BridgeRuntimeLogPushTool) Name() string { return "godot.bridge.runtime.log.push" }
func (t *BridgeRuntimeLogPushTool) Description() string {
	return "Pushes runtime log entries for a game session (internal bridge tool)"
}
func (t *BridgeRuntimeLogPushTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *BridgeRuntimeLogPushTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"entries":    map[string]any{"type": "array"},
		},
		Required: []string{"session_id", "entries"},
		Title:    "Bridge Runtime Log Push",
	}
}
func (t *BridgeRuntimeLogPushTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		SessionID string                                `json:"session_id"`
		Entries   []runtimebridge.RuntimeLogAppendEntry `json:"entries"`
		Context   struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime log push requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		return nil, tooltypes.NewRuntimeInvalidParamsError("session_id is required", t.Name(), "game_session_missing", nil)
	}
	if !runtimebridge.DefaultGameSessionRegistry().RuntimeSessionMatches(sessionID, payload.Context.SessionID) {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime log push session mismatch", t.Name(), "game_session_missing", map[string]any{
			"session_id": sessionID,
			"reason":     "runtime_session_mismatch",
		})
	}
	appended := runtimebridge.DefaultRuntimeLogStore().Append(sessionID, payload.Entries, time.Now().UTC())
	return json.Marshal(map[string]any{
		"source":     "runtime",
		"session_id": sessionID,
		"appended":   len(appended),
	})
}

type BridgeCommandAckTool struct{}

func (t *BridgeCommandAckTool) Name() string { return "godot.bridge.command.ack" }
func (t *BridgeCommandAckTool) Description() string {
	return "Acknowledges completion of an internal runtime command"
}
func (t *BridgeCommandAckTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *BridgeCommandAckTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"command_id":     map[string]any{"type": "string"},
			"success":        map[string]any{"type": "boolean"},
			"result":         map[string]any{"type": "object"},
			"error":          map[string]any{"type": "string"},
			"reason":         map[string]any{"type": "string"},
			"retryable":      map[string]any{"type": "boolean"},
			"schema_version": map[string]any{"type": "string"},
		},
		Required: []string{"command_id"},
		Title:    "Bridge Command Ack",
	}
}
func (t *BridgeCommandAckTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		CommandID string         `json:"command_id"`
		Success   *bool          `json:"success"`
		Result    map[string]any `json:"result"`
		Error     string         `json:"error"`
		Reason    string         `json:"reason"`
		Retryable *bool          `json:"retryable"`
		SchemaVer string         `json:"schema_version"`
		Context   struct {
			SessionID          string `json:"session_id"`
			SessionInitialized bool   `json:"session_initialized"`
		} `json:"_mcp"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Context.SessionID) == "" || !payload.Context.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Command ack requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	commandID := strings.TrimSpace(payload.CommandID)
	if commandID == "" {
		return nil, tooltypes.NewRuntimeInvalidParamsError("command_id is required", t.Name(), "command_timeout", nil)
	}
	success := true
	if payload.Success != nil {
		success = *payload.Success
	}
	result := payload.Result
	if result == nil {
		result = map[string]any{}
	}
	if strings.TrimSpace(payload.Reason) != "" {
		result["reason"] = strings.TrimSpace(payload.Reason)
	}
	if payload.Retryable != nil {
		result["retryable"] = *payload.Retryable
	}
	if strings.TrimSpace(payload.SchemaVer) != "" {
		result["schema_version"] = strings.TrimSpace(payload.SchemaVer)
	}
	ack := runtimebridge.CommandAck{
		CommandID: commandID,
		Success:   success,
		Result:    result,
		Error:     strings.TrimSpace(payload.Error),
		AckedAt:   time.Now().UTC(),
	}
	if ok := runtimebridge.DefaultCommandBroker().Ack(strings.TrimSpace(payload.Context.SessionID), ack); !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Command acknowledgement rejected", t.Name(), "command_timeout", map[string]any{
			"command_id": commandID,
			"reason":     "unknown_or_expired_command",
		})
	}
	return json.Marshal(map[string]any{
		"acknowledged": true,
		"command_id":   commandID,
	})
}
