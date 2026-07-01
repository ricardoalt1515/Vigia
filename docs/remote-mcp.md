# Remote MCP Server

Vigía exposes a first-slice local MCP-compatible JSON-RPC adapter for external AI clients. This is an external integration surface only; it is not the internal Harness runtime.

## Run locally

```bash
export VIGIA_MCP_API_KEY=local-dev-key
export VIGIA_MCP_TENANT_ID=SYN-TENANT-001
export VIGIA_MCP_KEY_ID=local-dev-key-id
go run ./cmd/vigia-mcp
```

The command reads newline-delimited JSON-RPC requests from stdin and writes one response per line to stdout.

Example:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_case_brief","authorization":"Bearer local-dev-key","arguments":{"case_id":"CASE-SYN-001"}}}
```

## Exposed tools

Only first-slice read tools are exposed:

- `read_case_brief` — returns a redacted synthetic case brief.
- `read_evidence_manifest` — returns synthetic evidence manifest metadata without mutation.

Draft and authority-bearing tools are intentionally not executable through this adapter.

## Security boundaries

- Tool calls authenticate before any case or artifact lookup.
- The client provides `case_id`; it must not provide `tenant_id`, artifact paths, raw database selectors, or filesystem paths.
- Cross-tenant and unknown cases return the same external `not found` shape to avoid tenant enumeration.
- Raw transcript text, raw PII, and API keys are excluded from external outputs and audit records.
- Client-provided content is treated as untrusted data, never instructions.

## Audit records

Every MCP tool call emits bounded audit metadata in-process. Records include call ID, tenant ID, actor/API-key ID, tool name, input hash, case ID, permission decision, redaction profile, output schema version, outcome, and bounded error class.

Audit records do not include raw API keys, raw transcripts, or raw PII.

## Non-goals for this slice

- No HTTP/SSE remote transport.
- No database-backed artifact index.
- No evidence ledger mutation.
- No campaign blocking or other authority-bearing action.
- No client-specific MCP extensions.
