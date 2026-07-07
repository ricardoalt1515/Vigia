package outboundgate

import "github.com/ricardoalt1515/vigia/internal/harness"

// Runtime wraps a Harness runtime with the outbound guardrail permission gate.
// The original permission gate remains the fallback for non-outbound tools.
func Runtime(base harness.Runtime, config Config) harness.Runtime {
	config.Fallback = base.Permissions
	base.Permissions = NewGate(config)
	return base
}
