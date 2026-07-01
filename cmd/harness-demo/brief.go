package main

import (
	"encoding/json"
	"fmt"

	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// briefDTO flattens a caseflow.CaseBrief into a JSON-serializable shape. It is forward-only:
// there is no unmarshal path back into the interface-typed CaseBrief.
type briefDTO struct {
	CaseID        string     `json:"case_id"`
	Status        string     `json:"status"`
	Stages        []stageDTO `json:"stages"`
	FailedAgent   string     `json:"failed_agent,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
}

// stageDTO flattens one caseflow.StageEntry.
type stageDTO struct {
	AgentName string          `json:"agent_name"`
	Kind      string          `json:"kind"`
	Handoff   json.RawMessage `json:"handoff"`
}

// kindJSON marshals v and pairs it with its Kind() string. Callers pass the concrete
// caseflow.HandoffArtifact implementation.
func kindJSON(kind caseflow.HandoffKind, v any) (string, json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", nil, err
	}
	return string(kind), b, nil
}

// marshalHandoff type-switches on the concrete caseflow.HandoffArtifact implementation and
// marshals it into the flattened stage shape. Forward-only: never unmarshals into the interface.
func marshalHandoff(h caseflow.HandoffArtifact) (string, json.RawMessage, error) {
	switch v := h.(type) {
	case *caseflow.PolicyExplanation:
		return kindJSON(v.Kind(), v)
	case *caseflow.CaseInvestigation:
		return kindJSON(v.Kind(), v)
	case *caseflow.EvidenceManifestDraft:
		return kindJSON(v.Kind(), v)
	case *caseflow.SupervisorNoteDraft:
		return kindJSON(v.Kind(), v)
	default:
		return "", nil, fmt.Errorf("unknown handoff kind %q", h.Kind())
	}
}

// toBriefDTO flattens a caseflow.CaseBrief into a briefDTO, propagating the first marshalHandoff
// error encountered.
func toBriefDTO(brief caseflow.CaseBrief) (briefDTO, error) {
	dto := briefDTO{
		CaseID:        brief.CaseID,
		Status:        string(brief.Status),
		FailedAgent:   brief.FailedAgent,
		FailureReason: brief.FailureReason,
	}

	dto.Stages = make([]stageDTO, 0, len(brief.Stages))
	for _, stage := range brief.Stages {
		kind, raw, err := marshalHandoff(stage.Handoff)
		if err != nil {
			return briefDTO{}, err
		}
		dto.Stages = append(dto.Stages, stageDTO{
			AgentName: stage.AgentName,
			Kind:      kind,
			Handoff:   raw,
		})
	}

	return dto, nil
}
