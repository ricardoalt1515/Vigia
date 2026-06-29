# Foundation Bootstrap Specification

## Purpose

Define the issue #13 foundation required before downstream Vigía implementation: local development infrastructure, initial tenant-aware schema, SQL generation, fail-fast configuration, core type scaffolding, and preserved project paths. This specification intentionally excludes auth runtime behavior, River runtime proof, Harness behavior, MCP, and Bedrock implementation.

## Requirements

### Requirement: Local Development Dependencies

The system SHALL provide a local development command that starts PostgreSQL and MinIO for issue #13 development.

#### Scenario: Start local dependencies

- GIVEN a developer has the repository checkout and required local container runtime
- WHEN the developer runs `make dev`
- THEN PostgreSQL SHALL be started for local database development
- AND MinIO SHALL be started for local object-storage-compatible development
- AND the command SHALL NOT require River, API, Harness, MCP, Bedrock, or auth runtime behavior to be implemented.

#### Scenario: MinIO scope remains readiness only

- GIVEN MinIO is available through the local development stack
- WHEN issue #13 is reviewed
- THEN MinIO acceptance SHALL be limited to local infrastructure and configuration readiness
- AND issue #13 SHALL NOT guarantee WORM behavior, Object Lock enforcement, bucket lifecycle policy behavior, or evidence storage semantics.

### Requirement: Initial Schema Migration

The system SHALL provide an initial migration path that applies the issue #13 database schema successfully through `make migrate-up`.

#### Scenario: Apply initial schema

- GIVEN the local PostgreSQL service is reachable with the configured database URL
- WHEN the developer runs `make migrate-up`
- THEN the initial schema SHALL be applied successfully
- AND the migration SHALL establish the schema foundations required by issue #13 only.

#### Scenario: Avoid downstream runtime scope

- GIVEN the initial schema has been applied
- WHEN reviewers inspect issue #13 behavior
- THEN the schema SHALL NOT require API endpoints, auth middleware, request/session tenant context, River workers, Harness runtime, MCP tools, or Bedrock providers to exist.

### Requirement: Tenant-Scoped Tables and Schema-Level RLS

The system SHALL mark every tenant-scoped table introduced by issue #13 with `tenant_id` and SHALL enable PostgreSQL row-level security on every such table.

#### Scenario: Tenant-scoped tables carry tenant identity

- GIVEN the issue #13 migration introduces a table whose rows belong to a tenant
- WHEN the schema is inspected after migration
- THEN that table SHALL include a `tenant_id` column
- AND the table SHALL be distinguishable from global or reference tables that are not tenant-scoped.

#### Scenario: RLS enabled for tenant-scoped tables

- GIVEN the issue #13 migration introduces tenant-scoped tables
- WHEN the migrated PostgreSQL catalog is inspected
- THEN row-level security SHALL be enabled for every tenant-scoped table introduced by issue #13.

#### Scenario: Runtime tenant isolation remains issue #14

- GIVEN schema-level RLS is enabled by issue #13
- WHEN runtime request isolation is considered
- THEN API-key authentication, tenant lookup, transaction/session tenant variables, and request-level RLS isolation proof SHALL remain out of scope for issue #13
- AND those runtime behaviors SHALL belong to issue #14.

### Requirement: SQLC Query Generation

The system SHALL provide SQL queries under `db/queries` that generate compiling Go query code through the project sqlc configuration.

#### Scenario: Generate query code

- GIVEN issue #13 schema migrations and SQL query files are present
- WHEN the developer runs `make sqlc`
- THEN sqlc SHALL generate Go query code from `db/queries` without errors
- AND the generated code SHALL compile with the repository Go packages.

#### Scenario: SQL-first persistence boundary

- GIVEN issue #13 introduces database access foundations
- WHEN reviewers inspect persistence code
- THEN SQL-first generation through sqlc SHALL be the accepted database foundation
- AND issue #13 SHALL NOT introduce an ORM as the persistence foundation.

### Requirement: Fail-Fast Configuration Loading

The system SHALL provide `internal/config` functionality that loads required environment configuration and fails fast with a useful error when required values are missing or invalid.

#### Scenario: Load valid configuration

- GIVEN all issue #13 required environment variables are present and valid
- WHEN the configuration loader is invoked at startup or by tests
- THEN it SHALL return a validated configuration value suitable for the local bootstrap foundations.

#### Scenario: Reject missing required configuration

- GIVEN one or more required issue #13 environment variables are missing
- WHEN the configuration loader is invoked
- THEN it SHALL fail before dependent runtime work begins
- AND the failure SHALL identify the missing or invalid configuration clearly enough for a developer to correct it.

#### Scenario: Preserve later-provider opt-in boundaries

- GIVEN later Harness work may use a Fake Model Provider by default and Bedrock only as an explicit opt-in in issue #22
- WHEN issue #13 configuration is reviewed
- THEN Bedrock configuration SHALL NOT be required for default issue #13 bootstrap, tests, or demos
- AND issue #13 SHALL NOT make any Harness model provider operational.

### Requirement: Core Foundation Types

The system SHALL provide pure core type scaffolding for the issue #13 schema foundations, including Tenant, Debtor, InteractionEvent, TenantAPIKey, DetectorResultRow, PolicyBundleRule, and other schema-foundation types only where needed by the initial model.

#### Scenario: Core types exist for schema foundations

- GIVEN the issue #13 schema foundation defines canonical entities
- WHEN reviewers inspect `internal/core`
- THEN pure Go core types SHALL exist for the required foundation entities
- AND those types SHALL include Tenant, Debtor, InteractionEvent, TenantAPIKey, DetectorResultRow, and PolicyBundleRule where represented by the issue #13 schema.

#### Scenario: Core remains framework-free

- GIVEN core types represent domain data foundations
- WHEN reviewers inspect those types
- THEN they SHALL NOT depend on database drivers, sqlc generated packages, HTTP frameworks, River runtime types, Harness runtime packages, MCP packages, or Bedrock SDK packages.

### Requirement: Preserved Scaffold Paths

The system SHALL preserve the foundation directory scaffold required for future work, including Harness Lab paths as directories only.

#### Scenario: Required paths survive fresh clone

- GIVEN a fresh clone of the repository after issue #13 is applied
- WHEN the filesystem is inspected
- THEN `internal/harness` SHALL exist
- AND `data/synthetic/cases` SHALL exist
- AND `data/synthetic/harness-runs` SHALL exist.

#### Scenario: Harness behavior remains out of scope

- GIVEN the Harness scaffold paths exist
- WHEN issue #13 is reviewed
- THEN those paths SHALL contain no required Harness runtime behavior, model-provider behavior, domain-agent behavior, event-log behavior, demo CLI behavior, MCP integration, or Bedrock integration.

### Requirement: Downstream Runtime Boundaries

The system SHALL keep issue #13 limited to bootstrap foundations and SHALL NOT implement downstream runtime behavior assigned to later issues.

#### Scenario: River proof remains outside issue 13

- GIVEN planning documents mention River in the broader foundation roadmap
- WHEN issue #13 is implemented or reviewed
- THEN River worker boot, queue processing, and trivial job proof SHALL remain out of scope for issue #13 unless explicitly re-scoped later
- AND the current ownership of River runtime proof SHALL remain issue #1.

#### Scenario: Auth and remote integration order is preserved

- GIVEN issue #13 precedes issue #14, issue #1, and issue #17
- WHEN issue #13 changes are reviewed
- THEN issue #13 SHALL NOT implement issue #14 API-key auth or tenant session context
- AND SHALL NOT implement issue #1 walking skeleton or River runtime proof
- AND SHALL NOT implement issue #17 Remote MCP server
- AND SHALL NOT implement issue #17 before issue #14.

#### Scenario: Model and MCP boundaries are preserved

- GIVEN later plans require separate Judge and Harness model ports and MCP as an external integration surface
- WHEN issue #13 foundations are reviewed
- THEN issue #13 SHALL NOT merge Judge and Harness model ports
- AND SHALL NOT make MCP the internal Harness runtime
- AND SHALL NOT make Bedrock a default provider for tests, demos, or bootstrap behavior.
