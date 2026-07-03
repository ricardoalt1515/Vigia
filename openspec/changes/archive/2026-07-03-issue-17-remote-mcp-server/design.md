# Design: Remote MCP Server tenant-scoped external AI-client integration

## Objective

Implement the first reviewable MCP slice as a local external adapter over existing Vigía Harness concepts. The adapter proves tenant-scoped read access, redacted output, fail-closed request handling, and audit events without turning MCP into the internal runtime.

## Non-goals

- No internal Harness runtime replacement.
- No raw database access surface.
- No raw transcripts or raw PII in external responses.
- No evidence ledger mutation or authority-bearing campaign actions.
- No external network dependency in default tests.
- No Bedrock default path changes.

## Architecture

### Boundary

Add a new MCP adapter package separate from the internal Harness runtime. The package owns transport request/response handling, external tool schemas, authentication metadata extraction, synthetic tenant index lookup, and audit record emission.

The adapter may reuse Harness concepts (`ToolCall`, `ToolResult`, `PermissionDecisionKind`) as vocabulary, but MCP request dispatch remains an adapter layer.

### Transport

Use a minimal JSON-RPC-over-stdio compatible command for the first slice. This avoids adding a server framework and keeps local AI-client integration testable without networking.

Suggested command boundary:

- `cmd/vigia-mcp` starts the local server.
- It reads newline-delimited JSON-RPC requests from stdin.
- It writes one JSON-RPC response per line to stdout.

Supported first-slice methods:

- `initialize`
- `tools/list`
- `tools/call`

### Tools

Expose exactly these first-slice tools:

- `read_case_brief`
- `read_evidence_manifest`

Draft tools are not executable in this slice. If advertised later, they must return `approval_required` or `denied` unless explicitly allowed by the permission gate.

### Authentication and tenant context

For this local first slice, each `tools/call` request must carry authentication metadata equivalent to an API-key identity. The adapter resolves this into a tenant/key context before lookup. Tests can use an in-memory authenticator seam so they do not require Postgres.

The adapter must reject requests before lookup when authentication is missing or invalid.

### Tenant-aware synthetic index

Introduce an in-memory synthetic index for the first slice. Entries include:

- `tenant_id`
- `case_id`
- redaction profile
- artifact metadata for brief and manifest outputs

The client never provides `tenant_id` or artifact paths. It provides `case_id` only, and the adapter joins it with the authenticated tenant.

### Output safety

`read_case_brief` returns a narrow external DTO with case identity, tenant-safe labels, risk summary, detector/rule summary, and redaction metadata. It must not return raw transcript text by default.

`read_evidence_manifest` returns manifest metadata only: case ID, artifact kind, redaction profile, generated/provenance metadata, and evidence item summaries. It must not mutate the evidence ledger.

### Permission envelope

Each tool call passes through a permission decision seam. The first-slice default gate can allow read tools and deny unknown/draft/authority tools. Denied or approval-required calls do not execute data lookup.

### Audit records

Every call appends an audit record to an in-memory sink. Records include:

- `mcp_call_id`
- `tenant_id`
- `actor_or_api_key_id`
- `tool_name`
- `input_schema_version`
- `input_hash`
- `case_id` when present
- `policy_decision`
- `redaction_profile`
- `output_schema_version`
- `outcome`
- `error_class`

Audit records must not include raw API keys, raw transcripts, or raw PII.

### Failure semantics

Cross-tenant, unknown, and stale case/artifact lookups return the same external denied/not-found shape. Audit records keep bounded internal error classes so operators can distinguish causes without turning the MCP surface into a tenant enumeration oracle.

## TDD seams

- Package-level tests for MCP request validation and JSON-RPC method handling.
- Package-level tests for tenant-aware index lookup and cross-tenant indistinguishability.
- Package-level tests for redaction/no transcript leakage.
- Package-level tests for audit record shape and no secret leakage.
- Command smoke test for initialize/tools list if command is included in the first slice.

## Verification

Focused:

```bash
go test ./internal/mcp ./cmd/vigia-mcp -count=1
```

Full:

```bash
go test ./... -count=1
```

## Review workload forecast

The full issue may exceed 400 changed lines. The first slice should stay reviewable by limiting scope to a local stdio JSON-RPC adapter, two read-only tools, in-memory synthetic index, audit sink, and docs/OpenSpec updates. Remote HTTP/SSE transport, database-backed artifact index, and draft tools should be follow-ups unless the first slice remains small enough.
