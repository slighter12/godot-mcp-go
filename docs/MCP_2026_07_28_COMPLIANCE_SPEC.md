# MCP 2026-07-28 Compliance Specification

Status: Proposed  
Target: `godot-mcp-go`  
Protocol revision: `2026-07-28`

## Purpose

Bring the server's declared MCP surface into verifiable alignment with the
`2026-07-28` protocol revision. The result must support the applicable core
features, preserve the existing `godot.*` contract, and produce repeatable
evidence from the official MCP conformance runner.

The server already implements the main stateless lifecycle, per-request
metadata, standard HTTP routing headers, cache hints for most list results,
`server/discover`, and `subscriptions/listen`. This specification closes the
remaining protocol-surface and verification gaps.

## Sources of Truth

Implementation decisions must be checked against these first-party sources:

- [MCP 2026-07-28 specification](https://modelcontextprotocol.io/specification/2026-07-28)
- [MCP 2026-07-28 schema](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2026-07-28/schema.json)
- [MCP 2026-07-28 changelog](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/changelog.mdx)
- [Official MCP conformance runner](https://github.com/modelcontextprotocol/conformance)

When prose and schema appear to disagree, use the released schema for wire
shape and the specification prose for behavioral requirements. Record any
remaining ambiguity in the implementation handoff instead of inventing a
project-specific protocol form.

## Scope

### Included

- Streamable HTTP and stdio behavior for protocol revision `2026-07-28`.
- Capability discovery and method dispatch for the core features this server
  declares.
- `resources/templates/list`, including required cache metadata.
- Conditional `completion/complete`, enabled only when a completion provider is
  configured through the production server options; the default executable
  omits it.
- Multi Round-Trip Requests (MRTR) result encoding and state validation for
  production-capable handlers that opt into the shared coordinator; the
  default Godot catalog currently has no such handler.
- Standard custom HTTP header mapping described by `x-mcp-header`.
- MCP tool result content blocks for text, image, audio, embedded resource,
  resource link, and mixed-content results.
- Official conformance coverage through an opt-in test fixture server.
- CI enforcement for the applicable, frozen `2026-07-28` requirements.
- Focused Godot editor/runtime smoke verification after protocol work passes.

### Excluded

- Compatibility with protocol revisions before `2026-07-28`.
- Tasks and other optional MCP extensions that this server does not advertise.
- Adding Roots, Sampling, or Logging as standalone server features; these are
  deprecated in `2026-07-28`. MRTR may still carry the standard input request
  forms when a handler explicitly needs them.
- OAuth or remote authorization deployment policy.
- Renaming existing `godot.*` tools or changing their documented payloads.
- Adding conformance-only tools, resources, or prompts to the normal production
  catalog.

## Agreed Test Seams

Tests must exercise behavior at these public seams:

1. Shared JSON-RPC dispatch
   - Covers method availability, result envelopes, MRTR state, completion, and
     resource template behavior shared by HTTP and stdio.
2. Streamable HTTP
   - Covers protocol headers, custom header mapping, content negotiation,
     status codes, request cancellation, cache metadata, and SSE behavior.
3. Official conformance runner
   - Treats a running server as a black box and supplies standard fixture
     tools, resources, prompts, and completion references through a dedicated
     opt-in harness.

Stdio tests verify framing and parity with shared dispatch. They do not copy
the complete HTTP test suite.

## Functional Requirements

### 1. Capability Accuracy

`server/discover` must advertise only behavior available for the current
server configuration.

- Every advertised core capability has a working method implementation.
- Optional capabilities are omitted when disabled.
- Server identity remains under
  `_meta["io.modelcontextprotocol/serverInfo"]`.
- Discovery remains cacheable and includes `resultType`, `ttlMs`, and
  `cacheScope` as required by the released schema.
- Existing Godot extensions remain namespaced and keep their current meaning.

### 2. Resource Templates

Implement `resources/templates/list` in shared dispatch.

- The result uses the released `ListResourceTemplatesResult` wire shape.
- The list is deterministic and supports the repository's existing cursor
  convention where pagination is applicable.
- The result contains `resultType`, `ttlMs`, `cacheScope`, and server metadata.
- An installation with no templates returns a successful empty list.
- Unsupported or malformed cursors return `-32602`.

### 3. Completion

Implement `completion/complete` for configured prompt and resource-template
providers. Omit the capability and return method-not-found when no provider is
configured.

- The handler validates the released reference and argument shapes.
- The result contains at most 100 values and correctly reports `total` and
  `hasMore` when those values are known.
- Unsupported references and malformed parameters return `-32602` without
  leaking internal details.
- Capability advertisement matches whether completion is enabled.
- Empty completion candidates produce a valid successful result.

### 4. Multi Round-Trip Requests

The shared coordinator permits explicitly MRTR-capable `tools/call`,
`prompts/get`, and `resources/read` handlers to return `resultType:
"input_required"` when they need client input. Production embeddings enable
state protection with explicit key material; the conformance fixture consumes
the same handler interfaces. The default Godot catalog has no MRTR handler.

- `inputRequests`, `inputResponses`, and `requestState` follow the released
  schema without project-specific wrappers.
- A retry uses a fresh JSON-RPC request ID and re-enters the original handler.
- `inputResponses` apply only to the current round.
- Opaque `requestState` is treated as attacker-controlled input.
- Any server-minted state is confidential and integrity-protected with
  AES-256-GCM, bound to the original method, handler identity, relevant
  parameters, current response contract, optional authenticated principal,
  round, and expiry.
- Every retry is validated before handler entry. Unknown well-formed response
  IDs are ignored, malformed or contract-mismatched values return `-32602`,
  and partial valid responses are encrypted while only missing inputs are
  requested again.
- `inputResponses` are limited to 256 KiB, 128 top-level entries, depth 32,
  and 4096 decoded nodes. Form elicitation accepts only the released restricted
  top-level primitive/enum schema; URL elicitation requires an absolute URL.
- Invalid, expired, or tampered state returns the specification-required
  `-32602` response without disclosing verification details.
- Extra parameters are ignored only where the released schema permits them.
- A handler that never needs client input continues to return the existing
  complete result shape.

### 5. Custom HTTP Headers

Implement the standard `x-mcp-header` mapping defined by the released schema.

- Header extraction is based on validated method parameters.
- Header names and values use the specification's encoding rules.
- `MCP-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` remain authoritative and
  cannot be overridden by custom parameter mappings.
- Duplicate, conflicting, malformed, or unsafe values fail closed with the
  specification-required JSON-RPC error and HTTP status.
- Tests cover literal and Base64 sentinel forms.

### 6. Tool Result Content

The public tool-result representation must preserve standard MCP content block
types rather than coercing every result into text.

- Supported blocks: text, image, audio, embedded resource, resource link, and
  mixed arrays of those blocks.
- Existing tools that return structured JSON retain `structuredContent` and
  their current human-readable text fallback.
- Binary values remain encoded according to the released schema.
- Decoded blocks and ordinary raw tool output are limited to 8 MiB, and the
  complete normalized result is limited to 16 MiB; invalid block shapes, MIME
  types, URIs, Base64, or oversized output fail as protocol errors.
- `outputSchema`, when declared, is preserved as JSON Schema and is not
  automatically dereferenced from external URIs.
- Semantic tool failures continue to use `isError` and structured error data;
  protocol validation failures remain JSON-RPC errors.

### 7. Stateless Transport Behavior

Preserve the stateless behavior already implemented.

- Every request requires protocol version and client capabilities in `_meta`.
- `clientInfo` remains optional.
- Streamable HTTP requires matching `MCP-Protocol-Version`, `Mcp-Method`, and,
  for named operations, `Mcp-Name`.
- Removed initialize, initialized, session, GET `/mcp`, and DELETE `/mcp`
  forms remain rejected.
- Aborting an in-flight HTTP POST cancels its request and SSE context and
  suppresses late results. The current `Tool.Execute` contract is not
  cooperatively cancellable, so an operation that already started may finish.
- A subscription stream carries only acknowledged notification categories and
  never carries independent JSON-RPC requests.

## Conformance Harness

Create an opt-in conformance fixture entry point that uses the real transport
and shared dispatch layers while registering the deterministic fixtures
expected by the official runner.

- The fixture entry point is inaccessible in the normal server configuration.
- Fixture names and values are local constants verified against the pinned
  runner's scenario list.
- The harness exercises the same result encoding and validation code used by
  production tools.
- Conformance output is written to a temporary or ignored directory.
- CI pins `@modelcontextprotocol/conformance@0.2.0-alpha.11` until a stable
  release supporting `2026-07-28` is available.
- A dependency update must include a documented manual diff of the runner's
  frozen requirement list and explicitly accept any new scenarios.

The primary command must run the frozen release requirements, not the mutable
default suite:

```sh
bunx @modelcontextprotocol/conformance@0.2.0-alpha.11 server \
  --url http://127.0.0.1:<port>/mcp \
  --requirements 2026-07-28 \
  --output-dir <temporary-directory>
```

Scenarios marked extension or pending by the runner are reported separately
and do not block core compliance unless this server advertises the associated
feature.

## Verification Requirements

The implementation is complete only when all applicable checks below pass.

1. `go test ./...`
2. Focused shared-dispatch tests for resource templates, completion, MRTR, and
   content block preservation.
3. Focused Streamable HTTP tests for headers, cancellation, cache metadata,
   and HTTP/JSON-RPC error mapping.
4. Existing smoke targets documented in `docs/DEVELOPMENT.md`.
5. Frozen official `2026-07-28` conformance requirements through the opt-in
   fixture server, with no expected-failure waiver for an advertised core
   capability.
6. Godot addon static check.
7. Manual or automated Godot editor/runtime smoke:
   - editor registration and fresh snapshot;
   - one editor-backed read;
   - one mutating call with the Godot capability;
   - runtime registration and session discovery;
   - one runtime command round trip;
   - cancellation or reconnect recovery.

If Docker or Godot is unavailable, report those gates as unverified and include
the exact commands and manual checklist. Passing unit tests must not be reported
as equivalent to those environment-dependent gates.

## CI Requirements

CI must run the repository's documented release gates rather than a subset
that can drift silently.

- Keep one authoritative list of release commands.
- Run the frozen official conformance requirements on pull requests and pushes
  to the default branch.
- Preserve focused Go tests for fast failure before external harnesses run.
- Pin external actions, images, and conformance packages to explicit versions.
- Store conformance diagnostics as CI artifacts when the suite fails.

## Compatibility and Safety Constraints

- Preserve existing user work and public `godot.*` contracts.
- Prefer additions at shared protocol seams over duplicated HTTP/stdio logic.
- Introduce no new runtime dependency unless the released protocol cannot be
  implemented safely with the standard library and existing modules.
- Apply strict input bounds to schemas, content payloads, MRTR state, and
  header values.
- Keep secrets, authentication material, and internal verification reasons out
  of wire responses and test artifacts.
- Keep the production server free of conformance-only fixtures.

## Acceptance Criteria

The work is accepted when:

- the server's discovery response accurately describes its enabled behavior;
- every advertised core method has focused HTTP and shared-dispatch evidence;
- resource templates, completion, MRTR, custom headers, and standard content
  blocks meet the released wire schema;
- all existing tests and smoke checks pass;
- the opt-in fixture server passes all applicable frozen `2026-07-28` core
  conformance scenarios without waivers;
- extension and pending outcomes are separated from the core compliance claim;
- Godot editor/runtime validation is either passed or explicitly handed off
  with reproducible steps and stated environment risks; and
- documentation states precisely which MCP revision and optional extensions
  the server supports.
