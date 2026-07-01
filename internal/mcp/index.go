package mcp

import (
	"context"

	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

// SyntheticIndex is the first-slice tenant-aware index over synthetic fixtures.
// It keeps tenant ownership server-side, so clients can only address artifacts by case_id.
type SyntheticIndex struct {
	cases map[string]labtools.SyntheticCase
}

func NewSyntheticIndex(cases labtools.CaseStore) *SyntheticIndex {
	copyCases := make(map[string]labtools.SyntheticCase, len(cases))
	for id, c := range cases {
		copyCases[id] = c
	}
	return &SyntheticIndex{cases: copyCases}
}

func (i *SyntheticIndex) Lookup(ctx context.Context, tenantID, caseID string) (SyntheticArtifact, LookupStatus) {
	if i == nil || caseID == "" {
		return SyntheticArtifact{}, LookupNotFound
	}
	c, ok := i.cases[caseID]
	if !ok {
		return SyntheticArtifact{}, LookupNotFound
	}
	if c.TenantID != tenantID {
		return SyntheticArtifact{}, LookupCrossTenant
	}
	return SyntheticArtifact{Case: c, RedactionProfile: RedactionDefault}, LookupFound
}
