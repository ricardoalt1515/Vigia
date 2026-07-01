package mcp

import (
	"context"
	"encoding/json"

	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

const (
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeUnauthorized   = -32001
	ErrCodeForbidden      = -32003
	ErrCodeNotFound       = -32004
)

const (
	RedactionDefault = "external-default"

	AuditOutcomeSuccess = "success"
	AuditOutcomeDenied  = "denied"
	AuditOutcomeError   = "error"
)

const (
	PermissionAllowed          = harness.PermissionAllowed
	PermissionDenied           = harness.PermissionDenied
	PermissionApprovalRequired = harness.PermissionApprovalRequired
)

// Request is the minimal JSON-RPC request shape accepted by the local MCP adapter.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is the minimal JSON-RPC response shape emitted by the local MCP adapter.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError is a bounded client-facing JSON-RPC error. It intentionally avoids
// internal tenant-enumeration and authorization details.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolDescriptor is the external catalog entry exposed through tools/list.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolCallParams is the params object for tools/call.
type ToolCallParams struct {
	Name          string         `json:"name"`
	Authorization string         `json:"authorization"`
	Arguments     map[string]any `json:"arguments"`
}

// Authenticator resolves client-provided auth metadata into tenant context before
// any artifact lookup. It is intentionally compatible with auth.Authenticator.
type Authenticator interface {
	Authenticate(ctx context.Context, authorization string) (auth.TenantContext, error)
}

// PermissionGate is the same decision seam used by the internal Harness runtime.
type PermissionGate interface {
	Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision
}

// ArtifactIndex resolves tenant-scoped synthetic artifacts without trusting
// client-provided tenant IDs or file paths.
type ArtifactIndex interface {
	Lookup(ctx context.Context, tenantID, caseID string) (SyntheticArtifact, LookupStatus)
}

// SyntheticArtifact is the narrow input used by external read tools.
type SyntheticArtifact struct {
	Case             labtools.SyntheticCase
	RedactionProfile string
}

type LookupStatus string

const (
	LookupFound       LookupStatus = "found"
	LookupNotFound    LookupStatus = "not_found"
	LookupCrossTenant LookupStatus = "cross_tenant"
)
