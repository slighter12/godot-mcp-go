---
name: policy-godot
description: "Decision-first Godot 4 policy guidance for scene ownership, physics, UI/input, signals, resources, gameplay patterns, and Rust/gdext. Use this for cross-project 2D and early 3D design and review decisions, independent of any repository-specific MCP workflow."
---

# Godot Policy Guide

## When to Use

Use this skill when the request is about:

- Choosing how a Godot 4 feature should be designed before deciding how to execute it.
- Reviewing or designing scene ownership, root node choice, signals, physics behavior, UI input ownership, or resource boundaries.
- Applying cross-project Godot guidance for 2D, early 3D architectural questions, UI, shared data, or Rust/gdext integrations.
- Evaluating tradeoffs such as local owner vs autoload, direct call vs signal, direct instancing vs pooling, or simple transitions vs state machine.
- Debugging or reviewing architecture, callback timing, collision setup, UI/gameplay boundaries, or refactor risk.
- Planning animation integration, AI transitions, save/load boundaries, progression state, spawn-heavy systems, or native code boundaries.

## Do Not Use This Skill When

- The task is mainly about this repository's `godot.*` MCP tool sequencing, runtime bridge behavior, or tool prerequisites.
- The task is mainly about transport setup, session lifecycle, mutating capability negotiation, or unblock steps for MCP execution.
- A repository-specific workflow skill already defines the execution flow and only needs a design reference.

## Default Stack

Start from these defaults unless the project already uses a better-fitting pattern:

- Keep the smallest ownership boundary that can clearly own the behavior or data.
- Choose the most specialized root node that matches the scene's job.
- Prefer the project's current language boundary. If a system already lives in GDScript or Rust, extend that side before introducing a new mixed-language split.
- Prefer stable references, signals, and clear owners over brittle cross-tree lookups.
- Keep gameplay as the source of truth and UI as presentation.
- Prefer local state before scene-global state, and scene-global state before autoload/global services.
- Prefer direct instancing before pooling.
- Extend the current project pattern before introducing a new abstraction.

## Escalation Patterns

Use these only after explaining why the default stack is insufficient:

- State machine
  - Use when: transition count, fallback rules, or state-specific behavior are already hard to reason about in the current representation.
  - Avoid when: one owner with a few explicit branches is still readable.
  - Review risks: duplicated state, hidden transitions, and animation becoming the real owner.
- Autoload service
  - Use when: progression, account/session state, settings, or a truly global service must outlive scenes.
  - Avoid when: the feature is local to one scene tree or one gameplay loop.
  - Review risks: hidden coupling, convenience-driven globals, and state ownership drift.
- Event bus
  - Use when: multiple unrelated systems need the same global event surface and local signal wiring is no longer proportional.
  - Avoid when: one scene or one small hierarchy already owns the interaction.
  - Review risks: turning events into hidden commands or a shadow state container.
- Resource-backed data
  - Use when: the same authored data shape must be reused, edited, or persisted across scenes or systems.
  - Avoid when: the data is purely transient and local to one owner.
  - Review risks: accidental shared mutable state and unclear runtime vs authored ownership.
- Component extraction
  - Use when: the behavior has its own lifecycle, tuning surface, or reuse boundary across multiple owners.
  - Avoid when: the extraction exists only to shorten one script.
  - Review risks: wrapper components without real ownership and signal sprawl.
- Pooling
  - Use when: spawn/despawn churn is measurable or likely to affect responsiveness.
  - Avoid when: direct instantiate/free is still simple and fast enough.
  - Review risks: stale state, stale connections, and hard-to-debug reuse bugs.
- Rust / GDExtension
  - Use when: the project already has a native boundary, or the feature is performance-sensitive, native-integration-heavy, or strongly typed enough to justify it.
  - Avoid when: clearer scene, script, or resource ownership would solve the problem.
  - Review risks: noisy cross-language APIs, debugging cost, and ownership mistakes.

## Problem-First Routing

- Root node choice, scene ownership, reusable content, or autoload boundaries: `references/GODOT_SCENE_STRUCTURE.md`
- Collision setup, body type choice, movement ownership, or layers/masks debugging: `references/GODOT_PHYSICS_AND_COLLISION.md`
- UI interaction, HUD ownership, input callback choice, focus, or `CanvasLayer` boundaries: `references/GODOT_UI_AND_INPUT.md`
- Tree timing, initialization order, `_ready()`, `_enter_tree()`, `_exit_tree()`, or cleanup: `references/GODOT_LIFECYCLE.md`
- Signal design, direct call vs signal, groups, or event-bus questions: `references/GODOT_SIGNALS.md` and `references/GODOT_ARCHITECTURE_PATTERNS.md`
- AI transitions, animation ownership, save/load, progression state, or pooling questions: `references/GODOT_GAMEPLAY_PATTERNS.md`
- Review risk, regression analysis, performance concerns, or refactor safety: `references/GODOT_ENGINEERING_QUALITY.md`
- GDScript structure, typing, naming, or script-level maintainability: `references/GODOT_GDSCRIPT.md`
- Rust/gdext API surface or ownership boundaries: `references/GODOT_RUST.md`

Use the smallest relevant set of references for the current decision.

## How To Answer

- Start with the default answer that fits the current ownership boundary.
- If the default is not enough, explain which escalation pattern is justified and why.
- If the question depends on exact engine semantics or API names, pair the answer with the official Godot documentation.
- Do not recommend state machines, autoloads, event buses, pooling, or Rust boundaries without first stating why the simpler option is insufficient.

## References

- `references/GODOT_GDSCRIPT.md`
- `references/GODOT_PHYSICS_AND_COLLISION.md`
- `references/GODOT_UI_AND_INPUT.md`
- `references/GODOT_LIFECYCLE.md`
- `references/GODOT_SCENE_STRUCTURE.md`
- `references/GODOT_SIGNALS.md`
- `references/GODOT_RESOURCE_MANAGEMENT.md`
- `references/GODOT_RUST.md`
- `references/GODOT_ARCHITECTURE_PATTERNS.md`
- `references/GODOT_GAMEPLAY_PATTERNS.md`
- `references/GODOT_ENGINEERING_QUALITY.md`
