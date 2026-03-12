# Godot UI And Input Guidance

Use this reference when the task depends on gameplay input vs UI input ownership, `Control` interactions, HUD boundaries, focus behavior, or `CanvasLayer` decisions.

## Default Answer

- Keep gameplay state in gameplay owners and keep UI focused on presentation.
- Decide the owner first, then choose the input hook that matches that owner's timing path.
- Use polling or buffered intent on the controller's main timing path for continuous or physics-sensitive gameplay actions.
- For `Control` interactions, prefer `_gui_input()` or built-in UI signals over `_input()`.
- Use `_unhandled_input()` only when UI should get first chance to consume the event and the remaining behavior is a discrete gameplay or menu action.
- Use `_input()` only for intentional top-level interception or truly global input ownership.
- Use `Control` for UI content ownership, and use `CanvasLayer` only when screen-space separation from world-space scenes is actually required.
- Push stable state changes from gameplay to UI instead of letting UI poll unrelated gameplay nodes broadly.

## Hook Selection Matrix

- Jump, dash, attack, aiming, held movement, or other controller-owned gameplay actions: sample or buffer on the controller timing path, usually alongside `_physics_process()` or the controller update loop
- Pause, confirm, back, or other discrete actions after UI has had first pass: `_unhandled_input()` when that ownership is intentional
- Button clicks, menu focus, hover, and widget-local interactions: `_gui_input()` or built-in `Control` signals
- Global debug shortcuts or app-level interception above both gameplay and UI: `_input()` only when that higher-level ownership is explicit

## Focus And Ownership

- Make focus and selection part of the UI ownership model, not a side effect of gameplay polling.
- Keep HUD updates reactive to gameplay state changes rather than making HUD code own gameplay transitions.
- If a prompt or menu can trigger gameplay outcomes, keep the gameplay consequence owned by the gameplay system that changes state.

## Escalate Only When Needed

- Add a UI adapter or presenter layer only when direct signal wiring between gameplay and UI is no longer easy to reason about.
- Introduce a wider input coordination layer only when multiple owners truly compete for the same input surface.

## Avoid

- Avoid reading input in `_unhandled_input()` while keeping the real state transition in `_physics_process()` if that splits ownership across timing paths.
- Avoid moving gameplay rules into `Control` callbacks when the UI should only trigger or present the result.
- Avoid using `_input()` as the default answer when a `Control` path or controller-owned timing path is clearer.

## Cross-Topic Routing

- Pair with `GODOT_SIGNALS.md` when UI updates depend on signal design or connection ownership.
- Pair with `GODOT_LIFECYCLE.md` when tree timing or teardown affects UI setup.
- Pair with `GODOT_SCENE_STRUCTURE.md` when the real question is `Control` vs `CanvasLayer` vs world-space scene ownership.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when HUD state, prompts, or animation timing depend on gameplay ownership.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when duplicated state, stale updates, or mixed ownership are creating review risk.

## Review Checks

- Does the chosen input hook match the owner of the interaction?
- For gameplay actions, does the final state transition still stay on the intended controller timing path?
- Is UI consuming UI events through `Control` patterns before gameplay-level interception?
- Does the HUD read gameplay state without taking ownership of it?
- Is `CanvasLayer` used only when screen-space separation is actually required?
