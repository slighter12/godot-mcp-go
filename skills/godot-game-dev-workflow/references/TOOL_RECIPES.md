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

If they fail with session or freshness errors, resolve via `SAFETY_AND_VERIFICATION.md` before continuing.

### Mutating tools

Before calling any mutating tool (`godot.project.run`, `godot.project.stop`, `godot.script.modify`, `godot.script.create`, `godot.node.create`, `godot.node.modify`, `godot.node.delete`, `godot.scene.create`, `godot.scene.save`, `godot.scene.apply`):

- Ensure `initialize.params.capabilities.godot.mutating=true` is already negotiated.
- Call `godot.runtime.health.get` and verify bridge status is healthy.
- If the bridge is unhealthy, resolve it via `SAFETY_AND_VERIFICATION.md` before proceeding.
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

Decision refs:

- `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`
- `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- `../../policy-godot/references/GODOT_GDSCRIPT.md`

Scene setup issue:

- `godot.scene.read` -> inspect the player scene root, child nodes, and comparable setup

Script logic issue:

- `godot.script.read` -> inspect the controller owner, input path, and ability state

Runtime ownership issue:

- `godot.editor.state.get` or `godot.node.tree.get` when live context is needed to confirm the active owner

Change:

- `godot.script.read` -> apply changes -> `godot.script.modify` (full content)
- `godot.node.modify` only if node wiring, exported values, or scene setup also changes

Verify:

- `godot.script.read` readback
- Optional mutating ack payload
- Logical scenario analysis on the controller path

Watch for:

- Input action assumptions
- `_physics_process()` ownership
- landing reset, timers, and animation hooks

Official docs:

- `OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`
- `OFFICIAL_DOCS_MAP.md` -> `GDScript Syntax and Core Patterns`

### Collision or interaction bug

Decision refs:

- `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- `../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`

Scene setup issue:

- `godot.scene.read` -> inspect body types, shapes, collision node placement, and comparable objects

Script logic issue:

- `godot.script.read` -> inspect movement helpers, callbacks, and gameplay reaction ownership

Runtime ownership issue:

- `godot.node.tree.get` -> `godot.node.properties.get` to confirm node path, owner, script, and groups when runtime-backed reads are available

Change:

- `godot.node.modify` for body setup, mask/layer, or shape-owner issues
- `godot.script.read` -> apply fix -> `godot.script.modify` (full content) for callback, movement helper, or reaction ownership issues

Verify:

- Mutating ack payload plus `godot.scene.read` or `godot.script.read` readback
- Logical repro-path analysis using the physics triage order

Watch for:

- Collision layer and mask mismatches
- disabled shapes, unexpected node ownership, or incorrect signal target
- logic that bypasses physics movement helpers

Official docs:

- `OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`

### UI or HUD sync

Decision refs:

- `../../policy-godot/references/GODOT_UI_AND_INPUT.md`
- `../../policy-godot/references/GODOT_SIGNALS.md`
- `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`

Scene setup issue:

- `godot.scene.read` -> inspect the `Control` tree, HUD ownership, and comparable UI widgets

Script logic issue:

- `godot.script.read` -> inspect the gameplay data source, signal path, and UI update code

Runtime ownership issue:

- `godot.node.tree.get` when runtime-backed reads are needed to confirm the active UI owner or gameplay source path

Change:

- `godot.script.read` -> apply fix -> `godot.script.modify` (full content)
- `godot.node.modify` only for minimal UI wiring or property changes

Verify:

- `godot.script.read` readback
- Logical state-to-UI update analysis with one gameplay event and one UI reaction

Watch for:

- duplicate signal connections
- stale state caching
- mixed gameplay and presentation responsibilities
- controller-owned gameplay input drifting into `_unhandled_input()` by default instead of staying on the intended timing path
- `_gui_input()` or built-in UI signals being more appropriate than `_input()`
- `CanvasLayer` being treated as the UI content root when the real need is only screen-space separation for a `Control` subtree

Official docs:

- `OFFICIAL_DOCS_MAP.md` -> `Signals and Scene Communication`
- `OFFICIAL_DOCS_MAP.md` -> `UI, Input, and Screen-Space Layers`

### Scene content or interactable setup

Decision refs:

- `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- `../../policy-godot/references/GODOT_RESOURCE_MANAGEMENT.md`
- `../../policy-godot/references/GODOT_SIGNALS.md`

Scene setup issue:

- `godot.scene.read` -> inspect the target hierarchy and comparable content

Script logic issue:

- `godot.script.read` when a content hook or reusable script owner is involved

Runtime ownership issue:

- `godot.node.tree.get` when the active edited scene or live hierarchy matters

Change:

- If the target scene is not the currently edited scene, use `godot.scene.apply` first
- Use `godot.node.create` or `godot.node.modify` for minimal node-tree changes
- Use `godot.scene.save` to persist
- Use `godot.scene.create` only when the slice truly needs a new scene file

Verify:

- `godot.node.tree.get` or `godot.scene.read` readback
- Logical player interaction analysis based on the final hierarchy

Watch for:

- scene ownership drifting away from the current composition pattern
- duplicated nodes or resources where an existing reusable scene would fit better
- interaction wiring that depends on brittle cross-scene lookups

Official docs:

- `OFFICIAL_DOCS_MAP.md` -> `Movement, Physics, and Collision`
- `OFFICIAL_DOCS_MAP.md` -> `Scene Structure and Node Types`
- `OFFICIAL_DOCS_MAP.md` -> `Resources, Packed Scenes, and Loading`

### Refactor without behavior change

Decision refs:

- `../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`
- `../../policy-godot/references/GODOT_GDSCRIPT.md`
- `../../policy-godot/references/GODOT_LIFECYCLE.md`

Scene setup issue:

- `godot.scene.read` when node dependencies or exported setup are part of the current behavior contract

Script logic issue:

- `godot.script.read` to understand structure and call sites
- `godot.script.analyze` as a complexity indicator only

Runtime ownership issue:

- `godot.node.tree.get` only when runtime ownership is unclear and affects the refactor boundary

Change:

- `godot.script.read` -> apply refactor -> `godot.script.modify` (full content)

Verify:

- `godot.script.read` readback
- Logical scenario check that ownership, callback timing, and state boundaries still match the old behavior

Watch for:

- helper extraction that changes ownership or timing assumptions
- accidental new abstractions that were not needed for the slice
- style changes that drift away from the surrounding script conventions
- ownership drift
- callback drift
- state-boundary drift

Official docs:

- `OFFICIAL_DOCS_MAP.md` -> `GDScript Syntax and Core Patterns`

### New scene or script creation

- Use `godot.scene.create` to create a new `.tscn` file.
- Use `godot.scene.apply` after `godot.scene.create` when follow-up node mutations need the new scene to become the active edited scene.
- Use `godot.script.create` to create a new `.gd` or `.rs` file. Set `replace: true` only when intentionally overwriting.
- After creation, wire the new asset into the existing project using the recipes above.

Relevant policy refs:

- `../../policy-godot/references/GODOT_GDSCRIPT.md`
- `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- `../../policy-godot/references/GODOT_RESOURCE_MANAGEMENT.md`
- `../../policy-godot/references/GODOT_RUST.md`

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
- Route design questions to `../../policy-godot/SKILL.md`, then return to the recipe.
- When the task needs GDScript syntax or Godot engine semantics, route to `OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.
