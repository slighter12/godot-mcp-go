# Official Godot Docs Map

Use this map when the task needs Godot engine semantics, callback behavior, node expectations, or exact GDScript syntax.

Default to the official `en/stable` docs. When the page offers multiple language tabs or code examples, prefer the GDScript examples unless the task explicitly targets C# or Rust.

## GDScript Syntax and Core Patterns

Use these pages when the task needs exact syntax for declarations, annotations, properties, functions, signals, or common script structure.

- GDScript basics: <https://docs.godotengine.org/en/stable/tutorials/scripting/gdscript/gdscript_basics.html>
  - Use for general language syntax, annotations, functions, properties, signals, and node references.
- GDScript exported properties: <https://docs.godotengine.org/en/stable/tutorials/scripting/gdscript/gdscript_exports.html>
  - Use for `@export` patterns, inspector-facing tuning values, and export hints.

Common routing:

- Use the exported properties page first for `@export`.
- Use the GDScript basics page for `@onready`.
- Use the GDScript basics page for script structure or syntax uncertainty.

## Signals and Scene Communication

Use these pages when the task involves signal declaration, connection, callback patterns, or cross-node communication.

- Using signals: <https://docs.godotengine.org/en/stable/getting_started/step_by_step/signals.html>
  - Use for workflow-level signal setup and connection examples.
- `Signal` class reference: <https://docs.godotengine.org/en/stable/classes/class_signal.html>
  - Use for engine-level signal API behavior and method names.

Common routing:

- Use the signals tutorial first for signal wiring in UI or gameplay.
- Use the `Signal` class reference for exact signal API behavior.

## Movement, Physics, and Collision

Use these pages when the task touches `_process()`, `_physics_process()`, movement ownership, body type selection, overlap detection, or collision behavior.

- Idle and Physics Processing: <https://docs.godotengine.org/en/stable/tutorials/scripting/idle_and_physics_processing.html>
  - Use for `_process()` versus `_physics_process()` semantics.
- Physics introduction: <https://docs.godotengine.org/en/stable/tutorials/physics/physics_introduction.html>
  - Use for collision concepts, physics body roles, and interaction expectations.
- `CharacterBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_characterbody2d.html>
  - Use for player or enemy movement code, grounded motion, and movement helper methods.
- `Area2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_area2d.html>
  - Use for overlap detection, trigger zones, pickups, and detection volumes.
- 2D movement overview: <https://docs.godotengine.org/en/stable/tutorials/2d/2d_movement.html>
  - Use for movement patterns and practical 2D motion examples.

Common routing:

- Use Idle and Physics Processing for movement callback selection.
- Use Physics introduction for collision layers, body roles, or trigger behavior.
- Use `CharacterBody2D` for player or enemy controller logic.
- Use `Area2D` for interaction zones or detection ranges.

## Input Actions and Project Setup

Use these pages when the task depends on input action names, Input Map setup, or project-level configuration behavior.

- Project Settings: <https://docs.godotengine.org/en/stable/tutorials/editor/project_settings.html>
  - Use for where Input Map lives in the editor and how project settings are organized.
- `InputMap` class reference: <https://docs.godotengine.org/en/stable/classes/class_inputmap.html>
  - Use for API-level input action management behavior.

Common routing:

- Use Project Settings when checking whether an input action should exist.
- Use the `InputMap` class reference for exact InputMap API behavior.
