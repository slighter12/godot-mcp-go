# Godot GDScript Guidance

Use this reference when the task depends on script structure, language conventions, or maintainability in GDScript.

## Default Answer

- Keep one script focused on one owner or one responsibility boundary.
- Prefer type hints for new or modified variables, parameters, and return values when the surrounding code already uses typed GDScript or the type meaning is not obvious.
- Use `PascalCase` for class names, `snake_case` for variables and functions, `UPPER_SNAKE_CASE` for constants, and `_leading_underscore` for private members.
- Use `@export` for inspector-facing values that are likely to be tuned.
- Use `@onready` for stable child-node references instead of repeated `get_node()` calls.
- Prefer named constants or exported values over unexplained literals.

## Naming And Organization Checks

- Keep file, class, and scene naming aligned when that improves discoverability.
- Group related exports, cached node references, signals, and helpers in a predictable order.
- Use `class_name` when the script benefits from a reusable global type name. Do not force it onto every file.
- Use `match` when it clearly improves readability for multi-branch logic; do not force it where a small conditional reads better.

## Escalate Only When Needed

- Split helpers or helper scripts when the current function or script is hard to reason about, not just because it is long.
- Extract reusable script-level abstractions only when multiple owners truly share the same behavior or data contract.
- Introduce `class_name` or a reusable helper type only when that improves discoverability or reuse enough to justify the wider API surface.
- Check node or object validity before dereferencing when the lifetime is not obvious.

## Cross-Topic Routing

- Pair with `GODOT_LIFECYCLE.md` when callback timing or cleanup changes are part of the script decision.
- Pair with `GODOT_SCENE_STRUCTURE.md` when the script boundary is unclear because the scene owner is unclear.
- Pair with `GODOT_SIGNALS.md` when the script defines or connects signals.
- Pair with `GODOT_ARCHITECTURE_PATTERNS.md` when the script shape is really an ownership or state-placement choice.
- Pair with `GODOT_ENGINEERING_QUALITY.md` when readability, review risk, or refactor safety is the main concern.

## Review Checks

- Does this script own a single clear behavior or data boundary?
- Are new inspector-facing values exported instead of buried as ad hoc literals?
- Are naming and typing choices consistent with the surrounding project?
