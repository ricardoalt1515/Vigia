# Remote MCP Server Specification

## Purpose

Define the tenant-scoped external AI-client integration surface for Vigía. The MCP server exposes a small, safe, read-only tool catalog over existing Harness contracts and synthetic Case Brief artifacts while preserving tenant isolation, permission decisions, and auditability. This is an external adapter, not the internal Harness runtime.

## ADDED Requirements

### Requirement: Local Remote MCP server entrypoint

Vigía SHALL provide a local MCP-compatible server entrypoint for external AI clients. The entrypoint SHALL be an external adapter and MUST NOT become the internal Harness runtime.

#### Scenario: Server handles initialize

- **Given** the MCP server is running locally
- **When** a client sends an initialize-style JSON-RPC request
- **Then** the server returns protocol metadata and server capabilities
- **And** no tenant data is read

#### Scenario: Server lists approved tools

- **Given** the MCP server is running locally
- **When** a client requests the tool catalog
- **Then** the response includes only approved first-slice external tools
- **And** authority-bearing tools are not executable in the catalog

---

### Requirement: Tenant authentication before lookup

Every MCP tool call that can read tenant-scoped data SHALL resolve tenant/API-key identity before any case, artifact, or permission lookup.

#### Scenario: Missing authentication fails closed

- **Given** a tool call that reads case data
- **When** authentication metadata is missing
- **Then** the call fails with a structured unauthorized error
- **And** no case/artifact lookup is performed

#### Scenario: Cross-tenant case lookup is externally indistinguishable from unknown case

- **Given** tenant A owns a synthetic case
- **When** tenant B requests the same case ID through MCP
- **Then** the external response is the same denied/not-found shape used for an unknown case
- **And** the audit record preserves the precise cross-tenant reason without exposing it to the client

---

### Requirement: Tenant-aware synthetic artifact index

MCP clients SHALL reference synthetic artifacts through tenant-scoped case/tool identifiers. Clients MUST NOT provide filesystem paths or authoritative tenant IDs.

#### Scenario: Artifact path input is rejected

- **Given** a tool schema for reading a case brief or evidence manifest
- **When** the request includes a raw artifact path or tenant override
- **Then** validation fails closed
- **And** no file path is read from client input

---

### Requirement: Safe read-only tools

The first slice SHALL expose at least two read-only tools over existing synthetic data or Case Brief contracts.

#### Scenario: Read case brief succeeds for owning tenant

- **Given** an authenticated tenant owns a synthetic case
- **When** it calls the external read case brief tool for that case
- **Then** the tool returns structured case brief data
- **And** raw transcript text is omitted by default

#### Scenario: Read evidence manifest succeeds for owning tenant

- **Given** an authenticated tenant owns a synthetic case
- **When** it calls the external read evidence manifest tool for that case
- **Then** the tool returns structured evidence manifest metadata
- **And** no evidence ledger mutation occurs

---

### Requirement: Permission and audit envelope

Every MCP tool call SHALL pass through a permission decision envelope and emit a structured audit record.

#### Scenario: Audit record is emitted for successful call

- **Given** a successful MCP read tool call
- **When** the call completes
- **Then** an audit record includes call ID, tenant ID, actor/key ID, tool name, input schema version, input hash, case ID when present, policy decision, redaction profile, output schema version, outcome, and bounded error class
- **And** the audit record excludes raw API keys, raw transcripts, and raw PII

#### Scenario: Unknown tool fails closed

- **Given** a client calls an unknown MCP tool
- **When** the request is handled
- **Then** the server returns a structured error
- **And** the audit record captures the denied outcome

---

### Requirement: Prompt injection treated as data

Client or connector-provided content SHALL be treated as untrusted data and MUST NOT override policy, tenant, redaction, or permission behavior.

#### Scenario: Injection attempt does not reveal raw data

- **Given** a request includes text asking the server to reveal raw PII, raw transcripts, or bypass policy
- **When** the MCP tool executes
- **Then** the output remains redacted or returns a structured refusal
- **And** no policy boundary is bypassed
