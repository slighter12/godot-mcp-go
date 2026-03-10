---
name: godot-game-dev-workflow
description: "Godot 4 game development workflow for feature work, bug fixes, scene/script iteration, and gameplay systems. Inspect the current project shape, preserve existing conventions, implement one small vertical slice, verify behavior, and continue with the next slice."
---

# Godot Game Development Workflow

## When to Use

Use this skill when the request is about:

- Building or extending gameplay systems in Godot 4.
- Fixing gameplay bugs in scenes, nodes, signals, physics, input, animation, or UI.
- Iterating on player, enemy, interaction, or HUD behavior in small slices.
- Refactoring Godot scenes and scripts without changing gameplay outcomes.

Do not use this skill for:

- Protocol-first MCP documentation.
- Repo-wide rewrites that are not driven by the requested game task.
- Inventing new architecture when the existing project pattern is sufficient.

## Critical Requirements

- Preserve the project's current scene/script ownership and naming unless the task explicitly requires structural changes.
- Do not introduce new global state, autoloads, or patterns such as state machines unless the project already uses them or the task clearly needs them.
- Prefer Godot-native conventions: signals for decoupling, `@export` for tunable values, `@onready` for child node references, scenes for composition, scripts for behavior, and resources for reusable data.
- Use `_physics_process()` for movement and physics behavior, `_process()` for visual updates, and keep collision/input/animation changes scoped to the mechanic being changed.
- File-backed reads (`godot.scene.list`, `godot.scene.read`, `godot.script.list`, `godot.script.read`, `godot.script.analyze`, `godot.project.settings.get`, `godot.project.resources.list`) do not require the runtime bridge.
- File-backed reads operate on the Godot project resolved by `GODOT_PROJECT_ROOT` or, when unset, the server working directory and nearest `project.godot`. If the server is running outside the target project tree, set `GODOT_PROJECT_ROOT` first.
- Runtime-backed reads (`godot.editor.state.get`, `godot.node.tree.get`, `godot.node.properties.get`) require an initialized MCP HTTP session plus a fresh runtime snapshot.
- Mutating tools require an initialized MCP HTTP session, `initialize.params.capabilities.godot.mutating=true`, and a healthy runtime bridge. Check `godot.runtime.health.get` before mutating.
- `godot.script.modify` replaces the entire script content. Always read the current script with `godot.script.read` first, apply changes to the full text, then send the complete new content.
- Never finish a task with an unclear verification story. Every slice needs one gameplay scenario and one readback check.
- When the task needs GDScript syntax or Godot engine semantics, route to `references/OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.

## Required Response Contract
Structure responses in this format:

- `Goal`
- `Acceptance Check`
- `Relevant State`
- `Implementation Slice`
- `Godot Conventions Applied`
- `Verification`
- `Next Slice`

## Workflow

1. Establish the gameplay outcome.

- Restate the requested behavior in player-facing terms.
- Define one acceptance check before editing anything.

2. Confirm runtime bridge and inspect the real ownership boundary.

- Use file-backed reads first when they are enough to identify ownership.
- Use runtime-backed reads only when live editor/runtime state is required; these need an initialized MCP HTTP session plus a fresh runtime snapshot.
- If the slice requires mutating tools, ensure `initialize.params.capabilities.godot.mutating=true` is already negotiated, then check `godot.runtime.health.get`.
- Find the scene, node, script, signal, input action, or resource that currently owns the behavior.
- Inspect only the paths that can actually change the target outcome.

3. Choose one vertical slice.

- Implement the smallest end-to-end change that produces visible progress.
- Prefer extending the existing owner over creating new abstractions.

4. Apply Godot-native changes.

- Keep scene composition, node setup, script logic, and exported tuning values aligned.
- Use signals, groups, collision layers, and resources intentionally instead of ad hoc cross-scene references.

5. Verify with state readback and logical analysis.

- Re-read the changed state via read tools to confirm the update was applied.
- Reason through one gameplay scenario and one adjacent regression path based on the code.
- Optionally suggest `godot.project.run` for manual user testing.

6. Continue with one next slice.

- Leave the project in a state where the next improvement is obvious.

## Unblock Rule

If execution is blocked by MCP lifecycle, capability negotiation, or transport/runtime issues, use the short unblock appendix in `references/SAFETY_AND_VERIFICATION.md`, then return to the game task immediately.

## References

- `references/CORE_LOOP.md`
- `references/TASK_PLAYBOOKS.md`
- `references/TOOL_RECIPES.md`
- `references/SAFETY_AND_VERIFICATION.md`
- `references/OFFICIAL_DOCS_MAP.md`
