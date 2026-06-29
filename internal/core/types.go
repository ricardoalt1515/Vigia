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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type InteractionEvent struct {
	ID            ID
	TenantID      ID
	DebtorID      ID
	Channel       InteractionChannel
	Direction     InteractionDirection
	Status        string
	OccurredAt    time.Time
	TranscriptRef *string
	CreatedAt     time.Time
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
