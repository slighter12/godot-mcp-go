# Safety and Verification

## Development Guardrails

- Preserve existing project conventions before introducing new patterns.
- Avoid adding autoloads, singleton managers, or new scene/script ownership layers unless the project already uses them or the task explicitly needs them.
- Prefer `@export` values over hardcoded tuning constants when designers or future tuning will need the value.
- Prefer `@onready` for child node references instead of repeated `get_node()` calls in methods.
- Prefer signals, groups, and explicit node ownership over brittle path lookups spread across multiple scripts.
- `godot.script.modify` is a full content replacement. Always read the script first, never send partial content.
- `godot.node.properties.get` currently exposes runtime metadata only (`path`, `name`, `type`, `owner`, `script`, `groups`, `child_count`), not arbitrary engine properties.
- Keep each iteration to one minimal behavior change and one verification pass.
- When the task needs GDScript syntax or Godot engine semantics, route to `references/OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.

## Godot-Specific Checks

Review these when relevant to the task:

- Input actions exist and map to the expected behavior.
- Physics logic lives in the right callback and does not bypass collision resolution.
- Collision masks, layers, areas, and body types match the intended interaction.
- Signals are connected once and to the correct owner.
- Animation, timers, and state resets still line up with the mechanic.
- UI reads state from the correct source and does not take ownership of gameplay logic.

## Verification Checklist

Use this checklist after each iteration:

1. State readback (`godot.script.read`, `godot.scene.read`, `godot.node.tree.get`, `godot.node.properties.get`) or the mutating ack payload confirms the intended update was applied, using only fields the tool actually exposes.
2. Logical analysis of the changed code confirms the primary gameplay scenario should behave as intended.
3. Logical analysis confirms one adjacent behavior is not broken by the change.
4. The implementation still matches the existing project pattern.
5. One concrete next slice is identified.
6. Optionally, suggest the user run `godot.project.run` for manual gameplay verification.

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
