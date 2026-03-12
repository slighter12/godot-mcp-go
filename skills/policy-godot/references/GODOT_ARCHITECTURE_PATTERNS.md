# Godot Architecture Patterns

## When to Use

Use this reference when the task is mainly about ownership boundaries, state placement, cross-scene communication, or deciding whether a pattern belongs in a local owner, reusable component, resource, or global service.

## Default Answer

- Prefer a clear owner for each mechanic, data flow, or UI reaction path before introducing helper systems.
- Keep scene-local behavior inside the scene owner unless reuse or complexity clearly justifies a separate component.
- Treat script, scene, resource, and autoload boundaries as architectural decisions, not just file-placement decisions.
- Avoid splitting one behavior across multiple owners unless each side has a clear responsibility.
- Use local state when the data only matters to one node or one immediate mechanic.
- Use scene-owned state when several child nodes depend on the same owner and that owner already orchestrates the behavior.
- Use resource-backed state when the same structured data needs to be edited, reused, or shared across scenes or systems.
- Use global state only for true cross-session or cross-scene concerns such as progression, account/session state, global settings, or intentionally global services.
- Do not centralize state preemptively. Pick the smallest owner that matches the actual sharing boundary.
- Use direct calls when the relationship is tight, local, and owned explicitly.
- Use signals when the emitter should not own the receiver or when reusable components need loose coupling.
- Use groups when the problem is category-style discovery or bulk interaction rather than state synchronization.
- Prefer stable ownership and explicit references over brittle tree-path lookups spread across unrelated scripts.

## Escalate Only When Needed

- Autoloads are appropriate for true global services, shared coordination points, or intentionally global game/session state.
- Use an autoload event bus only when events are truly global and multiple systems need a stable decoupled broadcast surface.
- An event bus should carry events, not become a hidden state container or command router for unrelated systems.
- If a feature only touches one scene or a small local hierarchy, solve it locally before considering an autoload.
- Revisit autoload additions when they are compensating for unclear ownership elsewhere in the scene tree.
- Extract a reusable child component when the behavior has its own lifecycle, editor-facing tuning, or reuse across multiple owners.
- Keep logic on the root owner when the behavior is tightly coupled to the owner's movement, animation, health, UI contract, or state transitions.
- Prefer composition over inheritance when multiple scene types need a similar capability with different owners.
- Do not create a component solely to reduce line count if it makes ownership harder to understand.

## 2D / 3D / UI Boundary Notes

- In 2D and 3D scenes, keep world interaction ownership with gameplay nodes, not presentation helpers.
- In UI scenes, keep presentation ownership in `Control` trees and pull gameplay state from a stable gameplay owner or shared data source.
- When a mechanic crosses 2D/3D world space and UI space, define which side owns the source of truth before wiring updates.

## Decision Guide

- Ask who owns the source of truth for this behavior or data.
- Ask who needs to observe it and whether those observers should be coupled directly.
- Ask whether the problem is local reuse, cross-scene reuse, or intentionally global coordination.
- Choose the smallest pattern that solves the current sharing boundary without obscuring ownership.

## Common Pitfalls

- Ownership ambiguity where multiple nodes appear to own the same state or transition.
- Signal sprawl that exists only because the actual owner is unclear.
- Global singleton or event bus overuse for scene-local problems.
- Cross-scene brittle lookups that bypass scene ownership and make refactors dangerous.
- Components that exist as wrappers without a real lifecycle or reuse boundary.

## Review Checks

- Can a reviewer point to one clear owner for the mechanic or state?
- Is the chosen communication pattern proportional to the actual coupling boundary?
- Would moving this logic back to the owner or into a reusable component make the design clearer?
- Is any autoload justified by a truly global concern rather than convenience?

## Cross-Topic Routing

- Pair with `GODOT_SCENE_STRUCTURE.md` when the scene tree itself is causing the ownership confusion.
- Pair with `GODOT_SIGNALS.md` when communication choices are the main design question.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when the architecture decision is driven by controller, AI, UI sync, or save/load behavior.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when the question is whether the design is creating debugging, review, or performance risk.
