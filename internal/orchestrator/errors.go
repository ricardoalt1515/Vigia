package orchestrator

import "errors"

var ErrComplaintIdempotencyConflict = errors.New("complaint idempotency replay does not match original request")
