# Safety and Verification

## Development Guardrails

- Preserve existing project conventions before introducing new patterns.
- Avoid adding autoloads, singleton managers, or new scene/script ownership layers unless the project already uses them or the task explicitly needs them.
- `godot.script.modify` is a full content replacement. Always read the script first, never send partial content.
- `godot.node.properties.get` currently exposes runtime metadata only (`path`, `name`, `type`, `owner`, `script`, `groups`, `child_count`), not arbitrary engine properties.
- Keep each iteration to one minimal behavior change and one verification pass.
- When the task needs GDScript syntax or Godot engine semantics, route to `OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.
- Use `../../policy-godot/SKILL.md` for general Godot conventions and topic routing.

## Godot-Specific Checks

Review these when relevant to the task:

### Controller or physics

- Input actions exist and map to the expected behavior.
- Movement or simulation logic stays on the intended timing path (`../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`).
- Controller state, landing reset, timers, and animation hooks still line up with the mechanic.

### Collision or trigger

- Body type matches the mechanic (`../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`).
- Collision masks, layers, areas, and monitoring settings match the intended interaction (`../../policy-godot/references/GODOT_PHYSICS_AND_COLLISION.md`).
- Trigger detection and gameplay consequence still have one clear owner.

### UI or HUD

- UI reads gameplay state from the correct source and does not take ownership of gameplay logic (`../../policy-godot/references/GODOT_UI_AND_INPUT.md`).
- `Control` interactions use the right input path for the owner.
- HUD updates do not duplicate or cache stale gameplay state.

### Signals and ownership

- Signals are connected once and to the correct owner (`../../policy-godot/references/GODOT_SIGNALS.md`).
- The final implementation still has one clear owner and no unnecessary abstraction (`../../policy-godot/references/GODOT_ENGINEERING_QUALITY.md`).

### Resources and persistence

- Resource loading, shared data, or save-related changes still match the existing project organization (`../../policy-godot/references/GODOT_RESOURCE_MANAGEMENT.md`).
- Reused scenes or pooled objects do not retain stale state unexpectedly.

## Verification Checklist

Use this checklist after each iteration:

1. State readback (`godot.script.read`, `godot.scene.read`, `godot.node.tree.get`, `godot.node.properties.get`) or the mutating ack payload confirms the intended update was applied, using only fields the tool actually exposes.
2. Logical analysis of the changed code confirms the primary gameplay scenario should behave as intended.
3. Logical analysis confirms one adjacent behavior is not broken by the change.
4. Ownership sanity: one clear owner still controls the mechanic or state.
5. Callback sanity: the final behavior still runs on the intended timing path.
6. No unnecessary abstraction was introduced for this slice.
7. One concrete next slice is identified.
8. Optionally, suggest the user run `godot.project.run` for manual gameplay verification.

## MCP Quick Unblock Appendix

### Session lifecycle blocker

Symptoms:

- `session_not_initialized` or regular methods rejected.

Unblock:

1. Ensure `initialize` uses protocol version `2025-11-25`.
2. Send `notifications/initialized` after successful initialize.
3. Retry the blocked gameplay step.

### Transport/runtime blocker

Symptoms:

- runtime command timeout or transport unavailable.

Unblock:

1. Check `godot.runtime.health.get`.
2. Confirm the Godot plugin/runtime is active.
3. Retry a minimal read path before mutating again.

### Mutating capability blocker

Symptoms:

- `mutating_capability_required`
- Mutating tools are rejected even though read tools work

Unblock:

1. Return to `initialize`.
2. Ensure `initialize.params.capabilities.godot.mutating=true`.
3. Re-send `notifications/initialized` if the transport/session was recreated.
4. Retry the blocked mutating step.
