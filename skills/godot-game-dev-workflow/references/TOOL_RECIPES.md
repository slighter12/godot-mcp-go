# Tool Recipes

Use tools to support development decisions, not to define the task flow on their own.

## Tool Preconditions

### File-backed reads

These tools read project files directly and do not require the runtime bridge:

- `godot.scene.list`
- `godot.scene.read`
- `godot.script.list`
- `godot.script.read`
- `godot.script.analyze`
- `godot.project.settings.get`
- `godot.project.resources.list`

They operate on the Godot project resolved by `GODOT_PROJECT_ROOT` or, when unset, the server working directory and nearest `project.godot`.

### Runtime-backed reads

These tools depend on initialized MCP HTTP session state and a fresh runtime snapshot:

- `godot.editor.state.get`
- `godot.node.tree.get`
- `godot.node.properties.get`

If they fail with session or freshness errors, resolve via `references/SAFETY_AND_VERIFICATION.md` before continuing.

### Mutating tools

Before calling any mutating tool (`godot.project.run`, `godot.project.stop`, `godot.script.modify`, `godot.script.create`, `godot.node.create`, `godot.node.modify`, `godot.node.delete`, `godot.scene.create`, `godot.scene.save`, `godot.scene.apply`):

- Ensure `initialize.params.capabilities.godot.mutating=true` is already negotiated.
- Call `godot.runtime.health.get` and verify bridge status is healthy.
- If the bridge is unhealthy, resolve it via `references/SAFETY_AND_VERIFICATION.md` before proceeding.
- `godot.node.create`, `godot.node.modify`, `godot.node.delete`, and `godot.scene.save` operate on the currently edited scene. If the target scene is different, open it with `godot.scene.apply` first.

## Script Modification Constraint

`godot.script.modify` replaces the **entire** script content (not a diff or patch). The required workflow is:

1. `godot.script.read` -> get the current full content.
2. Apply changes to the full text in memory.
3. `godot.script.modify` -> send the complete new content.

Never call `godot.script.modify` without reading the script first.

## Find The Current Owner

### Start from the active gameplay surface

- `godot.editor.state.get` -> confirm the active scene or editor context when runtime-backed reads are available
- `godot.scene.list` -> locate the scene if the owner is not already known
- `godot.scene.read` -> inspect the candidate scene structure

### Inspect the behavior boundary

- `godot.node.tree.get` -> identify owner node path and nearby collaborators when runtime-backed reads are available
- `godot.node.properties.get` -> inspect runtime metadata currently exposed by the snapshot (`path`, `name`, `type`, `owner`, `script`, `groups`, `child_count`) when runtime-backed reads are available
- `godot.script.read` -> inspect the script that currently owns the behavior

Use `godot.script.list` only when the owner script is not obvious from the scene.
If runtime-backed reads are unavailable, stay with file-backed inspection first, then return to runtime-backed reads once session and snapshot health are restored.

## Feature-Oriented Recipes

### Player ability or movement change

- Inspect: `godot.scene.read` -> `godot.script.read`, then `godot.editor.state.get` when live context is needed
- Change: `godot.script.read` -> apply changes -> `godot.script.modify` (full content), and `godot.node.modify` only if tuning or node wiring also changes
- Verify: `godot.script.read` readback plus logical scenario analysis

Watch for:

- Input action assumptions
- `_physics_process()` ownership
- landing reset, timers, and animation hooks

Official docs:

- `references/OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`
- `references/OFFICIAL_DOCS_MAP.md` -> `GDScript Syntax and Core Patterns`

### Collision or interaction bug

- Inspect: `godot.scene.read` -> `godot.script.read`, then `godot.node.tree.get` -> `godot.node.properties.get` when runtime-backed reads are available to confirm node path, owner, script, and groups
- Change: `godot.node.modify` for setup issues, `godot.script.read` -> apply fix -> `godot.script.modify` (full content) for movement or callback logic
- Verify: use the mutating ack payload plus `godot.scene.read` or `godot.script.read` readback where the changed state is persisted, then do logical repro path analysis

Watch for:

- Collision layer and mask mismatches
- disabled shapes, unexpected node ownership, or incorrect signal target
- logic that bypasses physics movement helpers

Official docs:

- `references/OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`

### UI or HUD sync

- Inspect: `godot.scene.read` -> `godot.script.read`, then `godot.node.tree.get` when runtime-backed reads are available
- Change: `godot.script.read` -> apply fix -> `godot.script.modify` (full content) and only the minimal UI node/property wiring required
- Verify: `godot.script.read` readback and logical state-to-UI update analysis

Watch for:

- duplicate signal connections
- stale state caching
- mixed gameplay and presentation responsibilities

Official docs:

- `references/OFFICIAL_DOCS_MAP.md` -> `Signals and Scene Communication`

### Scene content or interactable setup

- Inspect: `godot.scene.read`, then `godot.node.tree.get` when runtime-backed reads are available
- Change: if the target scene is not the currently edited scene, use `godot.scene.apply` first. Then use `godot.node.create` or `godot.node.modify`, and `godot.scene.save` to persist. Use `godot.scene.create` when the task requires a new scene file.
- Verify: `godot.node.tree.get` readback and logical player interaction analysis

Official docs:

- `references/OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`

### Refactor without behavior change

- Inspect: `godot.script.read` to understand structure and call sites. `godot.script.analyze` provides line/function counts as a complexity indicator but does not perform semantic analysis.
- Change: `godot.script.read` -> apply refactor -> `godot.script.modify` (full content)
- Verify: `godot.script.read` readback confirms the same behavior with improved structure

Official docs:

- `references/OFFICIAL_DOCS_MAP.md` -> `GDScript Syntax and Core Patterns`

### New scene or script creation

- Use `godot.scene.create` to create a new `.tscn` file.
- Use `godot.scene.apply` after `godot.scene.create` when follow-up node mutations need the new scene to become the active edited scene.
- Use `godot.script.create` to create a new `.gd` or `.rs` file. Set `replace: true` only when intentionally overwriting.
- After creation, wire the new asset into the existing project using the recipes above.

## Supporting Context Tools

Use when the task depends on project-wide configuration:

- `godot.project.settings.get` for input map or engine setting assumptions
- `godot.project.resources.list` for reusable assets, scenes, or resources
- `godot.runtime.health.get` as a precondition before mutating, or when runtime state is suspect
- `godot.project.run` to launch the game for manual user testing
- `godot.project.stop` to stop the running game

## Rules

- Always prefer inspect-first before mutation.
- Keep tools subordinate to one concrete gameplay slice.
- Prefer editing the current owner over creating a parallel owner.
- For mutating tools, ensure capability negotiation is already satisfied before attempting the slice.
- When the task needs GDScript syntax or Godot engine semantics, route to `references/OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.
