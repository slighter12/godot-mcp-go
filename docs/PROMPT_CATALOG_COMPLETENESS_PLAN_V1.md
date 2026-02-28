# Prompt Catalog Completeness Plan (v1)

## Scope Ownership

This document owns prompt catalog runtime contracts only.

- In scope:
  - `prompt_catalog` config contract
  - Prompt discovery and registration behavior
  - `prompts/list` and `prompts/get` runtime contracts
  - Rendering modes (`legacy`, `strict`, `advanced`)
  - Prompt governance and watch mode behavior
  - Prompt catalog notifications and error semantics
- Out of scope:
  - Non-prompt Godot runtime roadmap
  - Packaging and release sequencing

## Runtime Inputs

- Config keys:
  - `prompt_catalog.enabled`
  - `prompt_catalog.paths`
  - `prompt_catalog.allowed_roots`
  - `prompt_catalog.watch.mode` (`poll` or `event`)
  - `prompt_catalog.governance.roots[]` (`path`, `tier`)
  - `prompt_catalog.auto_reload.enabled`
  - `prompt_catalog.auto_reload.interval_seconds`
  - `prompt_catalog.rendering.mode` (`legacy`, `strict`, `advanced`)
  - `prompt_catalog.rendering.reject_unknown_arguments`
- Environment overrides:
  - `MCP_PROMPT_CATALOG_ENABLED`
  - `MCP_PROMPT_CATALOG_PATHS`
  - `MCP_PROMPT_CATALOG_ALLOWED_ROOTS`
  - `MCP_PROMPT_CATALOG_WATCH_MODE`
  - `MCP_PROMPT_CATALOG_GOVERNANCE_ROOTS`
  - `MCP_PROMPT_CATALOG_AUTO_RELOAD_ENABLED`
  - `MCP_PROMPT_CATALOG_AUTO_RELOAD_INTERVAL_SECONDS`
  - `MCP_PROMPT_CATALOG_RENDERING_MODE`
  - `MCP_PROMPT_CATALOG_REJECT_UNKNOWN_ARGUMENTS`

## Discovery Contract

1. Resolve configured prompt paths.
2. Discover `SKILL.md` recursively.
3. Canonicalize paths and apply allowed-root policy.
4. Parse frontmatter + body.
5. Register deterministic case-insensitive prompt names.
6. Build metadata: `name`, `title`, `description`, `arguments`, `template`, `source_path`.

## Endpoint Contract

### `prompts/list`

Output:

- `prompts`: `{name, description, title?, arguments?}`
- optional `nextCursor`

Error semantics:

- `kind=not_supported` when prompt catalog disabled
- `kind=not_available` when loading failed and no prompts are available

### `prompts/get`

Input:

- `name` (required)
- `arguments` (optional map of string values)

Output:

- `name`
- `description`
- `messages`

Rendering modes:

- `legacy`: placeholder wrapping compatibility mode
- `strict`: validates required placeholder arguments and optional unknown-argument rejection
- `advanced`: Go template mode (`text/template`) with allowlisted functions

Governance behavior for advanced mode:

- Source roots default to `restricted`
- `advanced` rendering requires prompt source tier `trusted`
- Restricted prompts return `kind=not_supported`, `reason=governance_restricted`

## Watch Mode Contract

`prompt_catalog.watch.mode` behavior:

- `poll`: uses polling fingerprint reload (`auto_reload.*`)
- `event`: uses fsnotify event watch; on watcher init failure, runtime falls back to polling mode

Both modes share the same reload pipeline and list-changed emission logic.

## Error Semantic Contract

| kind | JSON-RPC code | Meaning |
| --- | --- | --- |
| `not_supported` | `-32601` | Feature disabled or blocked by runtime policy |
| `not_available` | `-32000` | Runtime dependency/data not ready |
| `invalid_params` | `-32602` | Request payload invalid for contract |
| `execution_failed` | `-32000` | Runtime path failed after acceptance |

All semantic errors include `error.data.kind`.

## Notifications and Capabilities

- Streamable HTTP target protocol: `2025-11-25`
- Prompt reload entrypoint: `godot-prompts-reload`
- `notifications/prompts/list_changed` emits only when visible list metadata changes
- stdio transport does not emit prompt list changed notifications

## Verification Matrix

| Contract Item | Coverage | Evidence |
| --- | --- | --- |
| Discovery and registration | Automated | `promptcatalog/registry_test.go` |
| Path governance and symlink rejection | Automated | `promptcatalog/registry_test.go` |
| `prompts/list` pagination | Automated | `transport/shared/helpers_prompt_catalog_test.go` |
| `prompts/get` strict validation | Automated | `transport/shared/helpers_prompt_catalog_test.go` |
| Strict mode unknown-argument behavior | Automated | `transport/shared/helpers_prompt_catalog_test.go` |
| Disabled catalog `not_supported` | Automated | `transport/http/prompt_catalog_integration_test.go`, `transport/stdio/prompt_catalog_integration_test.go` |
| Load-failure `not_available` | Automated | `transport/http/prompt_catalog_runtime_test.go` |
| list_changed emission behavior | Automated | `transport/http/prompt_catalog_integration_test.go` |
| Auto reload fingerprint behavior | Automated | `transport/http/prompt_catalog_runtime_test.go` |

## Completion State

Prompt catalog runtime contract is fully implemented for this repository line.
