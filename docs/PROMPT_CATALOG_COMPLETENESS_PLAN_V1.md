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

## Cross-Doc Alignment Note (2026-02-18)

- This update line includes Godot runtime bridge/tooling work only.
- Delivered non-prompt runtime changes include:
  - runtime bridge read path (`sync-editor-runtime` + `ping-editor-runtime`)
  - runtime command bridge for `run-project` / `stop-project`
  - runtime log summarization for large bridge payloads
- Prompt catalog runtime contract, payloads, and semantic rules remain unchanged.
- Track Godot integration progress in `docs/DEVELOPMENT.md` to avoid duplicating non-prompt scope here.

## v1 Contract Freeze (2026-02-17)

This section is the frozen runtime contract for prompt catalog behavior in this repo line.

### Runtime Inputs

- Config keys:
  - `prompt_catalog.enabled`
  - `prompt_catalog.paths`
  - `prompt_catalog.allowed_roots`
  - `prompt_catalog.auto_reload.enabled`
  - `prompt_catalog.auto_reload.interval_seconds`
  - `prompt_catalog.rendering.mode`
  - `prompt_catalog.rendering.reject_unknown_arguments`
- Environment overrides:
  - `MCP_PROMPT_CATALOG_ENABLED`
  - `MCP_PROMPT_CATALOG_PATHS`
  - `MCP_PROMPT_CATALOG_ALLOWED_ROOTS`
  - `MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED`
  - `MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS`
  - `MCP_PROMPT_CATALOG_RENDERING_MODE`
  - `MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS`
- Source format: `SKILL.md` files discovered from configured paths.

### Discovery Contract

1. Resolve all configured `prompt_catalog.paths`.
2. Recursively scan directories for `SKILL.md`.
3. Canonicalize candidate file paths (`filepath.Clean` + `EvalSymlinks` when available).
4. Enforce allow roots:
   - Use `prompt_catalog.allowed_roots` when non-empty.
   - Fallback to `prompt_catalog.paths` when `allowed_roots` is empty.
   - Files outside effective roots are skipped and recorded as load warnings.
5. Parse frontmatter (`name`, `description`, optional `title`) and body content.
6. Register prompt records with deterministic case-insensitive keying:
   - `name`
   - `title` (optional)
   - `description`
   - `arguments` (template-derived placeholders)
   - `template` (body)
   - `source_path`

### Endpoint Contract

#### `prompts/list`

- Input:
  - optional `cursor`
- Output:
  - `prompts`: array of `{name, description, title?, arguments?}`
  - optional `nextCursor`
- Error semantics:
  - `kind=not_supported` when prompt catalog disabled
  - `kind=not_available` when catalog loading failed and no prompts are available

#### `prompts/get`

- Input:
  - `name` (required)
  - `arguments` (optional map of string values)
- Output:
  - `name`
  - `description`
  - `messages` with rendered prompt text
- Rendering modes:
  - `legacy` (default): preserves historical behavior
  - `strict`: validates required placeholders before rendering
    - missing required argument -> `kind=invalid_params`
    - unknown arguments rejected only when `reject_unknown_arguments=true`
- Error semantics:
  - `kind=invalid_params` for payload/type/name/strict-validation issues
  - `kind=not_supported` / `kind=not_available` same as `prompts/list`

### Error Semantic Contract

| kind | JSON-RPC code | Meaning |
| --- | --- | --- |
| `not_supported` | `-32601` | Prompt catalog feature intentionally disabled or unavailable in deployment |
| `not_available` | `-32000` | Feature enabled but runtime dependency/data not currently ready |
| `invalid_params` | `-32602` | Request payload invalid for prompt catalog contract |
| `execution_failed` | `-32000` | Runtime execution path failed after acceptance |

All prompt catalog errors include `error.data.kind`.

### Policy Metadata Contract

- Resource URI: `godot://policy/godot-checks`
- Payload:
  - `policy`: `"policy-godot"`
  - `checks`: array of `{id, level, appliesTo, description, stopAndAsk}`

This resource is metadata only. It is not the execution engine for policy enforcement.

### Runtime Notifications and Capabilities

- Streamable HTTP integration targets protocol version `2025-11-25`.
- Clients should send `MCP-Protocol-Version: 2025-11-25` on Streamable HTTP requests.
- Prompt catalog reload is exposed via `tools/call` using `reload-prompt-catalog`.
- `reload-prompt-catalog` and auto-reload share the same reload pipeline.
- `notifications/prompts/list_changed` is emitted only when visible prompt list metadata changes.
- stdio transport does not emit prompt list changed notifications in this phase (`prompts.listChanged=false`).

## v1.1 Delivered Items

- Prompt catalog path governance with effective allow roots (`allowed_roots` fallback to `paths`).
- Polling-based auto-reload (`auto_reload.*`) using source fingerprint (path + size + mtime + content SHA-256).
- Optional strict prompt rendering validation (`rendering.mode=strict`).

## Deferred Backlog

- File-watch based auto-reload (event-driven) is not defined; polling is the current implementation.
- Rich templating features (conditionals/loops/custom functions) are out of scope; placeholder substitution is still the rendering model.
- Additional governance controls (for example per-root policy tiers) are out of scope.

## Verification Matrix

| Contract Item | Coverage | Evidence |
| --- | --- | --- |
| Prompt discovery and registration | Automated | `promptcatalog/registry_test.go` |
| Case-insensitive uniqueness and lookup determinism | Automated | `promptcatalog/registry_test.go` |
| Path governance and symlink escape rejection | Automated | `promptcatalog/registry_test.go` |
| `prompts/list` pagination and metadata output | Automated | `transport/shared/helpers_prompt_catalog_test.go` |
| `prompts/get` argument rendering and payload validation | Automated | `transport/shared/helpers_prompt_catalog_test.go` |
| Strict rendering mode validation | Automated | `transport/shared/helpers_prompt_catalog_test.go`, `transport/http/prompt_catalog_integration_test.go`, `transport/stdio/prompt_catalog_integration_test.go` |
| Disabled prompt catalog returns `not_supported` | Automated | `transport/shared/helpers_prompt_catalog_test.go`, `transport/http/prompt_catalog_integration_test.go`, `transport/stdio/prompt_catalog_integration_test.go` |
| Load-failure readiness returns `not_available` | Automated | `transport/shared/helpers_prompt_catalog_test.go`, `transport/http/prompt_catalog_runtime_test.go` |
| Prompt list changed notification semantics | Automated | `transport/http/prompt_catalog_integration_test.go`, `transport/http/prompt_catalog_runtime_test.go` |
| Auto-reload source-change trigger behavior | Automated | `transport/http/prompt_catalog_runtime_test.go` |
| Policy metadata resource contract | Manual gap noted | `transport/shared/helpers.go`, `transport/stdio/server.go` (add explicit endpoint tests if stricter coverage is required) |

## Manual Verification Checklist

- [ ] With `prompt_catalog.enabled=true`, `prompts/list` returns registered entries.
  - Automated Coverage: Yes
- [ ] With `prompt_catalog.enabled=true`, `prompts/get` renders placeholders from `arguments`.
  - Automated Coverage: Yes
- [ ] With `prompt_catalog.enabled=false`, prompt methods return `kind=not_supported`.
  - Automated Coverage: Yes
- [ ] Unknown prompt name returns `kind=invalid_params`.
  - Automated Coverage: Yes
- [ ] `resources/read` for `godot://policy/godot-checks` returns check metadata.
  - Automated Coverage: Partial (manual recommended)
- [ ] With `rendering.mode=strict`, missing required arguments return `kind=invalid_params`.
  - Automated Coverage: Yes
- [ ] With `auto_reload.enabled=true`, adding/modifying/deleting `SKILL.md` triggers reload only when source fingerprint changes.
  - Automated Coverage: Yes
