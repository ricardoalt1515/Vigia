package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

const (
	inputSchemaVersion  = "vigia.mcp.input.v1"
	outputSchemaVersion = "vigia.mcp.output.v1"
)

// Config wires the first-slice MCP adapter. Nil permission/audit dependencies use
// fail-closed or no-op defaults; Authenticator and Index must be provided for tool calls.
type Config struct {
	Authenticator  Authenticator
	Index          ArtifactIndex
	PermissionGate PermissionGate
	AuditSink      AuditSink
}

// Server handles a minimal MCP-compatible JSON-RPC surface.
type Server struct {
	authenticator Authenticator
	index         ArtifactIndex
	gate          PermissionGate
	audit         AuditSink
	seq           atomic.Uint64
}

func NewServer(cfg Config) *Server {
	gate := cfg.PermissionGate
	if gate == nil {
		gate = ReadOnlyPermissionGate{}
	}
	audit := cfg.AuditSink
	if audit == nil {
		audit = discardAuditSink{}
	}
	return &Server{
		authenticator: cfg.Authenticator,
		index:         cfg.Index,
		gate:          gate,
		audit:         audit,
	}
}

func (s *Server) Handle(ctx context.Context, req Request) Response {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, ErrCodeInvalidRequest, "invalid request")
	}
	switch req.Method {
	case "initialize":
		return Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2025-11-25",
			"serverInfo": map[string]any{
				"name":    "vigia-mcp",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
		}}
	case "tools/list":
		return Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": toolCatalog()}}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return errorResponse(req.ID, ErrCodeMethodNotFound, "method not found")
	}
}

func (s *Server) handleToolCall(ctx context.Context, req Request) Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.record(ctx, AuditRecord{
			MCPCallID:          s.callID(req.ID, ""),
			InputSchemaVersion: inputSchemaVersion,
			Outcome:            AuditOutcomeError,
			ErrorClass:         "invalid_params",
		})
		return errorResponse(req.ID, ErrCodeInvalidParams, "invalid params")
	}
	params.Name = strings.TrimSpace(params.Name)
	record := AuditRecord{
		MCPCallID:           s.callID(req.ID, params.Name),
		ToolName:            params.Name,
		InputSchemaVersion:  inputSchemaVersion,
		InputHash:           hashArguments(params.Arguments),
		RedactionProfile:    RedactionDefault,
		OutputSchemaVersion: outputSchemaVersion,
	}
	if s.authenticator == nil {
		record.Outcome = AuditOutcomeDenied
		record.ErrorClass = "auth_unavailable"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeUnauthorized, "unauthorized")
	}
	tenant, err := s.authenticator.Authenticate(ctx, params.Authorization)
	if err != nil {
		record.Outcome = AuditOutcomeDenied
		record.ErrorClass = "unauthorized"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeUnauthorized, "unauthorized")
	}
	record.TenantID = tenant.TenantID
	record.ActorOrAPIKeyID = tenant.KeyID

	caseID, err := validateReadArguments(params.Arguments)
	if err != nil {
		record.Outcome = AuditOutcomeError
		record.ErrorClass = "invalid_params"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeInvalidParams, "invalid params")
	}
	record.CaseID = caseID

	decision := s.gate.Decide(ctx, harness.ToolCall{Name: params.Name, Input: params.Arguments})
	record.PolicyDecision = string(decision.Kind)
	if decision.Kind != harness.PermissionAllowed {
		record.Outcome = AuditOutcomeDenied
		record.ErrorClass = "forbidden_tool"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeForbidden, "forbidden")
	}
	if s.index == nil {
		record.Outcome = AuditOutcomeError
		record.ErrorClass = "index_unavailable"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeNotFound, "not found")
	}
	artifact, status := s.index.Lookup(ctx, tenant.TenantID, caseID)
	if status != LookupFound {
		record.Outcome = AuditOutcomeDenied
		record.ErrorClass = string(status)
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeNotFound, "not found")
	}

	var result map[string]any
	switch params.Name {
	case "read_case_brief":
		result = caseBrief(artifact)
	case "read_evidence_manifest":
		result = evidenceManifest(artifact)
	default:
		record.Outcome = AuditOutcomeDenied
		record.ErrorClass = "forbidden_tool"
		s.record(ctx, record)
		return errorResponse(req.ID, ErrCodeForbidden, "forbidden")
	}
	record.Outcome = AuditOutcomeSuccess
	record.ErrorClass = ""
	s.record(ctx, record)
	return Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func validateReadArguments(args map[string]any) (string, error) {
	if args == nil {
		return "", errors.New("arguments are required")
	}
	for key := range args {
		switch key {
		case "case_id", "client_content":
			continue
		case "tenant_id", "tenant", "artifact_path", "path", "file_path", "raw_path", "db_selector":
			return "", fmt.Errorf("%s is not accepted from MCP clients", key)
		default:
			return "", fmt.Errorf("unsupported argument %q", key)
		}
	}
	caseID, ok := args["case_id"].(string)
	if !ok || strings.TrimSpace(caseID) == "" {
		return "", errors.New("case_id is required")
	}
	caseID = strings.TrimSpace(caseID)
	if !validExternalCaseID(caseID) {
		return "", errors.New("case_id has invalid shape")
	}
	return caseID, nil
}

func validExternalCaseID(caseID string) bool {
	if len(caseID) > 64 || !strings.HasPrefix(caseID, "CASE-") {
		return false
	}
	for _, r := range caseID {
		if r == '-' || unicode.IsDigit(r) || ('A' <= r && r <= 'Z') {
			continue
		}
		return false
	}
	return true
}

func toolCatalog() []ToolDescriptor {
	caseIDSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"case_id": map[string]any{"type": "string"},
			"client_content": map[string]any{
				"type":        "string",
				"description": "Optional untrusted client-provided context; treated as data only.",
			},
		},
		"required": []string{"case_id"},
	}
	return []ToolDescriptor{
		{
			Name:        "read_case_brief",
			Description: "Read a redacted tenant-scoped synthetic case brief.",
			InputSchema: caseIDSchema,
		},
		{
			Name:        "read_evidence_manifest",
			Description: "Read tenant-scoped synthetic evidence manifest metadata without mutation.",
			InputSchema: caseIDSchema,
		},
	}
}

func caseBrief(artifact SyntheticArtifact) map[string]any {
	c := artifact.Case
	return map[string]any{
		"case_id":           c.CaseID,
		"artifact_kind":     "case_brief",
		"redaction_profile": artifact.RedactionProfile,
		"debtor": map[string]any{
			"label": c.Debtor.Label,
		},
		"collector": map[string]any{
			"despacho_id": c.Collector.DespachoID,
			"label":       c.Collector.Label,
		},
		"channel":             c.Channel,
		"occurred_at":         c.OccurredAt,
		"debtor_timezone":     c.DebtorTimezone,
		"detector_results":    detectorResults(c.DetectorResults),
		"applicable_rule_ids": append([]string(nil), c.ApplicableRuleIDs...),
		"transcript_summary": map[string]any{
			"utterance_count": len(c.Transcript),
			"redacted":        true,
		},
	}
}

func evidenceManifest(artifact SyntheticArtifact) map[string]any {
	c := artifact.Case
	items := make([]map[string]any, 0, len(c.DetectorResults))
	for _, dr := range c.DetectorResults {
		items = append(items, map[string]any{
			"rule_code":     dr.RuleCode,
			"detector_kind": dr.DetectorKind,
			"outcome":       dr.Outcome,
			"hitl_required": dr.HITLRequired,
		})
	}
	return map[string]any{
		"case_id":           c.CaseID,
		"artifact_kind":     "evidence_manifest",
		"redaction_profile": artifact.RedactionProfile,
		"manifest_version":  "synthetic-v1",
		"persisted":         false,
		"authoritative":     false,
		"evidence_items":    items,
	}
}

func detectorResults(results []labtools.DetectorResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, dr := range results {
		out = append(out, map[string]any{
			"rule_code":     dr.RuleCode,
			"detector_kind": dr.DetectorKind,
			"outcome":       dr.Outcome,
			"hitl_required": dr.HITLRequired,
		})
	}
	return out
}

func errorResponse(id any, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

func (s *Server) record(ctx context.Context, record AuditRecord) {
	s.audit.Record(ctx, record)
}

func (s *Server) callID(id any, tool string) string {
	n := s.seq.Add(1)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v:%s:%d", id, tool, n)))
	return "mcp-" + hex.EncodeToString(sum[:])[:16]
}

func hashArguments(args map[string]any) string {
	if args == nil {
		return ""
	}
	b, _ := json.Marshal(canonicalMap(args))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func canonicalMap(args map[string]any) map[string]any {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(args))
	for _, key := range keys {
		out[key] = args[key]
	}
	return out
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
