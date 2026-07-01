package mcp

import (
	"context"
	"sync"
)

// AuditRecord is the bounded audit envelope for one MCP tool call. It contains
// identifiers and classifications only; raw request bodies, API keys, transcripts,
// and PII are intentionally excluded.
type AuditRecord struct {
	MCPCallID           string `json:"mcp_call_id"`
	TenantID            string `json:"tenant_id,omitempty"`
	ActorOrAPIKeyID     string `json:"actor_or_api_key_id,omitempty"`
	ToolName            string `json:"tool_name,omitempty"`
	InputSchemaVersion  string `json:"input_schema_version"`
	InputHash           string `json:"input_hash,omitempty"`
	CaseID              string `json:"case_id,omitempty"`
	PolicyDecision      string `json:"policy_decision,omitempty"`
	RedactionProfile    string `json:"redaction_profile,omitempty"`
	OutputSchemaVersion string `json:"output_schema_version,omitempty"`
	Outcome             string `json:"outcome"`
	ErrorClass          string `json:"error_class,omitempty"`
}

// AuditSink receives bounded MCP audit records.
type AuditSink interface {
	Record(ctx context.Context, record AuditRecord)
}

// MemoryAuditSink stores audit records in memory for local operation and tests.
type MemoryAuditSink struct {
	mu      sync.Mutex
	records []AuditRecord
}

func (s *MemoryAuditSink) Record(ctx context.Context, record AuditRecord) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

func (s *MemoryAuditSink) Records() []AuditRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditRecord, len(s.records))
	copy(out, s.records)
	return out
}

type discardAuditSink struct{}

func (discardAuditSink) Record(ctx context.Context, record AuditRecord) {}
