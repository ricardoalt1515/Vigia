// Package bedrock (doc.go): testing gap disclosure.
//
// A live Bedrock round trip is NOT automated by `go test ./internal/harness/bedrock/...` — every
// test in this package uses a fake invoker (see client_test.go's fakeInvoker) and never contacts
// AWS. This is intentional: it keeps the default test suite credential-free and network-free per
// project conventions ("Bedrock stays opt-in only").
//
// Exercising a real Bedrock call requires valid AWS_REGION/BEDROCK_MODEL_ID env vars, resolvable
// AWS credentials with bedrock:InvokeModel permission, and manual invocation outside `go test
// ./...` (e.g. via `cmd/harness-demo --provider bedrock`, added in Slice 2 of issue #22). This gap
// is deliberate and documented here, not silently skipped.
package bedrock
