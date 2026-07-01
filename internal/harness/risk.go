package harness

// RiskClass classifies a tool's potential regulatory impact and required authorization level.
// It is a static property of the tool contract and MUST NOT change between calls.
type RiskClass string

const (
	// RiskClassRead denotes tools that read data and produce no side effects.
	RiskClassRead RiskClass = "read"
	// RiskClassDraft denotes tools that propose artifacts without persisting them.
	RiskClassDraft RiskClass = "draft"
	// RiskClassAuthority denotes tools that produce regulatory side effects and
	// require explicit human authorization before execution.
	RiskClassAuthority RiskClass = "authority"
)
