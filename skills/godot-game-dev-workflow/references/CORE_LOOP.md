# Core Loop

Use this loop for feature work, bug fixes, and safe refactors.

## Step 1: Define the gameplay outcome

Ask:

- What should the player see, feel, or be able to do after this slice?
- What single acceptance scenario proves the slice is working?

Output:

- One player-facing goal.
- One acceptance check.

## Task Lane Gateway

Route the task to one primary lane before selecting tools:

- Controller or gameplay slice -> `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`
- Collision or physics bug -> `../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`
- Scene composition or content setup -> `../../policy-godot/references/GODOT_SCENE_STRUCTURE.md`
- UI or HUD sync -> `../../policy-godot/references/GODOT_UI_AND_INPUT.md`
- AI or state transition -> `../../policy-godot/references/GODOT_GAMEPLAY_PATTERNS.md`
- Refactor without behavior change -> `../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`

If the task spans multiple lanes, choose the one that owns the primary acceptance check and defer the others to a later slice.

## Step 2: Confirm runtime bridge and inspect the actual owner

Precondition:

- File-backed reads (`godot.scene.list`, `godot.scene.read`, `godot.script.list`, `godot.script.read`, `godot.script.analyze`, `godot.project.settings.get`, `godot.project.resources.list`) do not require the runtime bridge.
- File-backed reads operate on the Godot project resolved by `GODOT_PROJECT_ROOT` or, when unset, the server working directory and nearest `project.godot`.
- Runtime-backed reads (`godot.editor.state.get`, `godot.node.tree.get`, `godot.node.properties.get`) require initialized MCP HTTP session state plus a fresh runtime snapshot.
- If the slice requires mutating tools (`godot.project.run`, `godot.project.stop`, `godot.script.modify`, `godot.script.create`, `godot.node.create`, `godot.node.modify`, `godot.node.delete`, `godot.scene.create`, `godot.scene.save`, `godot.scene.apply`), ensure `initialize.params.capabilities.godot.mutating=true` is already negotiated and check `godot.runtime.health.get` first. If the bridge is unhealthy, resolve it before proceeding (see `SAFETY_AND_VERIFICATION.md`).

Inspect:

- The scene that owns the mechanic.
- The node path, signal path, or collision path that carries the behavior.
- The script that updates the mechanic.
- The input action, animation hook, exported declarations, or reusable resource when relevant.

Rules:

- Inspect the existing pattern before adding a new one.
- Do not inspect unrelated scenes or scripts once the owner is clear.

## Step 3: Choose one vertical slice

Prefer slices like:

- One player ability increment.
- One interaction loop.
- One enemy reaction.
- One HUD/data sync path.
- One bug root cause fix.

Avoid:

- Bundling feature work, refactor work, and polish into one change.
- Adding a new subsystem when extending the existing owner is enough.

## Step 4: Implement with Godot conventions

Use these execution defaults unless the project already does something else:

- Keep the change inside one lane and one primary owner.
- Use the lane's policy reference before inventing a new structure.
- Prefer `@export` values for tuning and `@onready` for stable child-node references when the current project style allows them.
- Keep physics-sensitive changes on the physics path and UI-sensitive changes on the UI path.
- When the task needs exact GDScript syntax or engine semantics, route to `OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.

Script modification constraint:

- `godot.script.modify` replaces the entire script content. Always `godot.script.read` first, apply your changes to the full text, then send the complete new content via `godot.script.modify`.

## Step 5: Verify with state readback

Verification relies on reading back the changed state. The AI cannot observe live gameplay directly.

Always verify with:

- A state readback using `godot.script.read`, `godot.scene.read`, `godot.node.tree.get`, or `godot.node.properties.get` depending on what the tool actually exposes. For mutating runtime commands, also use the ack payload returned by the tool.
- A logical gameplay scenario analysis: reason through the expected behavior given the code and scene state, and flag any adjacent risk.
- An owner and callback sanity check: verify the final state still leaves one clear source of truth on the intended timing path.
- Optionally use `godot.project.run` to launch the game for manual user testing, then confirm the result.

Examples:

- Double jump: read the script to confirm jump count logic, reset path, and exported declarations, then flag animation hook if present.
- Collision bug: read the node tree and node metadata to confirm the target node and owner, read scene/script content to inspect collision setup and movement callback, then flag adjacent collision paths.
- HUD update: read the script to confirm signal connection and update path, verify no duplicate connection pattern.

## Step 6: Leave the next slice obvious

Finish by stating:

- What changed.
- What remains.
- The single best next slice.
