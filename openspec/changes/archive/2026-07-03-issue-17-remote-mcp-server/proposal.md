# Proposal: Remote MCP Server tenant-scoped external AI-client integration

## Summary

Add the first Remote MCP Server slice for Vigía as an external AI-client integration surface. The MCP server exposes a small, safe tool catalog over existing Harness contracts and synthetic Case Brief artifacts while preserving tenant isolation, permission decisions, and auditability.

## Problem

Vigía now has the internal Harness runtime, tool contracts, permission gate, event model, tenant API-key authentication, RLS context helpers, synthetic lab tools, and Case Brief artifacts. External AI clients still have no controlled integration surface. If MCP is added without a strict adapter boundary, it can accidentally become a second authorization model or bypass internal Harness safety rules.

## Goals

- Provide a local Remote MCP server entrypoint for external AI clients.
- Expose a minimal read-only tool catalog for the first slice.
- Resolve tenant/API-key identity before case/artifact lookup or permission decisions.
- Adapt MCP calls to existing Harness tool concepts rather than replacing the Harness runtime.
- Use a tenant-aware synthetic artifact index; clients never provide filesystem paths or authoritative tenant IDs.
- Emit structured audit records for every MCP call.
- Redact or omit raw PII and transcripts from external outputs by default.

## Non-goals

- Do not make MCP the internal Harness runtime.
- Do not expose unrestricted database access, raw transcripts, raw PII, or artifact paths.
- Do not add authority-bearing campaign blocking or evidence-ledger mutation through MCP.
- Do not support client-specific MCP extensions beyond the first standard local server surface.
- Do not rework existing Harness runtime, tenant auth, or Case Brief generation beyond adapter seams needed for this slice.

## Dependencies

- Issue #14 tenant auth/RLS context is closed and present in code.
- Issues #18-#22 Harness Lab stack is merged to `main`.
- The first slice may use synthetic Harness artifacts only through a tenant-aware index/manifest.

## Risk controls

- Fail closed on malformed MCP requests, missing auth, invalid auth, unknown tools, cross-tenant case IDs, and schema-invalid outputs.
- Return the same external denied/not-found shape for cross-tenant, unknown, and stale artifact lookups; keep precise reason only in audit data.
- Treat all client-provided content as untrusted data, never instructions.
- Keep draft/write-class tools out of the first executable catalog unless they only return `approval_required` or `denied` without mutation.

## Success criteria

- A local MCP server command starts and handles initialize/list/call-style JSON-RPC requests.
- At least two read-only tools work end-to-end against synthetic data or Case Brief contracts.
- Tenant A can read its synthetic brief/manifest; Tenant B receives the same external denied/not-found shape.
- Every call emits an audit record with bounded, non-PII metadata.
- Focused Go tests and `go test ./...` pass.
