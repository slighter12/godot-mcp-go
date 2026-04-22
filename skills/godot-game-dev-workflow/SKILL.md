---
name: godot-game-dev-workflow
description: "2D-oriented Godot 4 MCP execution workflow for gameplay feature work, bug fixes, scene/script iteration, and safe refactors through this repository's `godot.*` tools."
---

# Godot Game Development Workflow

## When to Use

Use this skill when the request is about:

- Executing 2D-oriented Godot gameplay work through this repository's `godot.*` MCP tools.
- Fixing gameplay bugs in scenes, nodes, signals, physics, input, animation, or HUD/UI flows.
- Iterating on player, enemy, interaction, or HUD behavior in small vertical slices.
- Refactoring Godot scenes and scripts without changing gameplay outcomes.

Use `../policy-godot/SKILL.md` first when the task is mainly about Godot design decisions such as root node choice, collision architecture, UI input ownership, state placement, autoload/event-bus tradeoffs, or broader review guidance. Return to this workflow skill once the design decision is clear.

Do not use this skill for:

- Protocol-first MCP documentation.
- Transport or session-lifecycle troubleshooting except for short unblock steps that immediately return to the gameplay task.
- Repo-wide rewrites that are not driven by the requested game task.
- General Godot architecture invention when the execution question can be solved by following the current project pattern and the policy skill.

## Critical Requirements

- This is a workflow skill for this repository's MCP server usage, not a general Godot policy skill.
- This workflow currently targets common 2D gameplay slices. Use the policy skill for cross-project 3D design questions, then bring the conclusion back here only if the execution still runs through this repo's MCP tools.
- Preserve the project's current scene/script ownership and naming unless the task explicitly requires structural changes.
- Do not introduce new global state, autoloads, or patterns such as state machines unless the project already uses them or the task clearly needs them.
- Keep general Godot guidance short during execution: identify the lane, route to the relevant policy reference, then continue the MCP flow.
- File-backed reads (`godot.scene.list`, `godot.scene.read`, `godot.script.list`, `godot.script.read`, `godot.script.analyze`, `godot.project.settings.get`, `godot.project.resources.list`) do not require the runtime bridge.
- File-backed reads operate on the Godot project resolved by `GODOT_PROJECT_ROOT` or, when unset, the server working directory and nearest `project.godot`. If the server is running outside the target project tree, set `GODOT_PROJECT_ROOT` first.
- Treat `godot.offerings.list` as a coarse global health signal only. It can tell you whether some editor/runtime path is alive, but not whether the current task's target session is the one that is available.
- Editor-backed reads (`godot.editor.state.get`) require an initialized MCP HTTP session plus a fresh editor snapshot.
- Runtime-backed reads (`godot.runtime.scene_tree.get`, `godot.runtime.node_properties.get`) require an active game `session_id`. Resolve it cautiously: pass an explicit `editor_session_id` to `godot.runtime.session.get_active`, then fail closed unless the returned `editor_session_id` still matches the intended editor owner. Use `godot.runtime.await_snapshot` only after that task-scoped session check passes.
- Mutating tools require an initialized MCP HTTP session, `initialize.params.capabilities.godot.mutating=true`, and a healthy runtime bridge. Check `godot.runtime.health.get` before mutating.
- `godot.script.modify` replaces the entire script content. Always read the current script with `godot.script.read` first, apply changes to the full text, then send the complete new content.
- Never finish a task with an unclear verification story. Every slice needs one gameplay scenario and one readback check.
- When exact Godot API behavior matters, route to `references/OFFICIAL_DOCS_MAP.md` and prefer the official GDScript examples.
- Route design questions to `../policy-godot/SKILL.md`, then return to this workflow for task intake, ownership discovery, tool selection, mutation, and verification.

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

- Start with `godot.offerings.list` only as a coarse signal that some live lane may be available.
- Use file-backed reads first when they are enough to identify ownership.
- Use `godot.editor.state.get` only when live editor state is required; it is editor-backed and needs an initialized MCP HTTP session plus a fresh editor snapshot.
- When editor ownership matters, pass `editor_session_id` explicitly to editor-owner tools instead of trusting global freshness alone.
- Use `godot.project.is_running` with the intended `editor_session_id` as a lifecycle pre-check before `godot.project.run`, `godot.project.stop`, or attach/recover decisions when the current runtime state is uncertain.
- For runtime-backed reads or runtime verification, call `godot.runtime.session.get_active` with an explicit `editor_session_id`.
- Fail closed if the returned `editor_session_id` does not match the intended editor owner; do not continue with runtime reads or runtime inputs on that `session_id`.
- If runtime freshness matters after the session check passes, call `godot.runtime.await_snapshot` before `godot.runtime.scene_tree.get` or `godot.runtime.node_properties.get`.
- Pass the verified `session_id` explicitly to every runtime-backed tool.
- If the slice requires mutating tools, ensure `initialize.params.capabilities.godot.mutating=true` is already negotiated, then check `godot.runtime.health.get`.
- Find the scene, node, script, signal, input action, collision setup, or resource that currently owns the behavior.
- Inspect only the paths that can actually change the target outcome.

3. Choose one vertical slice.

- Implement the smallest end-to-end change that produces visible progress.
- Prefer extending the existing owner over creating new abstractions.

4. Apply Godot-native changes.

- Route to the relevant `policy-godot` reference before changing ownership or conventions.
- Keep the mutation bounded to one lane and one owner.
- Prefer the smallest scene/script/node change that satisfies the acceptance check.

5. Verify with state readback and logical analysis.

- Re-read the changed state via read tools to confirm the update was applied.
- Use `godot.runtime.log.get` as the first runtime diagnostics stream when verification or bootstrap behavior looks wrong.
- Use `godot.runtime.screenshot.get` as an optional manual verification aid when a visual outcome matters.
- Use `godot.runtime.input.tap`, `godot.runtime.input.press`, or `godot.runtime.input.release` only for playable verification paths that genuinely need runtime interaction.
- Reason through one gameplay scenario, one adjacent regression path, and the owner/callback sanity of the final state.
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
