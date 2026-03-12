# Godot Physics And Collision Guidance

Use this reference when the task depends on body type selection, callback ownership, rigid-body steering, movement helpers, collision layers and masks, trigger behavior, or collision debugging.

## Default Answer

- Keep physics ownership with the gameplay node that actually moves or reacts to collisions.
- Use `_physics_process(delta)` for `CharacterBody*`, manual controller movement, and other collision-sensitive logic that you own directly.
- For `RigidBody2D` or `RigidBody3D`, prefer forces, impulses, or `_integrate_forces()` when directly steering body state.
- Choose the body type that matches the job:
  - `CharacterBody2D` or `CharacterBody3D` for controller-style actors
  - `RigidBody2D` or `RigidBody3D` for physics-driven props and reactive objects
  - `AnimatableBody2D` or `AnimatableBody3D` for moving platforms, doors, conveyors, or other scripted/animated moving collision owners
  - `StaticBody2D` or `StaticBody3D` for non-moving blockers or level collision owners
  - `Area2D` or `Area3D` for triggers, pickups, hurtboxes, hitboxes, and detection zones
- Prefer the engine's movement helpers and body semantics over manual transform updates that bypass collision resolution.
- Keep hitbox, hurtbox, and detection ownership explicit. The zone detects; the gameplay owner decides the state change.

## Layers, Masks, And Monitoring

- Treat layers as what the object is and masks as what the object listens to.
- Verify both sides of a collision or trigger interaction before assuming the bug is in movement code.
- For `Area2D` or `Area3D`, confirm monitoring and monitorable settings match the intended interaction.
- Do not use groups as a substitute for collision filtering when physics layers and masks are the real mechanic boundary.

## Escalate Only When Needed

- Add secondary collision helpers only when the primary owner cannot express the needed detection clearly.
- Split hitbox, hurtbox, or detection nodes into reusable subscenes only when the project reuses them across multiple owners.
- Use custom resource-backed collision data only when authored data truly needs to be reused or tuned outside one scene.

## Collision Bug Triage Order

Check in this order before redesigning the feature:

1. Wrong node type for the mechanic
2. Wrong callback or timing path
3. Wrong movement helper or transform update path
4. Wrong layers, masks, or monitoring settings
5. Disabled shape or inactive detection setup
6. Wrong owner, signal target, or gameplay reaction path

## Cross-Topic Routing

- Pair with `GODOT_SCENE_STRUCTURE.md` when body type choice is really a root-node or ownership decision.
- Pair with `GODOT_LIFECYCLE.md` when callback timing or cleanup is the likely failure point.
- Pair with `GODOT_UI_AND_INPUT.md` when collision events feed prompts or HUD reactions.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when controller, AI, hitbox, or trigger behavior is the real gameplay design question.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when the issue is review risk, debugging flow, or performance rather than raw setup.

## Review Checks

- Does the chosen body type match the mechanic?
- Is movement or collision logic running on the intended physics path?
- For `RigidBody*`, is direct body steering using the rigid-body path rather than controller-style movement code?
- Do layers, masks, and monitoring settings match the interaction contract?
- Is the gameplay owner still the source of truth after the collision or trigger fires?
