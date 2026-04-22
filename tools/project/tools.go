package project

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
	tooltypes "github.com/slighter12/godot-mcp-go/tools/types"
)

const projectCommandTimeout = 8 * time.Second
const projectListPageSize = 200

type GetProjectSettingsTool struct{}

func (t *GetProjectSettingsTool) Name() string        { return "godot.project.settings.get" }
func (t *GetProjectSettingsTool) Description() string { return "[file-based] Gets project settings" }
func (t *GetProjectSettingsTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *GetProjectSettingsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"cursor":         map[string]any{"type": "string", "description": "Pagination cursor returned by previous call"},
			"section_prefix": map[string]any{"type": "string", "description": "Optional section name prefix filter"},
		},
		Required: []string{},
		Title:    "Get Project Settings",
	}
}
func (t *GetProjectSettingsTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Cursor        string `json:"cursor"`
		SectionPrefix string `json:"section_prefix"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}

	settings, err := readProjectSettings()
	if err != nil {
		return nil, err
	}
	filtered := make([]projectSettingEntry, 0, len(settings))
	sectionPrefix := strings.TrimSpace(payload.SectionPrefix)
	for _, entry := range settings {
		if sectionPrefix != "" && !strings.HasPrefix(entry.Section, sectionPrefix) {
			continue
		}
		filtered = append(filtered, entry)
	}

	start, err := parseProjectCursor(payload.Cursor, len(filtered))
	if err != nil {
		return nil, err
	}
	end := min(start+projectListPageSize, len(filtered))
	result := map[string]any{
		"settings": filtered[start:end],
	}
	if end < len(filtered) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return json.Marshal(result)
}

type ListProjectResourcesTool struct{}

func (t *ListProjectResourcesTool) Name() string { return "godot.project.resources.list" }
func (t *ListProjectResourcesTool) Description() string {
	return "[file-based] Lists all resources in the project"
}
func (t *ListProjectResourcesTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *ListProjectResourcesTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"cursor":         map[string]any{"type": "string", "description": "Pagination cursor returned by previous call"},
			"extensions":     map[string]any{"type": "array", "description": "Optional extension filter (e.g. ['.tscn','.gd'])"},
			"include_hidden": map[string]any{"type": "boolean", "description": "Include hidden dot-prefixed files"},
		},
		Required: []string{},
		Title:    "List Project Resources",
	}
}
func (t *ListProjectResourcesTool) Execute(args json.RawMessage) ([]byte, error) {
	var payload struct {
		Cursor        string   `json:"cursor"`
		Extensions    []string `json:"extensions"`
		IncludeHidden bool     `json:"include_hidden"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	extensionFilter := normalizeExtensionFilter(payload.Extensions)

	projectRoot := tooltypes.ResolveProjectRootFromEnvOrCWD()
	projectAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	resources := make([]projectResourceEntry, 0, 256)
	err = filepath.WalkDir(projectAbs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == projectAbs {
			return nil
		}

		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipProjectDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		if !payload.IncludeHidden && strings.HasPrefix(name, ".") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		if len(extensionFilter) > 0 {
			if _, ok := extensionFilter[ext]; !ok {
				return nil
			}
		}

		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		relPath, relErr := filepath.Rel(projectAbs, path)
		if relErr != nil {
			return relErr
		}
		resources = append(resources, projectResourceEntry{
			Path:       "res://" + filepath.ToSlash(relPath),
			Extension:  ext,
			SizeBytes:  fileInfo.Size(),
			ModifiedAt: fileInfo.ModTime().UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Path < resources[j].Path
	})

	start, err := parseProjectCursor(payload.Cursor, len(resources))
	if err != nil {
		return nil, err
	}
	end := min(start+projectListPageSize, len(resources))
	result := map[string]any{
		"resources": resources[start:end],
	}
	if end < len(resources) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	return json.Marshal(result)
}

type GetEditorStateTool struct{}

func (t *GetEditorStateTool) Name() string { return "godot.editor.state.get" }
func (t *GetEditorStateTool) Description() string {
	return "[editor-plugin] Gets the current editor state"
}
func (t *GetEditorStateTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *GetEditorStateTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"editor_session_id": map[string]any{"type": "string", "description": "Optional explicit editor session id override"},
		},
		Required: []string{},
		Title:    "Get Editor State",
	}
}
func (t *GetEditorStateTool) Execute(args json.RawMessage) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Editor state requires an initialized MCP HTTP session", t.Name(), "editor_session_missing", map[string]any{
			"reason": "session_not_initialized",
		})
	}

	stored, semErr := tooltypes.ResolveFreshEditorSnapshot(arguments, ctx, t.Name(), "Editor state is unavailable until editor snapshot is healthy")
	if semErr != nil {
		return nil, semErr
	}
	result := map[string]any{
		"source":        "editor",
		"active_scene":  stored.Snapshot.RootSummary.ActiveScene,
		"active_script": stored.Snapshot.RootSummary.ActiveScript,
		"root_summary":  stored.Snapshot.RootSummary,
		"session_id":    stored.SessionID,
		"updated_at":    stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type IsProjectRunningTool struct{}

func (t *IsProjectRunningTool) Name() string { return "godot.project.is_running" }
func (t *IsProjectRunningTool) Description() string {
	return "[editor-plugin] Returns whether a game session is running"
}
func (t *IsProjectRunningTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   tooltypes.BoolPtr(true),
		IdempotentHint: tooltypes.BoolPtr(true),
	}
}
func (t *IsProjectRunningTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":        map[string]any{"type": "string", "description": "Optional game session id"},
			"editor_session_id": map[string]any{"type": "string", "description": "Optional explicit editor session id override"},
		},
		Required: []string{},
		Title:    "Project Is Running",
	}
}
func (t *IsProjectRunningTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments := map[string]any{}
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}
	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Project running check requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	querySessionID := ""
	if raw, ok := arguments["session_id"]; ok {
		value, ok := raw.(string)
		if !ok {
			return nil, tooltypes.NewRuntimeInvalidParamsError("session_id must be a string", t.Name(), "game_session_missing", nil)
		}
		querySessionID = strings.TrimSpace(value)
	}
	resolvedEditorSessionID := ""
	if querySessionID == "" {
		editorSessionID, semErr := tooltypes.ResolveFreshEditorSessionID(arguments, ctx, t.Name(), "Project running check requires healthy editor snapshot")
		if semErr != nil {
			return nil, semErr
		}
		resolvedEditorSessionID = editorSessionID
		if active, ok := runtimebridge.DefaultGameSessionRegistry().ActiveForEditor(editorSessionID); ok {
			querySessionID = active.SessionID
		}
	}
	if querySessionID == "" {
		return json.Marshal(map[string]any{
			"source":            "runtime",
			"running":           false,
			"session_id":        "",
			"editor_session_id": resolvedEditorSessionID,
		})
	}
	session, ok := runtimebridge.DefaultGameSessionRegistry().Session(querySessionID)
	if !ok {
		return json.Marshal(map[string]any{
			"source":            "runtime",
			"running":           false,
			"session_id":        querySessionID,
			"editor_session_id": resolvedEditorSessionID,
		})
	}
	return json.Marshal(map[string]any{
		"source":            "runtime",
		"session_id":        session.SessionID,
		"editor_session_id": session.EditorSessionID,
		"running":           session.Running,
		"started_at":        session.StartedAt,
		"scene_path":        session.ScenePath,
	})
}

type RunProjectTool struct{}

func (t *RunProjectTool) Name() string        { return "godot.project.run" }
func (t *RunProjectTool) Description() string { return "[editor-plugin] Runs the project" }
func (t *RunProjectTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *RunProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":        map[string]any{"type": "string", "description": "Optional pre-allocated game session id"},
			"scene_path":        map[string]any{"type": "string", "description": "Optional scene path for runtime launch metadata"},
			"editor_session_id": map[string]any{"type": "string", "description": "Optional explicit editor session id override"},
		},
		Required: []string{},
		Title:    "Run Project",
	}
}
func (t *RunProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments := map[string]any{}
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}
	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Project run requires initialized session", t.Name(), "editor_session_missing", nil)
	}

	runSessionID := strings.TrimSpace(extractString(arguments["session_id"]))
	if runSessionID == "" {
		runSessionID = generateGameSessionID()
	}
	scenePath := strings.TrimSpace(extractString(arguments["scene_path"]))
	launchToken := generateLaunchToken()
	startedAt := time.Now().UTC()
	editorCommandSessionID, semErr := resolveProjectEditorCommandSessionID(arguments, ctx, runSessionID, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	log.Printf("godot-mcp project.run request received: caller_session_id=%q editor_session_id=%q game_session_id=%q launch_token=%q scene_path=%q", strings.TrimSpace(ctx.SessionID), editorCommandSessionID, runSessionID, launchToken, scenePath)

	// Clean up zombie game sessions left by previous project.run calls that
	// timed out waiting for the first runtime snapshot.  These are sessions
	// that are still Running but have no RuntimeSessionID (runtime addon
	// never connected).
	if staleCount := runtimebridge.DefaultGameSessionRegistry().StopStaleForEditor(editorCommandSessionID, runSessionID, startedAt); staleCount > 0 {
		log.Printf("godot-mcp project.run cleaned up %d stale game session(s) for editor %q", staleCount, editorCommandSessionID)
	}

	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun(runSessionID, editorCommandSessionID, scenePath, launchToken, startedAt)

	ack, ok, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(editorCommandSessionID, t.Name(), map[string]any{
		"session_id":   runSessionID,
		"launch_token": launchToken,
		"scene_path":   scenePath,
	}, projectCommandTimeout)
	log.Printf("godot-mcp project.run dispatched: editor_session_id=%q game_session_id=%q launch_token=%q dispatch_ok=%t reason=%q", editorCommandSessionID, runSessionID, launchToken, ok, strings.TrimSpace(reason))
	if !ok {
		cleanupFailedRunSession(runSessionID)
		return nil, tooltypes.NewRuntimeNotAvailableError("Project run bridge is unavailable", t.Name(), mapProjectCommandReason(reason), map[string]any{
			"reason": reason,
		})
	}
	if !ack.Success {
		cleanupFailedRunSession(runSessionID)
		return nil, tooltypes.NewRuntimeNotAvailableError("Project run failed", t.Name(), "game_not_running", map[string]any{
			"reason": ack.Error,
		})
	}

	if ackSession := strings.TrimSpace(extractString(ack.Result["session_id"])); ackSession != "" && ackSession != runSessionID {
		cleanupFailedRunSession(runSessionID)
		log.Printf("godot-mcp project.run attach remap: previous_game_session_id=%q ack_game_session_id=%q", runSessionID, ackSession)
		runSessionID = ackSession
		// Keep launch token untouched during attach remap; it may need to be read
		// from ack payload (or preserved from existing registry record) below.
		runtimebridge.DefaultGameSessionRegistry().UpsertFromRun(runSessionID, editorCommandSessionID, scenePath, "", startedAt)
	}
	if ackLaunchToken := strings.TrimSpace(extractString(ack.Result["launch_token"])); ackLaunchToken != "" {
		launchToken = ackLaunchToken
	} else if existingSession, ok := runtimebridge.DefaultGameSessionRegistry().Session(runSessionID); ok {
		if existingLaunchToken := strings.TrimSpace(existingSession.LaunchToken); existingLaunchToken != "" {
			launchToken = existingLaunchToken
		}
	}
	if ackScenePath := strings.TrimSpace(extractString(ack.Result["scene_path"])); ackScenePath != "" {
		scenePath = ackScenePath
	}
	if ackStartedAt := strings.TrimSpace(extractString(ack.Result["started_at"])); ackStartedAt != "" {
		if parsedStartedAt, err := time.Parse(time.RFC3339Nano, ackStartedAt); err == nil {
			startedAt = parsedStartedAt.UTC()
		}
	}
	runtimebridge.DefaultGameSessionRegistry().UpsertFromRun(runSessionID, editorCommandSessionID, scenePath, launchToken, startedAt)
	log.Printf("godot-mcp project.run ack accepted: editor_session_id=%q game_session_id=%q launch_token=%q scene_path=%q", editorCommandSessionID, runSessionID, launchToken, scenePath)

	if _, reason, ready := runtimebridge.DefaultRuntimeSnapshotStore().Await(runSessionID, 0, projectCommandTimeout, runtimebridge.FreshnessStateFresh); !ready {
		log.Printf("godot-mcp project.run await first snapshot failed: editor_session_id=%q game_session_id=%q reason=%q", editorCommandSessionID, runSessionID, strings.TrimSpace(reason))
		return nil, tooltypes.NewRuntimeNotAvailableError("Project run timed out waiting for runtime snapshot", t.Name(), reason, map[string]any{
			"session_id":                   runSessionID,
			"editor_session_id":            editorCommandSessionID,
			"runtime_registration_pending": true,
		})
	}
	log.Printf("godot-mcp project.run first snapshot observed: editor_session_id=%q game_session_id=%q", editorCommandSessionID, runSessionID)

	session, _ := runtimebridge.DefaultGameSessionRegistry().Session(runSessionID)
	return json.Marshal(map[string]any{
		"success":           true,
		"source":            "runtime",
		"session_id":        runSessionID,
		"editor_session_id": editorCommandSessionID,
		"running":           true,
		"started_at":        session.StartedAt,
		"scene_path":        session.ScenePath,
		"result":            ack.Result,
	})
}

type StopProjectTool struct{}

func (t *StopProjectTool) Name() string        { return "godot.project.stop" }
func (t *StopProjectTool) Description() string { return "[editor-plugin] Stops the running project" }
func (t *StopProjectTool) Annotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: tooltypes.BoolPtr(false),
	}
}
func (t *StopProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type: "object",
		Properties: map[string]any{
			"session_id":        map[string]any{"type": "string", "description": "Optional target game session id"},
			"editor_session_id": map[string]any{"type": "string", "description": "Optional explicit editor session id override"},
		},
		Required: []string{},
		Title:    "Stop Project",
	}
}
func (t *StopProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	arguments := map[string]any{}
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}
	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewRuntimeNotAvailableError("Project stop requires initialized session", t.Name(), "editor_session_missing", nil)
	}
	targetSessionID := strings.TrimSpace(extractString(arguments["session_id"]))
	if targetSessionID == "" {
		resolvedEditorSessionID, semErr := tooltypes.ResolveFreshEditorSessionID(arguments, ctx, t.Name(), "Project stop requires healthy editor snapshot")
		if semErr != nil {
			return nil, semErr
		}
		if active, ok := runtimebridge.DefaultGameSessionRegistry().ActiveForEditor(resolvedEditorSessionID); ok {
			targetSessionID = active.SessionID
		}
	}

	editorCommandSessionID, semErr := resolveProjectEditorCommandSessionID(arguments, ctx, targetSessionID, t.Name())
	if semErr != nil {
		return nil, semErr
	}
	ack, ok, reason := runtimebridge.DefaultCommandBroker().DispatchAndWait(editorCommandSessionID, t.Name(), map[string]any{
		"session_id": targetSessionID,
	}, projectCommandTimeout)
	if !ok {
		return nil, tooltypes.NewRuntimeNotAvailableError("Project stop bridge is unavailable", t.Name(), mapProjectCommandReason(reason), map[string]any{
			"reason": reason,
		})
	}
	if !ack.Success {
		return nil, tooltypes.NewRuntimeNotAvailableError("Project stop failed", t.Name(), "game_not_running", map[string]any{
			"reason": ack.Error,
		})
	}

	if targetSessionID != "" {
		runtimebridge.DefaultGameSessionRegistry().StopSession(targetSessionID, time.Now().UTC())
		runtimebridge.DefaultRuntimeSnapshotStore().RemoveSession(targetSessionID)
		runtimebridge.DefaultRuntimeLogStore().RemoveSession(targetSessionID)
	}
	return json.Marshal(map[string]any{
		"success":           true,
		"source":            "runtime",
		"session_id":        targetSessionID,
		"editor_session_id": editorCommandSessionID,
		"running":           false,
		"result":            ack.Result,
	})
}

func GetAllTools() []tooltypes.Tool {
	return []tooltypes.Tool{
		&GetProjectSettingsTool{},
		&ListProjectResourcesTool{},
		&GetEditorStateTool{},
		&RunProjectTool{},
		&StopProjectTool{},
		&IsProjectRunningTool{},
	}
}

func dispatchProjectRuntimeCommand(rawArgs json.RawMessage, commandName string) ([]byte, error) {
	return tooltypes.DispatchRuntimeCommand(tooltypes.RuntimeCommandDispatchOptions{
		RawArgs:                  rawArgs,
		CommandName:              commandName,
		Timeout:                  projectCommandTimeout,
		SessionRequiredMessage:   "Project execution requires an initialized MCP HTTP session",
		BridgeUnavailableMessage: "Project execution bridge is unavailable",
	})
}

func mapProjectCommandReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "command_ack_timeout":
		return "command_timeout"
	case "command_transport_unavailable":
		return "capability_not_enabled"
	case "session_missing":
		return "editor_session_missing"
	default:
		return "capability_not_enabled"
	}
}

func resolveProjectEditorCommandSessionID(arguments map[string]any, ctx tooltypes.MCPContext, gameSessionID string, toolName string) (string, *tooltypes.SemanticError) {
	if explicitEditorSessionID, provided, semErr := tooltypes.ParseOptionalEditorSessionID(arguments, toolName); semErr != nil {
		return "", semErr
	} else if provided {
		if _, ok, reason := runtimebridge.DefaultEditorStore().FreshForSession(explicitEditorSessionID, time.Now().UTC()); !ok {
			return "", tooltypes.NewRuntimeNotAvailableError("Project command requires healthy editor snapshot", toolName, reason, map[string]any{
				"editor_session_id": explicitEditorSessionID,
				"resolution":        "explicit_editor_session_id",
			})
		}
		return explicitEditorSessionID, nil
	}

	if session, ok := runtimebridge.DefaultGameSessionRegistry().Session(strings.TrimSpace(gameSessionID)); ok {
		if editorSessionID := strings.TrimSpace(session.EditorSessionID); editorSessionID != "" {
			return editorSessionID, nil
		}
	}

	return tooltypes.ResolveFreshEditorSessionID(arguments, ctx, toolName, "Project command requires healthy editor snapshot")
}

func extractString(raw any) string {
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func generateGameSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("game_%d", time.Now().UTC().UnixNano())
	}
	return "game_" + hex.EncodeToString(buf)
}

func generateLaunchToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("launch_%d", time.Now().UTC().UnixNano())
	}
	return "launch_" + hex.EncodeToString(buf)
}

func cleanupFailedRunSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	runtimebridge.DefaultGameSessionRegistry().RemoveSession(sessionID)
	runtimebridge.DefaultRuntimeSnapshotStore().RemoveSession(sessionID)
	runtimebridge.DefaultRuntimeLogStore().RemoveSession(sessionID)
}

type projectSettingEntry struct {
	Key     string `json:"key"`
	Section string `json:"section"`
	Value   any    `json:"value"`
	Raw     string `json:"raw"`
}

type projectResourceEntry struct {
	Path       string `json:"path"`
	Extension  string `json:"extension"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt string `json:"modified_at"`
}

func readProjectSettings() ([]projectSettingEntry, error) {
	projectRoot := tooltypes.ResolveProjectRootFromEnvOrCWD()
	projectFile := filepath.Join(projectRoot, "project.godot")
	raw, err := os.ReadFile(projectFile)
	if err != nil {
		return nil, err
	}

	entries := make([]projectSettingEntry, 0, 128)
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	currentSection := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		section := currentSection
		if section == "" {
			section = "global"
		}
		fullKey := section + "." + key
		rawValue := strings.TrimSpace(value)
		entries = append(entries, projectSettingEntry{
			Key:     fullKey,
			Section: section,
			Value:   parseProjectSettingValue(rawValue),
			Raw:     rawValue,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

func parseProjectSettingValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") && len(trimmed) >= 2 {
		return strings.Trim(trimmed, "\"")
	}
	lowered := strings.ToLower(trimmed)
	if lowered == "true" {
		return true
	}
	if lowered == "false" {
		return false
	}
	if intValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return intValue
	}
	if floatValue, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return floatValue
	}
	return trimmed
}

func parseProjectCursor(rawCursor string, total int) (int, error) {
	cursor := strings.TrimSpace(rawCursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil {
		return 0, tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Invalid cursor value", map[string]any{
			"field":   "cursor",
			"problem": "invalid_cursor",
		})
	}
	if offset < 0 || offset > total {
		return 0, tooltypes.NewSemanticError(tooltypes.SemanticKindInvalidParams, "Invalid cursor value", map[string]any{
			"field":   "cursor",
			"problem": "invalid_cursor",
		})
	}
	return offset, nil
}

func normalizeExtensionFilter(extensions []string) map[string]struct{} {
	if len(extensions) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		trimmed := strings.ToLower(strings.TrimSpace(ext))
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ".") {
			trimmed = "." + trimmed
		}
		out[trimmed] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldSkipProjectDir(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".git", ".godot":
		return true
	default:
		return false
	}
}
