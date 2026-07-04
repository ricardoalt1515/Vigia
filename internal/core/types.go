package core

import "time"

type ID string

type TenantStatus string

const (
	TenantStatusActive   TenantStatus = "active"
	TenantStatusDisabled TenantStatus = "disabled"
)

type APIKeyStatus string

const (
	APIKeyStatusActive  APIKeyStatus = "active"
	APIKeyStatusRevoked APIKeyStatus = "revoked"
)

type InteractionChannel string

const (
	InteractionChannelCall    InteractionChannel = "call"
	InteractionChannelMessage InteractionChannel = "message"
	InteractionChannelEmail   InteractionChannel = "email"
)

type InteractionDirection string

const (
	InteractionDirectionInbound  InteractionDirection = "inbound"
	InteractionDirectionOutbound InteractionDirection = "outbound"
)

type DetectorOutcome string

const (
	DetectorOutcomePass   DetectorOutcome = "pass"
	DetectorOutcomeReview DetectorOutcome = "review"
	DetectorOutcomeFail   DetectorOutcome = "fail"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Tenant struct {
	ID        ID
	Slug      string
	Name      string
	Status    TenantStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TenantAPIKey struct {
	ID         ID
	TenantID   ID
	KeyHash    string
	Label      string
	Status     APIKeyStatus
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

type Debtor struct {
	ID          ID
	TenantID    ID
	ExternalRef string
	DisplayName string
	Timezone    string
	// DateOfBirth is the durable DOB source (issue #7), snapshotted onto
	// InteractionEvent.ContactedPartyDOB at ingest. Nil for debtors that
	// predate this field.
	DateOfBirth *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Despacho is a tenant-scoped attribution/scoping dimension: one tenant
// (creditor) contracts N despachos (collection firms). Minimal identity
// schema only -- no status lifecycle or contract fields (despacho-registry
// spec's non-goals).
type Despacho struct {
	ID          ID
	TenantID    ID
	ExternalRef string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type InteractionEvent struct {
	ID             ID
	TenantID       ID
	DebtorID       ID
	Channel        InteractionChannel
	Direction      InteractionDirection
	Status         string
	OccurredAt     time.Time
	TranscriptRef  *string
	DebtorTimezone string
	// DespachoID is the optional despacho attribution FK (issue #7). Nil
	// when the interaction has not been attributed to a despacho.
	DespachoID *ID
	// ContactPartyRelationship is the contacted party's relationship to the
	// debtor ("debtor" | "authorized" | "third_party"), used by
	// MX-REDECO-06/07. Nil means unresolved (detectors fail closed).
	ContactPartyRelationship *string
	// ContactedPartyDOB is the snapshot of the contacted party's date of
	// birth at ingest (from Debtor.DateOfBirth), used by MX-REDECO-07.
	ContactedPartyDOB *time.Time
	// AuthorizedChannels is the per-interaction snapshot of channels the
	// debtor authorized, used by MX-REDECO-11.
	AuthorizedChannels []string
	// PaymentRecipient identifies who the interaction directed payment to
	// ("creditor" | other), used by MX-REDECO-10.
	PaymentRecipient *string
	// DisclosureProvided records whether the UNE/complaints-unit disclosure
	// was stated, used by MX-REDECO-03. Nil fails closed to "warn" (not
	// "block"), per that rule's WARN-level catalog action.
	DisclosureProvided *bool
	CreatedAt          time.Time
}

// Evaluation is the persisted header row proving a policy evaluation ran for
// an interaction. It records the overall outcome ("pass" | "fail") derived
// from one or more detector results.
type Evaluation struct {
	ID                  ID
	TenantID            ID
	InteractionEventID  ID
	OverallOutcome      string
	PolicyBundleVersion string
	// PolicyBundleID is the composite FK to the resolved active bundle
	// (issue #6). Nil when no bundle was active at evaluation time
	// (Design Decision 3's sentinel path) or for pre-#6 rows.
	PolicyBundleID *ID
	CreatedAt      time.Time
}

type PolicyRule struct {
	ID          ID
	Code        string
	Title       string
	Description string
	Severity    Severity
	CreatedAt   time.Time
}

type PolicyBundle struct {
	ID        ID
	TenantID  ID
	Name      string
	Version   string
	Status    string
	CreatedAt time.Time
}

type PolicyBundleRule struct {
	TenantID       ID
	PolicyBundleID ID
	PolicyRuleID   ID
	CreatedAt      time.Time
	// EffectiveDate and LegalBasis are issue #6 additions: every rule
	// snapshot records when it applied and its legal grounding.
	EffectiveDate time.Time
	LegalBasis    string
}

type DetectorResultRow struct {
	ID                 ID
	TenantID           ID
	InteractionEventID ID
	DetectorCode       string
	Outcome            DetectorOutcome
	Severity           Severity
	ResultPayload      []byte
	CreatedAt          time.Time
}
