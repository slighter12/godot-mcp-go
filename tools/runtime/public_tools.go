package runtime

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

type GetActiveGameSessionTool struct{}

func (t *GetActiveGameSessionTool) Name() string { return "godot.runtime.session.get_active" }
func (t *GetActiveGameSessionTool) Description() string {
	return "[runtime] Returns the active runtime game session for one resolved editor session owner"
}
func (t *GetActiveGameSessionTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *GetActiveGameSessionTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"editor_session_id": map[string]any{"type": "string"},
		},
		Required: []string{},
		Title:    "Get Active Game Session",
	}
}
func (t *GetActiveGameSessionTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}

	editorSessionID, semErr := tooltypes.ResolveFreshEditorSessionID(arguments, ctx, t.Name(), "Active game session is unavailable until editor snapshot is healthy")
	if semErr != nil {
		return nil, semErr
	}
	session, ok := runtimebridge.DefaultGameSessionRegistry().ActiveForEditor(editorSessionID)
	if !ok {
		// Fallback: the resolved editor session may differ from the one used
		// by project.run (e.g. editor reconnected).  Try the most recently
		// started running game session across all editors.
		session, ok = runtimebridge.DefaultGameSessionRegistry().LatestRunning()
	}
	if !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Active game session is unavailable", t.Name(), "game_session_missing", map[string]any{
			"editor_session_id": editorSessionID,
		})
	}
	return json.Marshal(map[string]any{
		"source":             "runtime",
		"session_id":         session.SessionID,
		"editor_session_id":  editorSessionID,
		"running":            session.Running,
		"started_at":         session.StartedAt,
		"scene_path":         session.ScenePath,
		"runtime_session_id": session.RuntimeSessionID,
		"has_snapshot":       session.HasSnapshot,
		"last_snapshot_at":   session.LastSnapshotAt,
	})
}

type RuntimeSyncNowTool struct{}

func (t *RuntimeSyncNowTool) Name() string { return "godot.runtime.sync_now" }
func (t *RuntimeSyncNowTool) Description() string {
	return "[runtime] Requests runtime to emit a fresh snapshot immediately"
}
func (t *RuntimeSyncNowTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeSyncNowTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
		},
		Required: []string{"session_id"},
		Title:    "Runtime Sync Now",
	}
}
func (t *RuntimeSyncNowTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	ack, dispatchErr := dispatchToRuntimeSession(sessionID, t.Name(), map[string]any{}, defaultRuntimeCommandTimeout)
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	return json.Marshal(map[string]any{
		"source":       "runtime",
		"session_id":   sessionID,
		"command_id":   ack.CommandID,
		"acknowledged": ack.AckedAt.UTC().Format(time.RFC3339Nano),
		"result":       ack.Result,
		"schema_version": func() string {
			if v, ok := ack.SchemaVersion(); ok {
				return v
			}
			return ""
		}(),
	})
}

type AwaitRuntimeSnapshotTool struct{}

func (t *AwaitRuntimeSnapshotTool) Name() string { return "godot.runtime.await_snapshot" }
func (t *AwaitRuntimeSnapshotTool) Description() string {
	return "[runtime] Waits until a runtime snapshot satisfies frame/freshness conditions"
}
func (t *AwaitRuntimeSnapshotTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(true),
	}
}
func (t *AwaitRuntimeSnapshotTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"min_frame":  map[string]any{"type": "integer"},
			"timeout_ms": map[string]any{"type": "integer"},
			"freshness":  map[string]any{"type": "string"},
		},
		Required: []string{"session_id"},
		Title:    "Await Runtime Snapshot",
	}
}
func (t *AwaitRuntimeSnapshotTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}

	var minFrame int64
	if raw, ok := arguments["min_frame"]; ok {
		value, ok := raw.(float64)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("min_frame must be a number", t.Name(), "runtime_snapshot_missing", nil)
		}
		minFrame = int64(value)
	}

	timeout := defaultAwaitSnapshotTimeout
	if raw, ok := arguments["timeout_ms"]; ok {
		value, ok := raw.(float64)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("timeout_ms must be a number", t.Name(), "command_timeout", nil)
		}
		if int(value) > 0 {
			timeout = time.Duration(int(value)) * time.Millisecond
		}
	}
	minFreshness := runtimebridge.FreshnessStateFresh
	if raw, ok := arguments["freshness"]; ok {
		value, ok := raw.(string)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("freshness must be a string", t.Name(), "command_timeout", nil)
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case runtimebridge.FreshnessStateFresh, runtimebridge.FreshnessStateGrace, runtimebridge.FreshnessStateStale:
			minFreshness = strings.ToLower(strings.TrimSpace(value))
		default:
			return nil, tooltypes.NewRuntimeInvalidParamsError("freshness must be one of fresh, grace, stale", t.Name(), "command_timeout", nil)
		}
	}

	stored, reason, ok := runtimebridge.DefaultRuntimeSnapshotStore().Await(sessionID, minFrame, timeout, minFreshness)
	if !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime snapshot await failed", t.Name(), reason, map[string]any{
			"session_id": sessionID,
		})
	}
	storedForState, freshness, _ := runtimebridge.DefaultRuntimeSnapshotStore().StateForSession(sessionID, time.Now().UTC())
	if storedForState.SessionID != "" {
		stored = storedForState
	}

	out := runtimeMetadata(stored)
	out["freshness"] = freshness
	out["root_scene_path"] = stored.Snapshot.RootScenePath
	out["root_node_name"] = stored.Snapshot.RootNodeName
	return json.Marshal(out)
}

type RuntimeSceneTreeGetTool struct{}

func (t *RuntimeSceneTreeGetTool) Name() string { return "godot.runtime.scene_tree.get" }
func (t *RuntimeSceneTreeGetTool) Description() string {
	return "[runtime] Returns the runtime scene tree from live game snapshot"
}
func (t *RuntimeSceneTreeGetTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeSceneTreeGetTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"max_depth":  map[string]any{"type": "integer"},
		},
		Required: []string{"session_id"},
		Title:    "Runtime Scene Tree Get",
	}
}
func (t *RuntimeSceneTreeGetTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}

	stored, ok, reason := runtimebridge.DefaultRuntimeSnapshotStore().FreshForSession(sessionID, time.Now().UTC())
	if !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime scene tree is unavailable", t.Name(), reason, map[string]any{"session_id": sessionID})
	}
	maxDepth := 4
	if raw, ok := arguments["max_depth"]; ok {
		if value, ok := raw.(float64); ok && int(value) > 0 {
			maxDepth = int(value)
		}
	}
	tree := limitRuntimeTreeDepth(stored.Snapshot.SceneTree, 0, maxDepth)
	out := runtimeMetadata(stored)
	out["root"] = tree
	out["root_scene_path"] = stored.Snapshot.RootScenePath
	out["root_node_name"] = stored.Snapshot.RootNodeName
	return json.Marshal(out)
}

type RuntimeNodePropertiesGetTool struct{}

func (t *RuntimeNodePropertiesGetTool) Name() string { return "godot.runtime.node_properties.get" }
func (t *RuntimeNodePropertiesGetTool) Description() string {
	return "[runtime] Reads runtime node properties via on-demand runtime fetch"
}
func (t *RuntimeNodePropertiesGetTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeNodePropertiesGetTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"node":       map[string]any{"type": "string"},
			"properties": map[string]any{"type": "array"},
		},
		Required: []string{"session_id", "node", "properties"},
		Title:    "Runtime Node Properties Get",
	}
}
func (t *RuntimeNodePropertiesGetTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}

	node, ok := arguments["node"].(string)
	if !ok || strings.TrimSpace(node) == "" {
		return nil, tooltypes.NewRuntimeInvalidParamsError("node is required", t.Name(), "node_not_found", nil)
	}
	propsRaw, ok := arguments["properties"].([]any)
	if !ok || len(propsRaw) == 0 {
		return nil, tooltypes.NewRuntimeInvalidParamsError("properties must be a non-empty array", t.Name(), "property_not_supported", nil)
	}
	properties := make([]string, 0, len(propsRaw))
	for _, raw := range propsRaw {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, tooltypes.NewRuntimeInvalidParamsError("properties entries must be strings", t.Name(), "property_not_supported", nil)
		}
		properties = append(properties, strings.TrimSpace(name))
	}

	ack, dispatchErr := dispatchToRuntimeSession(sessionID, t.Name(), map[string]any{
		"node":       strings.TrimSpace(node),
		"properties": properties,
	}, defaultRuntimeCommandTimeout)
	if dispatchErr != nil {
		return nil, dispatchErr
	}

	out := map[string]any{
		"source":      "runtime",
		"session_id":  sessionID,
		"command_id":  ack.CommandID,
		"node":        strings.TrimSpace(node),
		"properties":  ack.Result["properties"],
		"type":        ack.Result["type"],
		"snapshot_id": ack.Result["snapshot_id"],
		"frame":       ack.Result["frame"],
		"updated_at":  ack.Result["updated_at"],
	}
	return json.Marshal(out)
}

type RuntimeInputTapTool struct{}

func (t *RuntimeInputTapTool) Name() string { return "godot.runtime.input.tap" }
func (t *RuntimeInputTapTool) Description() string {
	return "[runtime] Sends a tap input to runtime game session"
}
func (t *RuntimeInputTapTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *RuntimeInputTapTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":  map[string]any{"type": "string"},
			"input":       map[string]any{"type": "string"},
			"duration_ms": map[string]any{"type": "integer"},
		},
		Required: []string{"session_id", "input"},
		Title:    "Runtime Input Tap",
	}
}
func (t *RuntimeInputTapTool) Execute(args json.RawMessage) ([]byte, error) {
	return executeInputCommand(args, t.Name(), true)
}

type RuntimeInputPressTool struct{}

func (t *RuntimeInputPressTool) Name() string        { return "godot.runtime.input.press" }
func (t *RuntimeInputPressTool) Description() string { return "[runtime] Presses a runtime input" }
func (t *RuntimeInputPressTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *RuntimeInputPressTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"input":      map[string]any{"type": "string"},
		},
		Required: []string{"session_id", "input"},
		Title:    "Runtime Input Press",
	}
}
func (t *RuntimeInputPressTool) Execute(args json.RawMessage) ([]byte, error) {
	return executeInputCommand(args, t.Name(), false)
}

type RuntimeInputReleaseTool struct{}

func (t *RuntimeInputReleaseTool) Name() string        { return "godot.runtime.input.release" }
func (t *RuntimeInputReleaseTool) Description() string { return "[runtime] Releases a runtime input" }
func (t *RuntimeInputReleaseTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *RuntimeInputReleaseTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"input":      map[string]any{"type": "string"},
		},
		Required: []string{"session_id", "input"},
		Title:    "Runtime Input Release",
	}
}
func (t *RuntimeInputReleaseTool) Execute(args json.RawMessage) ([]byte, error) {
	return executeInputCommand(args, t.Name(), false)
}

func executeInputCommand(args json.RawMessage, toolName string, allowDuration bool) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, toolName); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, toolName)
	if semErr != nil {
		return nil, semErr
	}
	input, ok := arguments["input"].(string)
	if !ok || strings.TrimSpace(input) == "" {
		return nil, tooltypes.NewRuntimeInvalidParamsError("input is required", toolName, "input_not_supported", nil)
	}
	cmdArgs := map[string]any{"input": strings.TrimSpace(input)}
	if allowDuration {
		if raw, ok := arguments["duration_ms"]; ok {
			if value, ok := raw.(float64); ok && int(value) > 0 {
				cmdArgs["duration_ms"] = int(value)
			}
		}
	}
	ack, dispatchErr := dispatchToRuntimeSession(sessionID, toolName, cmdArgs, defaultRuntimeCommandTimeout)
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	return json.Marshal(map[string]any{
		"source":      "runtime",
		"session_id":  sessionID,
		"command_id":  ack.CommandID,
		"input":       strings.TrimSpace(input),
		"frame":       ack.Result["frame"],
		"timestamp":   ack.Result["timestamp"],
		"updated_at":  ack.Result["updated_at"],
		"snapshot_id": ack.Result["snapshot_id"],
	})
}

type RuntimeLogGetTool struct{}

func (t *RuntimeLogGetTool) Name() string { return "godot.runtime.log.get" }
func (t *RuntimeLogGetTool) Description() string {
	return "[runtime] Returns runtime log entries for one game session"
}
func (t *RuntimeLogGetTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeLogGetTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":     map[string]any{"type": "string"},
			"level":          map[string]any{"type": "string"},
			"limit":          map[string]any{"type": "integer"},
			"since_sequence": map[string]any{"type": "integer"},
		},
		Required: []string{"session_id"},
		Title:    "Runtime Log Get",
	}
}
func (t *RuntimeLogGetTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	session, ok := runtimebridge.DefaultGameSessionRegistry().Session(sessionID)
	if !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime log stream is unavailable", t.Name(), "game_session_missing", map[string]any{
			"session_id": sessionID,
		})
	}
	if !session.Running {
		return nil, tooltypes.NewRuntimeNotAvailableError("Runtime log stream is unavailable", t.Name(), "game_not_running", map[string]any{
			"session_id": sessionID,
		})
	}

	level := "all"
	if raw, ok := arguments["level"]; ok {
		value, ok := raw.(string)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("level must be a string", t.Name(), "runtime_snapshot_missing", nil)
		}
		level = strings.TrimSpace(value)
	}
	limit := 50
	if raw, ok := arguments["limit"]; ok {
		if value, ok := raw.(float64); ok && int(value) > 0 {
			limit = int(value)
		}
	}
	var sinceSequence int64
	if raw, ok := arguments["since_sequence"]; ok {
		if value, ok := raw.(float64); ok {
			sinceSequence = int64(value)
		}
	}
	entries := runtimebridge.DefaultRuntimeLogStore().Get(sessionID, level, limit, sinceSequence)
	return json.Marshal(map[string]any{
		"source":     "runtime",
		"session_id": sessionID,
		"entries":    entries,
	})
}

type RuntimeLogClearTool struct{}

func (t *RuntimeLogClearTool) Name() string { return "godot.runtime.log.clear" }
func (t *RuntimeLogClearTool) Description() string {
	return "[runtime] Clears runtime log buffer for one game session"
}
func (t *RuntimeLogClearTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    tooltypes.BoolPtr(false),
		DestructiveHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeLogClearTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
		},
		Required: []string{"session_id"},
		Title:    "Runtime Log Clear",
	}
}
func (t *RuntimeLogClearTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	ack, dispatchErr := dispatchToRuntimeSession(sessionID, t.Name(), map[string]any{}, defaultRuntimeCommandTimeout)
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	cleared := runtimebridge.DefaultRuntimeLogStore().Clear(sessionID)
	return json.Marshal(map[string]any{
		"source":     "runtime",
		"session_id": sessionID,
		"cleared":    cleared,
		"command_id": ack.CommandID,
	})
}

type RuntimeScreenshotGetTool struct{}

func (t *RuntimeScreenshotGetTool) Name() string { return "godot.runtime.screenshot.get" }
func (t *RuntimeScreenshotGetTool) Description() string {
	return "[runtime] Captures one runtime screenshot from running game"
}
func (t *RuntimeScreenshotGetTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *RuntimeScreenshotGetTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id": map[string]any{"type": "string"},
			"mode":       map[string]any{"type": "string"},
		},
		Required: []string{"session_id"},
		Title:    "Runtime Screenshot Get",
	}
}
func (t *RuntimeScreenshotGetTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments, ctx, err := decodeArgs(args)
	if err != nil {
		return nil, err
	}
	if semErr := requireInitializedContext(ctx, t.Name()); semErr != nil {
		return nil, semErr
	}
	sessionID, semErr := requireGameSessionID(arguments, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	mode := "viewport"
	if raw, ok := arguments["mode"]; ok {
		value, ok := raw.(string)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("mode must be a string", t.Name(), "capability_not_enabled", nil)
		}
		if strings.TrimSpace(value) != "" {
			mode = strings.TrimSpace(value)
		}
	}
	ack, dispatchErr := dispatchToRuntimeSession(sessionID, t.Name(), map[string]any{"mode": mode}, defaultRuntimeCommandTimeout)
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	out := map[string]any{
		"source":     "runtime",
		"session_id": sessionID,
		"command_id": ack.CommandID,
	}
	for key, value := range ack.Result {
		out[key] = value
	}
	return json.Marshal(out)
}

func limitRuntimeTreeDepth(node runtimebridge.CompactNode, depth int, maxDepth int) runtimebridge.CompactNode {
	if depth >= maxDepth {
		node.Children = nil
		return node
	}
	if len(node.Children) == 0 {
		return node
	}
	children := make([]runtimebridge.CompactNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, limitRuntimeTreeDepth(child, depth+1, maxDepth))
	}
	node.Children = children
	return node
}
