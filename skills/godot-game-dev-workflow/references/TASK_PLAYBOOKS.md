# Task Playbooks

Use these playbooks to keep work scoped to one gameplay slice and one clear ownership boundary.

## Player Ability Or Movement

Example:

- Add a player double jump.

Flow:

1. Inspect the player scene root, movement script, exported tuning, and relevant input action.
2. Identify the existing owner of jump state, velocity updates, and landing reset.
3. Extend the current owner instead of adding a parallel movement system.
4. Verify with one player scenario and one adjacent regression check such as wall jump, ledge contact, or animation timing.
5. Leave one next slice such as animation polish or VFX, not both.

## Collision Or Interaction Bug

Example:

- Character passes through wall.

Flow:

1. Inspect collision nodes, masks/layers, body type, and the movement callback that advances the actor.
2. Check whether the issue is data setup, node wiring, or movement logic before editing.
3. Fix one root cause only.
4. Verify the exact repro path plus one nearby interaction such as slopes, triggers, or pickups.
5. Report one follow-up hardening step if risk remains.

## Scene Composition Or Content Setup

Example:
- Add a new interactable object.

Flow:
1. Inspect the target scene hierarchy and an existing comparable object.
2. Reuse the current composition pattern before adding custom wiring.
3. Add the minimal node tree, collision/input setup, and script hook.
4. Verify placement, trigger path, and feedback to the player.
5. Report the next polish or content-authoring step.

## UI Or HUD Sync

Example:
- Update health HUD after damage.

Flow:
1. Inspect the UI scene, the gameplay data source, and current signal or callback flow.
2. Keep data ownership in gameplay code and presentation ownership in UI code.
3. Add or fix one update path only.
4. Verify the gameplay event, the visual update, and absence of duplicate or stale updates.
5. Report the next UI refinement step.

## Enemy Behavior Or State Transition

Example:
- Enemy should switch from patrol to chase when the player enters range.

Flow:
1. Inspect the enemy scene, the state owner, and the detection path such as signal, group, or area overlap.
2. Reuse the current state representation instead of introducing a new one.
3. Implement one transition or reaction at a time.
4. Verify entry, exit, and fallback behavior.
5. Report the next transition or tuning slice.

## Refactor Without Gameplay Change

Example:
- Split a monolithic enemy behavior function.

Flow:
1. Inspect call sites, node dependencies, and timing assumptions.
2. Extract one coherent function boundary while preserving current inputs and outputs.
3. Do not mix refactor work with new behavior.
4. Verify the previous gameplay scenario still behaves the same.
5. Report the next safe extraction candidate.
