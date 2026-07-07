package outbound

import (
	"context"
	"errors"
)

// ProposalContextResolver resolves authority context from a trusted, already
// expanded outbound proposal. It is intended for dry-run campaign preflight,
// where the campaign artifact supplies recipient/channel/timezone context and
// no external send provider is involved.
type ProposalContextResolver struct{}

func (ProposalContextResolver) Resolve(_ context.Context, tenantID string, p OutboundActionProposal) (AuthorityContext, error) {
	if tenantID == "" {
		return AuthorityContext{}, errors.New("tenant id is required")
	}
	return AuthorityContext{
		TenantID:                 tenantID,
		DebtorID:                 p.DebtorID,
		DebtorTimezone:           p.DebtorTimezone,
		Channel:                  p.Channel,
		ProposedAt:               p.ProposedAt,
		ContactPartyRelationship: p.ContactPartyRelationship,
		AuthorizedChannels:       p.AuthorizedChannels,
		PaymentRecipient:         p.PaymentTarget,
	}, nil
}

var _ AuthorityContextResolver = ProposalContextResolver{}
