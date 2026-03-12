# Godot Signals Guidance

Use this reference when the task changes signal definitions, connection points, callback ownership, or cross-scene communication.

## Default Answer

- Use `snake_case` names that describe the event clearly.
- Prefer built-in signals when the engine already exposes the needed event.
- Include enough payload for the receiver to react without re-querying unrelated state.
- Keep signal ownership with the node or object that naturally emits the event.
- Keep signal wiring explicit and easy to find, usually in `_ready()` or in the setup path that owns the relationship.
- Use signals for decoupled communication across siblings, reusable child components, UI/gameplay boundaries, or scene transitions.
- Use direct calls when the relationship is tight and already owned locally.

## Escalate Only When Needed

- Use groups when the problem is category-style discovery or bulk interaction rather than point-to-point signaling.
- Treat global event buses as an explicit architectural choice, not the default answer to cross-scene communication.
- If the signal graph becomes hard to reason about, revisit scene ownership before adding a broader communication layer.

## Cleanup And Safety

- If the task introduces long-lived manual connections across owners, make the cleanup point explicit.
- Do not assume every signal needs manual disconnection; add cleanup where lifetime, reuse, or teardown ordering makes it necessary.
- Be careful when emitting signals that can trigger scene-tree mutation during physics updates or teardown paths.

## Cross-Topic Routing

- Pair with `GODOT_LIFECYCLE.md` when connection timing or `_exit_tree()` cleanup is part of the issue.
- Pair with `GODOT_UI_AND_INPUT.md` when the signal question is really about UI ownership or `Control` interactions.
- Pair with `GODOT_SCENE_STRUCTURE.md` when signal sprawl points to an unclear ownership boundary.
- Pair with `GODOT_GDSCRIPT.md` when signal-related API shape or naming needs script-level cleanup.
- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when deciding between direct calls, signals, groups, or event buses.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when signal usage is causing debugging, review, or performance concerns.

## Review Checks

- Is the signal connected once and in a predictable place?
- Is the receiver the correct owner for the resulting behavior?
- Would a local method call be simpler because the coupling is already explicit and stable?
