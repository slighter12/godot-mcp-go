# Runtime Log Backlog

This document tracks the current runtime diagnostics coverage and the deferred backlog for full runtime error streaming.

## Current Coverage

The current `godot.runtime.log.get` path is intended to provide a reliable runtime diagnostics stream for the active game session.

Covered now:

- runtime companion lifecycle issues
- runtime bridge transport issues
- runtime register / snapshot push / command ack / log push failures
- runtime command failures for:
  - `godot.runtime.node_properties.get`
  - `godot.runtime.input.tap`
  - `godot.runtime.input.press`
  - `godot.runtime.input.release`
  - `godot.runtime.screenshot.get`
  - `godot.runtime.sync_now`
  - `godot.runtime.log.clear`

Not guaranteed yet:

- Godot-native GDScript parse errors
- Godot-native runtime exceptions outside the companion-owned command paths
- external game process `stderr/stdout` capture

## Deferred Backlog

### 1. Process Output Capture

Goal:

- capture game process `stderr/stdout` and normalize it into the runtime log stream

Blocked by current architecture:

- `godot.project.run` currently uses `EditorInterface.play_main_scene()`
- this does not expose an external process handle in the current repo architecture

Implication:

- implementing this requires a run-model expansion, not a local runtime companion tweak

### 2. Native In-Engine Error Hook

Goal:

- hook stable Godot-native error/debug APIs so GDScript parse/runtime errors can be streamed without changing the run model

Open problem:

- the current repo does not yet prove a stable hook path that works for playable runtime execution and matches the desired error fidelity

Implication:

- this remains a focused follow-up slice and should be validated in-engine before promising coverage

### 3. Full Union Plus Dedupe

Goal:

- merge diagnostics from:
  - runtime companion structured errors
  - process output capture
  - native in-engine error hook
- dedupe repeated events while preserving ordering and source attribution

Status:

- explicitly deferred until both upstream sources exist
