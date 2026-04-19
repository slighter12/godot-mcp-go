package utility

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/slighter12/godot-mcp-go/mcp"
	"github.com/slighter12/godot-mcp-go/runtimebridge"
)

type ListOfferingsTool struct{}

func (t *ListOfferingsTool) Name() string        { return "godot.offerings.list" }
func (t *ListOfferingsTool) Description() string { return "Lists available offerings" }
func (t *ListOfferingsTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{Type: "object", Properties: map[string]any{}, Required: []string{}, Title: "List Offerings"}
}
func (t *ListOfferingsTool) Execute(args json.RawMessage) ([]byte, error) {
	result := map[string]any{"offerings": []map[string]any{{"name": "godot-mcp", "version": "0.2.0", "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}, "prompts": map[string]any{}}, "serverInfo": map[string]any{"name": "godot-mcp-go", "version": "0.2.0"}}}}
	return json.Marshal(result)
}

// RuntimeHealthTool returns runtime bridge freshness and command broker metrics.
type RuntimeHealthTool struct{}

func NewRuntimeHealthTool() *RuntimeHealthTool {
	return &RuntimeHealthTool{}
}

func (t *RuntimeHealthTool) Name() string { return "godot.runtime.health.get" }

func (t *RuntimeHealthTool) Description() string {
	return "Returns runtime bridge freshness and command broker health metrics"
}

func (t *RuntimeHealthTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
		Title:      "Get Runtime Health",
	}
}

func (t *RuntimeHealthTool) Execute(args json.RawMessage) ([]byte, error) {
	return json.Marshal(runtimebridge.HealthSnapshot(time.Now().UTC()))
}

// RuntimeDiagnoseTool returns a structured diagnostic report for the runtime
// bootstrap pipeline (game session → editor fresh → companion connected →
// registered → first snapshot).
type RuntimeDiagnoseTool struct{}

func NewRuntimeDiagnoseTool() *RuntimeDiagnoseTool {
	return &RuntimeDiagnoseTool{}
}

func (t *RuntimeDiagnoseTool) Name() string { return "godot.runtime.diagnose" }

func (t *RuntimeDiagnoseTool) Description() string {
	return "Diagnoses runtime bootstrap pipeline — shows which step is stuck (game session, editor freshness, companion connection, registration, first snapshot)"
}

func (t *RuntimeDiagnoseTool) InputSchema() mcp.InputSchema {
	return mcp.InputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
		Title:      "Diagnose Runtime Pipeline",
	}
}

func (t *RuntimeDiagnoseTool) Execute(args json.RawMessage) ([]byte, error) {
	now := time.Now().UTC()

	// Game session info
	gameSessionInfo := map[string]any{
		"exists": false,
	}
	gameSession, hasGame := runtimebridge.DefaultGameSessionRegistry().LatestRunning()
	if hasGame {
		gameSessionInfo = map[string]any{
			"exists":                true,
			"session_id":            gameSession.SessionID,
			"running":               gameSession.Running,
			"has_snapshot":          gameSession.HasSnapshot,
			"runtime_session_id":    gameSession.RuntimeSessionID,
			"editor_session_id":     gameSession.EditorSessionID,
			"launch_token_present":  strings.TrimSpace(gameSession.LaunchToken) != "",
			"started_at":            gameSession.StartedAt,
		}
	}

	// Editor store info
	editorHealth := runtimebridge.DefaultEditorStore().Health(now)
	editorFresh := editorHealth.States["fresh"]

	// MCP session counts
	mcpCounts := runtimebridge.GetSessionCounts()

	// Build pipeline checklist
	checklist := buildPipelineChecklist(hasGame, gameSession, editorFresh, mcpCounts)

	result := map[string]any{
		"timestamp":          now.Format(time.RFC3339Nano),
		"game_session":       gameSessionInfo,
		"mcp_sessions":       mcpCounts,
		"editor_store": map[string]any{
			"sessions":    editorHealth.Sessions,
			"fresh_count": editorFresh,
		},
		"pipeline_checklist": checklist,
	}
	return json.Marshal(result)
}

type pipelineStep struct {
	Step string `json:"step"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint,omitempty"`
}

func buildPipelineChecklist(hasGame bool, game runtimebridge.GameSession, editorFresh int, mcpCounts map[string]any) []pipelineStep {
	steps := make([]pipelineStep, 0, 5)

	// Step 1: game session exists
	gameOK := hasGame
	step1 := pipelineStep{Step: "game_session_exists", OK: gameOK}
	if !gameOK {
		step1.Hint = "no running game session — call project.run first"
	}
	steps = append(steps, step1)

	// Step 2: editor session fresh
	editorOK := editorFresh > 0
	step2 := pipelineStep{Step: "editor_session_fresh", OK: editorOK}
	if !editorOK {
		step2.Hint = "no fresh editor session — is the Godot editor plugin connected?"
	}
	steps = append(steps, step2)

	// Step 3: runtime companion connected (expect 3+ sessions: editor, AI, runtime)
	fullyInitialized, _ := mcpCounts["fully_initialized"].(int)
	runtimeConnected := fullyInitialized >= 3
	step3 := pipelineStep{Step: "runtime_session_connected", OK: runtimeConnected}
	if !runtimeConnected {
		step3.Hint = "runtime companion MCP session not found — check: (1) godot_mcp_runtime plugin enabled in Project Settings > Plugins, (2) handshake file exists at user://godot_mcp/runtime/active_handshake.json, (3) Go server reachable at configured URL"
	}
	steps = append(steps, step3)

	// Step 4: runtime registered
	registeredOK := hasGame && strings.TrimSpace(game.RuntimeSessionID) != ""
	step4 := pipelineStep{Step: "runtime_session_registered", OK: registeredOK}
	if !registeredOK {
		if !runtimeConnected {
			step4.Hint = "depends on runtime_session_connected"
		} else {
			step4.Hint = "runtime companion connected but register failed — check Go server logs for 'runtime.register rejected' with launch_token_mismatch or game_session_missing"
		}
	}
	steps = append(steps, step4)

	// Step 5: first snapshot received
	snapshotOK := hasGame && game.HasSnapshot
	step5 := pipelineStep{Step: "first_snapshot_received", OK: snapshotOK}
	if !snapshotOK {
		if !registeredOK {
			step5.Hint = "depends on runtime_session_registered"
		} else {
			step5.Hint = "runtime registered but no snapshot yet — check Go server logs for 'runtime snapshot rejected'"
		}
	}
	steps = append(steps, step5)

	return steps
}
