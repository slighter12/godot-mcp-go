# Godot Rust Guidance

Use this reference when the task targets Rust-based Godot code, GDExtension registration, native integration boundaries, or GDScript-to-Rust coordination.

## Default Answer

- Prefer the Godot 4 `gdext` stack and keep versions aligned with the engine version used by the project.
- Use `#[derive(GodotClass)]` with an explicit base class that matches the node role.
- Export methods and properties only when they need to be visible to Godot or GDScript.
- Keep the exported API small and stable enough that scene scripts are not forced to understand Rust internals.
- Use `Gd<T>` and temporary borrows instead of holding raw long-lived references to Godot objects.
- Let Godot own node lifetimes and use engine-friendly destruction patterns.
- Avoid panics in exported functions; convert failures into logged errors or structured results the engine can tolerate.
- Keep ownership short-lived and explicit around callbacks, signals, and scene lookups.

## Escalate Only When Needed

- Keep Rust focused on performance-sensitive, native-integration-heavy, or strongly typed subsystems when that boundary already exists in the project.
- Avoid excessive back-and-forth calls between GDScript and Rust for hot-path behavior.
- Reuse the project's current module layout and registration pattern before introducing a new native boundary.
- Do not move code to Rust by default if the problem is better solved by clearer scene or script ownership.

## Cross-Topic Routing

- Pair with `GODOT_SCENE_STRUCTURE.md` when the Rust class ownership in the scene tree is unclear.
- Pair with `GODOT_GDSCRIPT.md` when the GDScript-facing API shape is becoming noisy or inconsistent.
- Pair with `GODOT_RESOURCE_MANAGEMENT.md` when Rust code owns or transforms reusable data assets.
- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when native boundaries are being used to compensate for unclear ownership.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when performance, review risk, or cross-language debugging cost is the main concern.

## Review Checks

- Does the exported API use Godot-supported types and a clear ownership boundary?
- Is object access bounded to short borrows instead of stored raw references?
- Is Rust actually the right place for this logic given the current project split?
