package harness

// Budget sets explicit maximums for one runtime step.
type Budget struct {
	MaxModelAttempts int
	MaxToolCalls     int
}
