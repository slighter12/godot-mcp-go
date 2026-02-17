# Prompt Catalog Completeness Plan (v1)

## Scope Ownership

This document owns prompt catalog runtime details only.

- In scope:
  - `prompt_catalog` config contract
  - Prompt discovery and registration model
  - `prompts/list` and `prompts/get` response contracts
  - Prompt catalog specific error semantics and readiness checks
  - Prompt catalog related policy metadata exposure
- Out of scope:
  - General product roadmap and release sequencing
  - Non-prompt Godot feature roadmap
  - Packaging and distribution plans

Cross-project priorities and milestone status are tracked in `docs/DEVELOPMENT.md`.

## Current Implementation Boundary

### Runtime inputs

- Config key: `prompt_catalog.enabled`, `prompt_catalog.paths`
- Environment overrides:
  - `MCP_PROMPT_CATALOG_ENABLED`
  - `MCP_PROMPT_CATALOG_PATHS`
- Source format: `SKILL.md` files discovered from configured paths

### Runtime outputs

- `prompts/list` exposes registered prompt names and descriptions.
- `prompts/get` resolves one prompt and renders template placeholders from `arguments`.
- `resources/list` and `resources/read` expose `godot://policy/godot-checks` for machine-readable policy metadata.

## Prompt Catalog Runtime Contract

## 1. Discovery Contract

1. Resolve all configured `prompt_catalog.paths`.
2. Recursively scan directories for `SKILL.md`.
3. Parse frontmatter (`name`, `description`) and body content.
4. Register a prompt record:
   - `name`
   - `description`
   - `template` (body)
   - `source_path`

## 2. Endpoint Contract

### `prompts/list`

- Input:
  - optional `cursor`
- Output:
  - `prompts`: array of `{name, description}`
  - optional `nextCursor`
- Error semantics:
  - `kind=not_supported` when prompt catalog disabled
  - `kind=not_available` when catalog loading failed

### `prompts/get`

- Input:
  - `name` (required)
  - `arguments` (optional map)
- Output:
  - `name`
  - `description`
  - `messages` with rendered prompt text
- Rendering rule:
  - replace `{{key}}` with `arguments[key]` string value
- Error semantics:
  - `kind=invalid_params` for missing or unknown prompt name
  - `kind=not_supported` / `kind=not_available` same as `prompts/list`

## 3. Error Semantic Contract

| kind | JSON-RPC code | Meaning |
| --- | --- | --- |
| `not_supported` | `-32601` | Prompt catalog feature intentionally disabled or unavailable in deployment |
| `not_available` | `-32000` | Feature enabled but runtime dependency/data not currently ready |
| `invalid_params` | `-32602` | Request payload invalid for prompt catalog contract |
| `execution_failed` | `-32000` | Runtime execution path failed after acceptance |

All prompt catalog errors should include `error.data.kind`.

## 4. Policy Metadata Contract

- Resource URI: `godot://policy/godot-checks`
- Payload:
  - `policy`: `"policy-godot"`
  - `checks`: array of `{id, level, appliesTo, description, stopAndAsk}`

This resource is metadata only. It is not the execution engine for policy enforcement.

## Outstanding Gaps (Prompt Catalog Scope)

- Runtime hot-reload lifecycle is manual (`reload-prompt-catalog` tool); file-watch auto-reload is not defined.
- Template rendering currently supports simple placeholder replacement only.
- Prompt catalog path governance and allow-list policy are not defined.

## Delivery Phases

### Phase 1: Config and Discovery Baseline

- Prompt catalog config keys and env overrides
- Runtime registry initialization
- File discovery and prompt registration

### Phase 2: Prompt Endpoint Correctness

- `prompts/list` with pagination
- `prompts/get` with argument rendering
- Structured semantic errors (`kind`)

### Phase 3: Determinism and Compatibility

- Collision policy for duplicate prompt names
- Deterministic lookup for case-insensitive names
- Capability reporting aligned to enabled/disabled state
- Prompt metadata enrichment (`title`, template-derived `arguments`)
- Prompt capability flag `prompts.listChanged` (transport-specific)

### Phase 4: Verification

- Unit tests for registry and prompt handlers
- HTTP smoke checks with prompt catalog enabled/disabled variants
- Inspector compatibility checks for prompt methods
- Streamable HTTP SSE checks for `notifications/prompts/list_changed`

## Current Runtime Notes (2025-11-25)

- Streamable HTTP integration targets MCP protocol version `2025-11-25`.
- Clients should send `MCP-Protocol-Version: 2025-11-25` on Streamable HTTP requests; missing header is treated as invalid when no negotiated session version exists.
- Streamable HTTP supports optional SSE channel through `GET /mcp` for server-to-client notifications.
- Prompt catalog reload is exposed via `tools/call` using `reload-prompt-catalog`.
- `reload-prompt-catalog` emits `notifications/prompts/list_changed` only when prompt list visible metadata changed.
- stdio transport does not emit prompt list changed notifications in this phase (`prompts.listChanged=false`).

## Manual Verification Checklist

- [ ] With `prompt_catalog.enabled=true`, `prompts/list` returns registered entries.
- [ ] With `prompt_catalog.enabled=true`, `prompts/get` renders placeholders from `arguments`.
- [ ] With `prompt_catalog.enabled=false`, prompt methods return `kind=not_supported`.
- [ ] Unknown prompt name returns `kind=invalid_params`.
- [ ] `resources/read` for `godot://policy/godot-checks` returns check metadata.
