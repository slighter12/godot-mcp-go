# Godot Resource Management Guidance

Use this reference when the task touches asset loading, scene instancing, shared resources, memory pressure, or reusable data assets.

## Default Answer

- Prefer the loading API and language boundary already used by the project.
- In GDScript, use `preload()` for stable compile-time resources that are reused often.
- In GDScript, use `load()` when the path is dynamic or chosen at runtime.
- In Rust/gdext or mixed-language projects, keep resource loading and ownership on the side that already owns the mechanic or data unless there is a clear reason to move the boundary.
- Avoid repeated resource loading inside `_process()` or `_physics_process()`.
- Instance reusable scenes through `PackedScene.instantiate()`.
- Reuse existing assets and shared resource files before duplicating content for one feature slice.
- Cache frequently reused resources when repeated lookup would make the logic noisy or expensive.
- Use `Resource`-based data assets when multiple systems need the same editor-authored data shape.
- Keep shared resources in the project's existing organization rather than forcing a new top-level folder scheme.

## Escalate Only When Needed

- Let Godot's reference counting release unused resources, and clear references when a large asset should no longer stay resident.
- Consider threaded or deferred loading only when the asset size or runtime timing actually justifies the complexity.
- Introduce pooling or long-lived caches only when the churn or load cost is real enough to justify the lifecycle complexity.

## Lifetime And Performance

- Use `queue_free()` for nodes that should leave the scene tree.
- Be careful with hot-path instancing or loading in combat, streaming, UI refresh, or spawn-heavy systems.
- Keep the cross-language surface small when resources are shared between GDScript and Rust.
- Be explicit about whether a resource is authored shared data, runtime-local data, or persisted progression data.

## Cross-Topic Routing

- Pair with `GODOT_SCENE_STRUCTURE.md` when resource changes also affect ownership or reusable scene composition.
- Pair with `GODOT_PHYSICS_AND_COLLISION.md` when the resource decision is attached to hitbox, collision, or body setup data.
- Pair with `GODOT_GDSCRIPT.md` when resource access patterns are making the script harder to reason about.
- Pair with `GODOT_RUST.md` when native code owns or transforms resource-backed data.
- Pair with `GODOT_GAMEPLAY_PATTERNS.md` when loading, pooling, or persistence decisions are driven by gameplay-system needs.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when resource usage is creating performance or regression risk.

## Review Checks

- Is the chosen loading pattern consistent with how often the asset is used?
- Did this change introduce a repeated load in a hot path?
- If a shared resource was added, does it fit the project's existing organization and ownership model?
