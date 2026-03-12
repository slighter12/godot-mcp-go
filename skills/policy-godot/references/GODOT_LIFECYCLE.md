# Godot Lifecycle Guidance

Use this reference when the task depends on callback timing, initialization order, cleanup behavior, or tree-entry sequencing.

## Tree Entry And Initialization

- Put initialization that depends on child nodes or the scene tree in `_ready()` or later.
- Use `_enter_tree()` when the behavior truly needs to react before `_ready()`, such as registration or parent-tree coordination.
- Keep initialization order in mind when parents depend on children or when child setup depends on parent-provided state.

## Callback Timing Defaults

- Keep one mechanic on one primary timing path when possible.
- Use `_ready()` for scene-tree-dependent setup and initial signal wiring.
- Use `_process(delta)` for presentation-side updates, interpolation, and non-physics visuals.
- If a callback change would alter gameplay correctness or collision behavior, route to `GODOT_PHYSICS_AND_COLLISION.md`.
- If a callback change is really about UI event handling or focus, route to `GODOT_UI_AND_INPUT.md`.

## Cleanup And Lifetime

- Prefer `queue_free()` over `free()` for node destruction.
- Clean up long-lived manual connections, timers, or deferred work in `_exit_tree()` when the task introduces that lifetime risk.
- Check that deferred or async follow-up work still targets a valid node before using it.
- Keep teardown rules proportional to actual lifetime risk. Do not add cleanup ceremony where the engine already owns the lifecycle safely.

## Cross-Topic Routing

- Pair with `GODOT_GDSCRIPT.md` when lifecycle changes also reshape script structure.
- Pair with `GODOT_SIGNALS.md` when connection timing or teardown is part of the design.
- Pair with `GODOT_SCENE_STRUCTURE.md` when lifecycle confusion comes from unclear ownership.
- Pair with `GODOT_PHYSICS_AND_COLLISION.md` when update-hook choice affects movement, collision, or body behavior.
- Pair with `GODOT_UI_AND_INPUT.md` when the callback decision is really about UI or input ownership.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when timing bugs or unnecessary hooks are creating regression or performance risk.

## Review Checks

- Is scene-tree-dependent setup happening in `_ready()` or later unless there is a real `_enter_tree()` need?
- Does the chosen callback keep one clear timing path for the mechanic?
- If cleanup was added, is it limited to the connections or deferred work that actually need it?
