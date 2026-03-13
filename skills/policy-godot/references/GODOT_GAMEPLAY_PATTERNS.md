# Godot Gameplay Patterns

## When to Use

Use this reference when the task is about gameplay systems, AI reactions, animation-driven presentation, save/load boundaries, object pooling, interaction flows, or syncing UI with gameplay state.

## Default Answer

- Keep continuous input, movement ownership, and controller state in one clear gameplay owner.
- Extend an existing controller before adding a parallel input or movement system.
- Treat ability extensions such as dash, double jump, aim modes, or context interactions as additions to an existing ownership boundary unless the project already uses a more modular controller architecture.
- Separate raw input reading from downstream presentation when animation or UI only reacts to controller state.
- Keep patrol, chase, attack, flee, recovery, or fallback transitions attached to one explicit owner.
- Reuse the current state representation before introducing a state machine pattern the project does not already use.
- Prefer explicit transition conditions and fallback behavior over implicit timing or animation-side state ownership.
- Use signals, areas, or detection helpers to feed state decisions, but keep transition ownership in the AI/controller owner.
- Treat animation as presentation by default.
- Let gameplay state drive animation unless the design explicitly requires animation events to gate or release gameplay state.
- When animation events affect gameplay timing, keep the ownership explicit so the gameplay owner still controls the transition.
- Avoid burying core gameplay rules inside animation-player callbacks if that makes the mechanic harder to reason about.
- Keep transient scene state local unless it must persist between scene loads or play sessions.
- Use resource-backed or structured save data for progression, configuration, unlocked content, inventory-like state, or authored data that must survive scene reloads.
- Keep global progression data separate from scene-local runtime state.
- Define whether load restores world state, player progression, or both before choosing where the source of truth lives.
- Use direct instantiate/free for simple or low-frequency spawning.
- Use areas, prompts, or interaction controllers to detect opportunity; keep the actual response owned by the mechanic or system that changes state.
- Keep trigger detection separate from the gameplay consequence when different systems own them.
- For quest-like or multi-step flows, decide early whether the source of truth lives in scene-local progression, shared progression data, or a global coordination layer.
- Avoid letting prompt UI own gameplay outcomes.
- Gameplay owns state; UI owns presentation.
- Push or expose stable state changes from gameplay systems rather than letting UI poll unrelated nodes broadly.
- Use signals or explicit data adapters when UI must react to changing gameplay state across ownership boundaries.
- Keep UI formatting, animation, and display concerns on the UI side even when the triggering state comes from gameplay.

## Escalate Only When Needed

- Introduce a state machine only when the current transition surface is no longer readable as explicit conditions on one owner.
- Introduce pooling only when spawn/despawn churn or allocation cost is measurable or likely to affect gameplay responsiveness.
- Keep pooling ownership explicit: one system should own acquisition, reset, and release rules.
- Do not let pooled objects retain stale state, signal wiring, or references across reuse cycles.
- Use quest-like coordination layers only when scene-local progression can no longer own the flow clearly.

## Decision Guide

- Ask where the source of truth for the mechanic lives.
- Ask whether animation, UI, AI, or save data is reacting to gameplay or trying to own it.
- Ask whether the feature needs local scene state, reusable data, or persistent progression.
- Ask whether performance pressure is real enough to justify a pooling or coordination layer.

## Common Pitfalls

- Duplicated state between gameplay owners, UI, and animation systems.
- Animation callbacks becoming the hidden owner of gameplay transitions.
- Premature pooling that adds lifecycle bugs without measurable benefit.
- Save systems that over-centralize unrelated runtime state.
- AI transitions that are split across detection helpers, animation, and controller code with no single owner.

## Review Checks

- Does one gameplay owner clearly control the main state transition?
- Is animation reacting to gameplay unless the mechanic explicitly requires animation-gated logic?
- Is pooling justified by real churn instead of being introduced by default?
- Is saved data scoped to what actually needs to persist?
- Does UI presentation stay separate from gameplay ownership?

## Cross-Topic Routing

- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when the gameplay problem is really an ownership or communication problem.
- Pair with `GODOT_LIFECYCLE.md` when movement, input, or timing hooks are central to the pattern.
- Pair with `GODOT_UI_AND_INPUT.md` when gameplay state must coordinate with HUD or `Control` input surfaces.
- Pair with `GODOT_PHYSICS_AND_COLLISION.md` when controller, hitbox, or trigger behavior is driving the pattern.
- Pair with `GODOT_RESOURCE_MANAGEMENT.md` when persistence, pooled objects, or shared gameplay data depend on resources.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when a pattern choice needs performance, debugging, or regression scrutiny.
