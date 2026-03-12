# Task Playbooks

Use these playbooks to keep work scoped to one gameplay slice and one clear ownership boundary.

## Player Ability Or Movement

When this lane applies:

- Add or adjust a player ability, controller mechanic, or movement rule such as double jump, dash, or coyote time.

Primary policy refs:

- `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`
- `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- `../../policy-godot/references/GODOT_GDSCRIPT.md`

Primary MCP reads:

- `godot.scene.read`
- `godot.script.read`
- `godot.project.settings.get` when input actions matter

Mutation boundary:

- Extend the existing controller owner.
- Change one movement or ability path at a time.
- Avoid adding a parallel controller, new global state, or a new state machine.

Verification focus:

- Primary scenario such as jump, dash, or landing behavior.
- Adjacent regression such as wall contact, animation timing, or input buffering.
- Owner and callback sanity on the final movement path.

## Collision Or Interaction Bug

When this lane applies:

- Resolve pass-through, trigger, pickup, overlap, hurtbox, hitbox, or body-type bugs.

Primary policy refs:

- `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- `../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`

Primary MCP reads:

- `godot.scene.read`
- `godot.script.read`
- `godot.node.tree.get`
- `godot.node.properties.get`

Mutation boundary:

- Fix one root cause only: node type, callback, movement helper, layers/masks, monitoring, or owner wiring.
- Avoid mixing setup fixes with broader architecture rewrites.

Verification focus:

- Exact repro path.
- One adjacent interaction such as slopes, pickups, detection, or damage zones.
- Physics owner, body type, and callback sanity after the change.

## Scene Composition Or Content Setup

When this lane applies:

- Add a new interactable, reusable object, level chunk, or content-facing node tree.

Primary policy refs:

- `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- `../../policy-godot/references/GODOT_RESOURCE_MANAGEMENT.md`
- `../../policy-godot/references/GODOT_SIGNALS.md`

Primary MCP reads:

- `godot.scene.read`
- `godot.project.resources.list`
- `godot.node.tree.get` when runtime context matters

Mutation boundary:

- Reuse an existing composition pattern or subscene when possible.
- Add only the minimal node tree, resource wiring, and script hook needed for the slice.
- Avoid introducing new wrapper scenes or global coordination by default.

Verification focus:

- Placement and ownership in the target scene.
- Trigger or interaction path.
- Reuse sanity: no unnecessary duplication of existing assets or patterns.

## UI Or HUD Sync

When this lane applies:

- Sync HUD, prompt, or UI widgets to gameplay state, or fix UI input behavior tied to gameplay events.

Primary policy refs:

- `../../policy-godot/references/GODOT_UI_AND_INPUT.md`
- `../../policy-godot/references/GODOT_SIGNALS.md`
- `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`

Primary MCP reads:

- `godot.scene.read`
- `godot.script.read`
- `godot.node.tree.get` when runtime ownership is unclear

Mutation boundary:

- Keep gameplay as the source of truth and UI as presentation.
- Add or fix one signal or callback path at a time.
- Avoid moving gameplay rules into `Control` code.

Verification focus:

- One gameplay event reaching one UI update path.
- No duplicate connections or stale cached state.
- Correct UI/input ownership for the final callback path.
- Input ownership does not drift from the controller or gameplay owner into a UI callback without an explicit reason.

## Enemy Behavior Or State Transition

When this lane applies:

- Add or fix one enemy reaction, AI transition, detection rule, or animation-driven gameplay response.

Primary policy refs:

- `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`
- `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- `../../policy-godot/references/GODOT_SIGNALS.md`

Primary MCP reads:

- `godot.scene.read`
- `godot.script.read`
- `godot.node.tree.get` when detection ownership is unclear

Mutation boundary:

- Reuse the current state representation before introducing a new one.
- Implement one transition or reaction at a time.
- Avoid introducing a state machine unless the existing representation is already insufficient.

Verification focus:

- Entry, exit, and fallback behavior for the changed transition.
- Detection ownership and signal path.
- Animation and gameplay ownership sanity after the change.

## Refactor Without Gameplay Change

When this lane applies:

- Extract or simplify script structure without changing behavior.

Primary policy refs:

- `../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`
- `../../policy-godot/references/GODOT_GDSCRIPT.md`
- `../../policy-godot/references/GODOT_LIFECYCLE.md`

Primary MCP reads:

- `godot.script.read`
- `godot.script.analyze`
- `godot.scene.read` when node dependencies affect the refactor

Mutation boundary:

- Preserve ownership, callback timing, and state boundaries.
- Extract one coherent function or helper boundary per slice.
- Do not mix refactor work with new gameplay behavior.

Verification focus:

- Previous scenario still reasons the same way.
- No ownership or callback drift.
- One clear next safe extraction candidate.
