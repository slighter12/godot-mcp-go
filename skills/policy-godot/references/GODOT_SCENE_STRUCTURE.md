# Godot Scene Structure Guidance

Use this reference when the task changes scene ownership, node hierarchy, reusable content, UI composition, or project organization.

## Ownership And Scene Boundaries

- Keep each scene focused on one logical unit such as a character, enemy, interactable, UI panel, reusable widget, level chunk, or world controller.
- Put the main behavior on the scene owner that already drives the mechanic unless there is a clear reason to split it out.
- Reuse existing composition patterns before creating a new wrapper scene or ownership layer.

## Root Node Decision Matrix

- Prefer the most specialized root node that matches the scene's job.
- Non-visual coordinator, state owner, or scene-tree-only manager: `Node`
- Controller actor: `CharacterBody2D` or `CharacterBody3D`
- Physics-driven prop or reactive body: `RigidBody2D` or `RigidBody3D`
- Moving platform, door, conveyor, or other body moved explicitly by code or animation while still affecting collisions: `AnimatableBody2D` or `AnimatableBody3D`
- Static blocker or level collision owner: `StaticBody2D` or `StaticBody3D`
- Trigger, pickup, or detection zone: `Area2D` or `Area3D`
- Pure transform/content holder: `Node2D` or `Node3D`
- UI panel, widget, or reusable HUD content that needs layout, focus, or theme behavior: `Control`
- Screen-space separation layer or overlay anchor that should not inherit world or camera transforms: `CanvasLayer`

## UI Root Examples

- Pause menu, inventory panel, settings dialog, or reusable widget: prefer `Control`
- Reusable HUD content: prefer `Control` ownership unless screen-space separation is part of the scene's actual responsibility
- Fixed on-screen HUD integrated at the game or UI composition layer: often a `Control` subtree under a `CanvasLayer`
- Minimal overlay anchor with little or no `Control` layout behavior: `CanvasLayer` can own the scene directly

## Avoid

- Do not treat HUD as an automatic reason to choose `CanvasLayer` as the content root.
- Do not trade away `Control` layout, focus, or theme behavior just to force screen-space separation into the scene root.
- Do not couple reusable UI content to `CanvasLayer` unless that separation is part of the intended ownership boundary.

## Node Hierarchy And Reuse

- Avoid unnecessarily deep scene trees. Split a subtree into a reusable subscene when depth or mixed responsibilities make the scene harder to maintain.
- Use descriptive node names that reflect role instead of leaving default generated names in place.
- Prefer groups for stable category-style tagging, not for transient state management.

## Reuse And Global Patterns

- Prefer scene instancing for reusable gameplay objects, 3D props, interactables, and UI widgets instead of copy-pasting node trees.
- Only add autoloads for true global services already justified by the project architecture. Do not introduce them as a default coordination pattern.
- Keep shared scenes, resources, and scripts in the project's existing structure instead of forcing a new folder convention midstream.

## Cross-Topic Routing

- Pair with `GODOT_RESOURCE_MANAGEMENT.md` when scene changes depend on loaded assets or shared resources.
- Pair with `GODOT_PHYSICS_AND_COLLISION.md` when the scene root or subtree choice depends on body type, trigger behavior, or movement ownership.
- Pair with `GODOT_UI_AND_INPUT.md` when the scene boundary crosses gameplay and UI ownership.
- Pair with `GODOT_SIGNALS.md` when the change crosses scene boundaries through signal wiring.
- Pair with `GODOT_GDSCRIPT.md` when script ownership is unclear because the scene boundary is unclear.
- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when the real issue is ownership, component boundaries, or global coordination.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when scene shape is being driven by controllers, AI, interaction, or UI sync behavior.

## Review Checks

- Does this scene already have a clear owner for the behavior being changed?
- Does the root node match the actual job of the scene instead of only its rendering dimension?
- Is the node tree still readable without opening multiple unrelated scripts?
- Would a reusable subscene remove duplication without introducing a new global abstraction?
