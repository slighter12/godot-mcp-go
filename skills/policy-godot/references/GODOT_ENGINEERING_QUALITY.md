# Godot Engineering Quality

## When to Use

Use this reference when reviewing Godot code or scenes for performance, debuggability, verification quality, regression risk, or refactor safety.

## Debugging And Review Defaults

- Start by identifying the source-of-truth owner for the broken behavior.
- Confirm the callback timing: `_process()`, `_physics_process()`, `_ready()`, `_exit_tree()`, animation events, signal callbacks, or UI/input handlers.
- Inspect the scene tree and communication boundaries before assuming a code-only bug.
- Trace signal flow, resource state, and persistent/shared data when multiple systems react to the same event.
- Narrow the issue to one owner, one timing boundary, or one communication path before redesigning the feature.

## Reviewer Checklists

- Prefer clear ownership over clever indirection.
- Question any change that introduces new globals, hidden coupling, or duplicate state.
- Check ownership clarity: can one reviewer point to one main owner for the mechanic?
- Check callback correctness: is the behavior running on the intended timing path?
- Check duplicate state risk: is UI, animation, or a helper system shadowing gameplay state?
- Check signal storm risk: are many nodes reacting to the same event without a clear need?
- Check pooling reset risk: can reused objects retain stale state or connections?
- Check scene-tree churn risk: is the change rebuilding or reparenting more than needed?
- Check whether a new abstraction has an actual reuse or lifecycle boundary.
- Check whether editor-facing values are exported when tuning is likely.
- Review whether communication patterns match the real coupling boundary.

## Performance Pitfalls

- Repeated loading or scene instancing in hot paths such as combat, frequent UI refreshes, or spawn-heavy loops.
- Signal storms where many emitters or listeners update the same state every frame or every minor event.
- Unnecessary `_process()` or `_physics_process()` hooks on nodes that could react more locally or less often.
- Scene-tree churn from frequent add/remove/rebuild cycles that should be reused, deferred, or scoped differently.
- Over-instancing or premature pooling that complicates state reset without clear benefit.
- Excessive GDScript-to-Rust or Rust-to-GDScript chatter in hot paths.

## Verification And Regression Checks

- Validate the primary scenario that motivated the change.
- Validate one adjacent behavior that is likely to regress because it shares timing, ownership, or communication paths.
- Confirm ownership sanity: one clear source of truth, no hidden duplicate state.
- Confirm timing sanity: the behavior runs in the intended callback or event flow.
- Confirm persistence or resource sanity when shared data, pooling, or save/load state is involved.

## 2D / 3D / UI Quality Checks

- 2D: movement timing, collision ownership, node hierarchy clarity, and animation sync all match the intended mechanic.
- 3D: world-space ownership, transform flow, detection areas, and camera or control boundaries remain clear.
- UI: gameplay state remains external to the UI, signal/update flow is not duplicated, and presentation remains local to the `Control` tree.

## Refactor Safety

- Do not mix behavior changes into a refactor unless the task explicitly allows it.
- Do not change ownership boundaries casually while claiming behavior is unchanged.
- Keep one coherent extraction or simplification boundary per pass.
- Verify that renamed or moved logic still fires at the same time and under the same owner.

## Decision Guide

- Ask whether the problem is correctness, ownership clarity, timing, or performance.
- Ask whether a simpler local fix would solve it before introducing a new system.
- Ask what the most likely adjacent regression path is before approving the change.
- Prefer the smallest correction that improves clarity and verification confidence.

## Common Pitfalls

- Chasing symptoms in presentation code when the real issue is gameplay ownership.
- Fixing a timing bug by adding more callbacks instead of choosing the correct owner and hook.
- Accepting a refactor that silently changes behavior or state boundaries.
- Adding optimization patterns before the current bottleneck is understood.
- Reviewing only the happy path and missing adjacent regressions.

## Review Checks

- Is the main owner of the behavior still obvious after the change?
- Is the timing path still the intended one?
- Did the change introduce any new hidden global or duplicated state?
- Is there a concrete adjacent regression scenario worth checking?
- Is the complexity added by the change justified by the problem it solves?

## Cross-Topic Routing

- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when review risk comes from unclear ownership or communication choices.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when the quality question depends on controller, AI, animation, save/load, or pooling design.
- Pair with `GODOT_PHYSICS_AND_COLLISION.md` when review risk depends on body type, movement helper, or trigger correctness.
- Pair with `GODOT_UI_AND_INPUT.md` when review risk depends on UI ownership, input flow, or `Control` behavior.
- Pair with `GODOT_LIFECYCLE.md` when timing and callback selection are the likely source of failure.
- Pair with `GODOT_RESOURCE_MANAGEMENT.md` when performance or regression risk involves loading, instancing, or shared resources.
