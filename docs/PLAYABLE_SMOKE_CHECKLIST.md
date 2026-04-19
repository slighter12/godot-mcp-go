# Runtime Control V1 Playable Smoke Checklist

This document defines the expected request/response checkpoints for the 14-step playable smoke flow.

Scope:

- Streamable HTTP transport only
- Initialized MCP session
- Mutating capability negotiated unless compatibility fallback is explicitly enabled
- Runtime companion installed and active

Conventions:

- Dynamic values use angle-bracket placeholders such as `<game_session_id>`
- Additional fields are allowed unless this document says otherwise
- Validation should focus on required fields, field types, and behavioral assertions

## Preconditions

Before step 1:

1. The MCP client has completed `initialize`
2. The MCP client has sent `notifications/initialized`
3. The session has mutating capability enabled for `godot.project.run`, `godot.project.stop`, and runtime input tools
4. The Godot editor plugin is connected
5. The runtime companion addon is available to the running game

## Dynamic Field Rules

Use these rules throughout the flow:

- `<game_session_id>`: non-empty string starting with `game_`
- `<snapshot_id>`: non-empty string starting with `snap_`
- `<command_id>`: non-empty string
- `<timestamp>`: RFC3339/RFC3339Nano string
- `<frame>`: integer, monotonically non-decreasing during the flow
- `<player_path>`: exact runtime node path found in step 4

## 14-Step Flow

| Step | Tool | Request | Expected JSON Shape | Acceptance Rule |
| --- | --- | --- | --- | --- |
| 1 | `godot.project.run` | `{"scene_path":"res://scenes/mvp0/mvp0_root.tscn"}` or `{}` | `{"success":true,"source":"runtime","session_id":"<game_session_id>","running":true,"started_at":"<timestamp>","scene_path":"<scene_path>"}` | Call succeeds and returns a non-empty game session id. Keep `session_id` for all later steps. |
| 2 | `godot.runtime.await_snapshot` | `{"session_id":"<game_session_id>","timeout_ms":3000}` | `{"source":"runtime","session_id":"<game_session_id>","snapshot_id":"<snapshot_id>","frame":<frame>,"updated_at":"<timestamp>","freshness":"fresh","root_scene_path":"<scene_path>","root_node_name":"<root_name>"}` | First live runtime snapshot is available within timeout and freshness is `fresh` or, at worst, `grace` if you intentionally relax the request. |
| 3 | `godot.runtime.scene_tree.get` | `{"session_id":"<game_session_id>","max_depth":4}` | `{"source":"runtime","session_id":"<game_session_id>","snapshot_id":"<snapshot_id>","frame":<frame>,"updated_at":"<timestamp>","root":{...},"root_scene_path":"<scene_path>","root_node_name":"<root_name>"}` | Response contains a runtime tree rooted at the running game, not editor state. |
| 4 | `godot.runtime.scene_tree.get` result inspection | none | `root` contains a descendant node named `PlayerRoot` | Record the exact runtime path as `<player_path>`. If not found, the smoke flow fails. |
| 5 | `godot.runtime.input.press` | `{"session_id":"<game_session_id>","input":"ui_right"}` | `{"source":"runtime","session_id":"<game_session_id>","command_id":"<command_id>","input":"ui_right","frame":<frame>,"timestamp":"<timestamp>"}` | Press command is acknowledged by the runtime session. |
| 6 | wait | none | none | Wait about `300ms` in the client before step 7. |
| 7 | `godot.runtime.input.release` | `{"session_id":"<game_session_id>","input":"ui_right"}` | `{"source":"runtime","session_id":"<game_session_id>","command_id":"<command_id>","input":"ui_right","frame":<frame>,"timestamp":"<timestamp>"}` | Release command is acknowledged by the same runtime session. |
| 8 | `godot.runtime.node_properties.get` | `{"session_id":"<game_session_id>","node":"<player_path>","properties":["position","velocity"]}` | `{"source":"runtime","session_id":"<game_session_id>","command_id":"<command_id>","node":"<player_path>","type":"<node_type>","properties":{"position":{"x":<number>,"y":<number>},"velocity":{"x":<number>,"y":<number>}},"snapshot_id":"<snapshot_id>","frame":<frame>,"updated_at":"<timestamp>"}` | `position.x` changed relative to the pre-move baseline, or `velocity.x` is non-zero during/after the movement window. |
| 9 | `godot.runtime.input.tap` | `{"session_id":"<game_session_id>","input":"KEY_SPACE","duration_ms":120}` | `{"source":"runtime","session_id":"<game_session_id>","command_id":"<command_id>","input":"KEY_SPACE","frame":<frame>,"timestamp":"<timestamp>"}` | Tap command is acknowledged by the runtime session. |
| 10 | `godot.runtime.node_properties.get` | `{"session_id":"<game_session_id>","node":"<player_path>","properties":["velocity"]}` | `{"source":"runtime","session_id":"<game_session_id>","node":"<player_path>","properties":{"velocity":{"x":<number>,"y":<number>}},"snapshot_id":"<snapshot_id>","frame":<frame>}` | `velocity.y` changes in the direction expected by the project jump implementation. For most upward-jump controllers this means a negative `y`, but validate against project physics conventions. |
| 11 | `godot.runtime.screenshot.get` | `{"session_id":"<game_session_id>","mode":"viewport"}` | `{"source":"runtime","session_id":"<game_session_id>","command_id":"<command_id>","path":"<absolute_png_path>","width":<int>,"height":<int>,"frame":<frame>,"timestamp":"<timestamp>"}` | Screenshot file path is absolute, ends with `.png`, and width/height are positive integers. |
| 12 | `godot.runtime.log.get` | `{"session_id":"<game_session_id>","level":"error","limit":50}` | `{"source":"runtime","session_id":"<game_session_id>","entries":[...]}` | Request succeeds. An empty array is acceptable for a clean run. Non-empty entries must each include `sequence`, `time`, `level`, and `message`. |
| 13 | `godot.project.is_running` | `{"session_id":"<game_session_id>"}` | `{"source":"runtime","session_id":"<game_session_id>","running":true,"started_at":"<timestamp>","scene_path":"<scene_path>"}` | The game is still running before shutdown. |
| 14 | `godot.project.stop` | `{"session_id":"<game_session_id>"}` | `{"success":true,"source":"runtime","session_id":"<game_session_id>","running":false}` | Session stops cleanly and subsequent runtime tools should fail with `game_not_running` or `game_session_missing`. |

## Optional Cleanup

After step 14, you may clear buffered logs for the next test cycle:

```json
{
  "session_id": "<game_session_id>"
}
```

Tool:

- `godot.runtime.log.clear`

Expected shape:

```json
{
  "source": "runtime",
  "session_id": "<game_session_id>",
  "cleared": <int>,
  "command_id": "<command_id>"
}
```

## Negative Diagnostics Checks

Run these after the main 14-step flow or in an isolated smoke cycle.

### Unsupported input

1. Call `godot.runtime.input.tap` with:

    ```json
    {
        "session_id": "<game_session_id>",
        "input": "KEY_DOES_NOT_EXIST"
    }
    ```

2. Expected result:

   - semantic error with `code="input_not_supported"`

3. Then call:

    ```json
    {
        "session_id": "<game_session_id>",
        "level": "error",
        "limit": 20
    }
    ```

4. Acceptance:
   - at least one entry exists with `source="runtime_command:godot.runtime.input.tap"`

### Unsupported property

1. Call `godot.runtime.node_properties.get` with:

    ```json
    {
        "session_id": "<game_session_id>",
        "node": "<player_path>",
        "properties": ["rotation_degrees"]
    }
    ```

2. Expected result:

   - semantic error with `code="property_not_supported"`

3. Then call `godot.runtime.log.get(level="error")`
4. Acceptance:

   - at least one entry exists with `source="runtime_command:godot.runtime.node_properties.get"`

### Unsupported screenshot mode

1. Call `godot.runtime.screenshot.get` with:

    ```json
    {
        "session_id": "<game_session_id>",
        "mode": "window"
    }
    ```

2. Expected result:
   - semantic error from runtime command path
3. Then call `godot.runtime.log.get(level="error")`
4. Acceptance:
   - at least one entry exists with `source="runtime_command:godot.runtime.screenshot.get"`

## Recommended Failure Checks

When the flow fails, inspect these machine-readable codes first:

- `editor_session_missing`
- `game_session_missing`
- `game_not_running`
- `runtime_snapshot_missing`
- `runtime_snapshot_stale`
- `node_not_found`
- `property_not_supported`
- `input_not_supported`
- `capability_not_enabled`
- `command_timeout`

## Known Current Limitation

`godot.runtime.log.get(level="error")` is wired and session-scoped, but this repo has not yet proven a full hook for Godot-native GDScript parse/runtime error streams during live gameplay. Treat step 12 as a transport/runtime log verification step unless that hook is added and validated in-engine.

Backlog reference:

- see [RUNTIME_LOG_BACKLOG.md](./RUNTIME_LOG_BACKLOG.md)
