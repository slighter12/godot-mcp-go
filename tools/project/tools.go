package project

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
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

func (t *GetProjectSettingsTool) Name() string        { return "godot-project-get-settings" }
func (t *GetProjectSettingsTool) Description() string { return "Gets project settings" }
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

func (t *ListProjectResourcesTool) Name() string        { return "godot-project-list-resources" }
func (t *ListProjectResourcesTool) Description() string { return "Lists all resources in the project" }
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

func (t *GetEditorStateTool) Name() string        { return "godot-editor-get-state" }
func (t *GetEditorStateTool) Description() string { return "Gets the current editor state" }
func (t *GetEditorStateTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Get Editor State"}
}
func (t *GetEditorStateTool) Execute(args json.RawMessage) ([]byte, error) {
	var arguments map[string]any
	if err := json.Unmarshal(args, &arguments); err != nil {
		return nil, err
	}

	ctx := tooltypes.ExtractMCPContext(arguments)
	if strings.TrimSpace(ctx.SessionID) == "" || !ctx.SessionInitialized {
		return nil, tooltypes.NewNotAvailableError("Editor state requires an initialized MCP HTTP session", map[string]any{
			"feature": "runtime_bridge",
			"reason":  "session_not_initialized",
			"tool":    t.Name(),
		})
	}

	stored, ok, reason := runtimebridge.DefaultStore().FreshForSession(ctx.SessionID, time.Now().UTC())
	if !ok {
		return nil, tooltypes.NewNotAvailableError("Editor state is unavailable until runtime sync is healthy", map[string]any{
			"feature": "runtime_bridge",
			"reason":  reason,
			"tool":    t.Name(),
		})
	}
	result := map[string]any{
		"active_scene":  stored.Snapshot.RootSummary.ActiveScene,
		"active_script": stored.Snapshot.RootSummary.ActiveScript,
		"root_summary":  stored.Snapshot.RootSummary,
		"session_id":    stored.SessionID,
		"updated_at":    stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(result)
}

type RunProjectTool struct{}

func (t *RunProjectTool) Name() string        { return "godot-project-run" }
func (t *RunProjectTool) Description() string { return "Runs the project" }
func (t *RunProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Run Project"}
}
func (t *RunProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchProjectRuntimeCommand(args, t.Name())
}

type StopProjectTool struct{}

func (t *StopProjectTool) Name() string        { return "godot-project-stop" }
func (t *StopProjectTool) Description() string { return "Stops the running project" }
func (t *StopProjectTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "Stop Project"}
}
func (t *StopProjectTool) Execute(args json.RawMessage) ([]byte, error) {
	return dispatchProjectRuntimeCommand(args, t.Name())
}

func GetAllTools() []tooltypes.Tool {
	return []tooltypes.Tool{
		&GetProjectSettingsTool{},
		&ListProjectResourcesTool{},
		&GetEditorStateTool{},
		&RunProjectTool{},
		&StopProjectTool{},
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
