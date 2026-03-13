# Official Godot Docs Map

Use this map when the task needs exact Godot engine semantics, class behavior, callback behavior, node expectations, or exact GDScript syntax. This file is an official-docs lookup table, not a replacement for policy decisions.

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

## Scene Structure and Node Types

Use these pages when the task depends on root node selection, scene composition, UI ownership, or reusable node patterns.

- Nodes and scene instances: <https://docs.godotengine.org/en/stable/getting_started/step_by_step/nodes_and_scene_instances.html>
  - Use for scene composition basics, node ownership, and instancing mental models.
- `Node` class reference: <https://docs.godotengine.org/en/stable/classes/class_node.html>
  - Use for tree ownership, groups, lifecycle-adjacent node behavior, scene-tree APIs, and non-visual coordinators or managers.
- `Node2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_node2d.html>
  - Use for pure 2D transform/content holders when no more specialized body or area root is needed.
- `Node3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_node3d.html>
  - Use for pure 3D transform/content holders when no more specialized body or area root is needed.
- `CharacterBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_characterbody2d.html>
  - Use for 2D controller actors and grounded movement helpers.
- `CharacterBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_characterbody3d.html>
  - Use for 3D controller actors and grounded movement helpers.
- `RigidBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_rigidbody2d.html>
  - Use for 2D physics-driven props or reactive bodies.
- `RigidBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_rigidbody3d.html>
  - Use for 3D physics-driven props or reactive bodies.
- `AnimatableBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_animatablebody2d.html>
  - Use for 2D moving platforms, doors, conveyors, or other collision owners moved explicitly by code or animation.
- `AnimatableBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_animatablebody3d.html>
  - Use for 3D moving platforms, doors, conveyors, or other collision owners moved explicitly by code or animation.
- `StaticBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_staticbody2d.html>
  - Use for 2D static blockers or level collision roots.
- `StaticBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_staticbody3d.html>
  - Use for 3D static blockers or level collision roots.
- `Control` class reference: <https://docs.godotengine.org/en/stable/classes/class_control.html>
  - Use for UI scene roots, layout behavior, and input handling on interface nodes.
- `CanvasLayer` class reference: <https://docs.godotengine.org/en/stable/classes/class_canvaslayer.html>
  - Use for screen-space separation and overlays that should not inherit world transforms, not as a blanket replacement for `Control` content roots.

Common routing:

- Use Nodes and scene instances first for ownership and reusable scene composition questions.
- Use the most specialized class reference that matches the scene job, not only its rendering dimension.
- Use `AnimatableBody2D` or `AnimatableBody3D` for moving collision owners before falling back to `StaticBody*` or generic transform nodes.

## Movement, Physics, and Collision

Use these pages when the task touches `_process()`, `_physics_process()`, movement ownership, body type selection, overlap detection, or collision behavior.

- Idle and Physics Processing: <https://docs.godotengine.org/en/stable/tutorials/scripting/idle_and_physics_processing.html>
  - Use for `_process()` versus `_physics_process()` semantics.
- Physics introduction: <https://docs.godotengine.org/en/stable/tutorials/physics/physics_introduction.html>
  - Use for collision concepts, physics body roles, and interaction expectations.
- `CharacterBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_characterbody2d.html>
  - Use for player or enemy movement code, grounded motion, and movement helper methods.
- `CharacterBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_characterbody3d.html>
  - Use for 3D controller movement and grounded motion behavior.
- `RigidBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_rigidbody2d.html>
  - Use for 2D rigid-body behavior and physics-driven objects.
- `RigidBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_rigidbody3d.html>
  - Use for 3D rigid-body behavior and physics-driven objects.
- `AnimatableBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_animatablebody2d.html>
  - Use for 2D moving collision owners such as platforms or doors moved by script or animation.
- `AnimatableBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_animatablebody3d.html>
  - Use for 3D moving collision owners such as platforms or doors moved by script or animation.
- `StaticBody2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_staticbody2d.html>
  - Use for 2D collision blockers and static interaction surfaces.
- `StaticBody3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_staticbody3d.html>
  - Use for 3D collision blockers and static interaction surfaces.
- `Area2D` class reference: <https://docs.godotengine.org/en/stable/classes/class_area2d.html>
  - Use for overlap detection, trigger zones, pickups, and detection volumes.
- `Area3D` class reference: <https://docs.godotengine.org/en/stable/classes/class_area3d.html>
  - Use for 3D overlap detection, trigger volumes, and pickups.
- 2D movement overview: <https://docs.godotengine.org/en/stable/tutorials/2d/2d_movement.html>
  - Use for movement patterns and practical 2D motion examples.

Common routing:

- Use Idle and Physics Processing for movement callback selection.
- Use Physics introduction for collision layers, body roles, or trigger behavior.
- Use `CharacterBody2D` for player or enemy controller logic.
- Use `RigidBody2D` or `RigidBody3D` when direct body control, forces, impulses, or `_integrate_forces()` behavior matters.
- Use `AnimatableBody2D` or `AnimatableBody3D` for moving collision owners that are not controller actors.
- Use `Area2D` for interaction zones or detection ranges.
- The workflow playbooks in this repository still focus on common 2D recipes even though these docs also include 3D class references.

## UI, Input, and Screen-Space Layers

Use these pages when the task depends on `Control` input handling, `_gui_input()`, focus, HUD ownership, or screen-space UI layout.

- `Control` class reference: <https://docs.godotengine.org/en/stable/classes/class_control.html>
  - Use for `_gui_input()`, focus, built-in UI signals, layout, and interaction behavior.
- `CanvasLayer` class reference: <https://docs.godotengine.org/en/stable/classes/class_canvaslayer.html>
  - Use for screen-space separation when HUD or overlay content should not inherit world transforms.
- GUI navigation tutorial: <https://docs.godotengine.org/en/stable/tutorials/ui/gui_navigation.html>
  - Use for focus behavior and controller/keyboard navigation.

Common routing:

- Use `Control` first for UI event handling and `_gui_input()` behavior.
- Use `Control` first for HUD content ownership, layout, and widget behavior.
- Use `CanvasLayer` when the question is about screen-space separation from world-space scenes or when a `Control` subtree needs a separate screen-space layer.
- Use GUI navigation when focus behavior is part of the problem.

## Input Actions and Project Setup

Use these pages when the task depends on input action names, Input Map setup, or project-level configuration behavior.

- Project Settings: <https://docs.godotengine.org/en/stable/tutorials/editor/project_settings.html>
  - Use for where Input Map lives in the editor and how project settings are organized.
- `InputMap` class reference: <https://docs.godotengine.org/en/stable/classes/class_inputmap.html>
  - Use for API-level input action management behavior.

Common routing:

- Use Project Settings when checking whether an input action should exist.
- Use the `InputMap` class reference for exact InputMap API behavior.

## Resources, Packed Scenes, and Loading

Use these pages when the task depends on `preload()`, `load()`, `PackedScene`, `Resource`, or asynchronous loading behavior. When the project is implemented in Rust / GDExtension, pair this section with `Godot Rust / GDExtension` below and keep the resource boundary aligned with the existing project split.

- `PackedScene` class reference: <https://docs.godotengine.org/en/stable/classes/class_packedscene.html>
  - Use for scene instancing behavior and saved scene data expectations.
- `Resource` class reference: <https://docs.godotengine.org/en/stable/classes/class_resource.html>
  - Use for shared data assets, inspector-backed resources, and reference-counted data objects.
- `ResourceLoader` class reference: <https://docs.godotengine.org/en/stable/classes/class_resourceloader.html>
  - Use for dynamic and threaded loading behavior.
- Resources tutorial: <https://docs.godotengine.org/en/stable/tutorials/scripting/resources.html>
  - Use for the broader mental model around reusable resources and custom resource types.

Common routing:

- Use `PackedScene` when deciding whether a reusable entity should be instanced as a scene.
- Use `Resource` and the resources tutorial for shared data assets.
- Use `ResourceLoader` when the task needs exact threaded or runtime loading behavior.

## Godot Rust / GDExtension

Use these pages when the task depends on engine-side extension concepts or Rust integration boundaries.

- GDExtension overview: <https://docs.godotengine.org/en/stable/tutorials/scripting/gdextension/what_is_gdextension.html>
  - Use for engine-side extension architecture and when native code integration is appropriate.
- GDExtension C example: <https://docs.godotengine.org/en/stable/tutorials/scripting/gdextension/gdextension_c_example.html>
  - Use for the Godot-side registration model when language-specific rust docs are not enough.
- godot-rust book: <https://godot-rust.github.io/book/intro/index.html>
  - Use for Rust-side workflows, class registration patterns, ownership expectations, and cross-language integration guidance.
- `godot` crate docs: <https://docs.rs/godot/latest/godot/>
  - Use for exact Rust API names, macro usage, supported types, and version-specific surface details.

Common routing:

- Use the GDExtension overview first for engine integration questions.
- Use the godot-rust book first when the task is implemented in Rust rather than only discussing Godot-side concepts.
- Use the `godot` crate docs for exact Rust API behavior or macro/type signatures.
- Use the C example only to confirm Godot-side concepts such as registration flow or extension boundaries when the Rust docs do not cover the engine-side concept clearly.
