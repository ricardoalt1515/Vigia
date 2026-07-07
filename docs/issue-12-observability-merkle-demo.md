# Issue 12 demo: timestamped Merkle checkpoint and GenAI trace

Issue 12 adds the final trust-and-visibility layer: evidence records can be summarized into a Merkle checkpoint, the checkpoint bytes can be anchored with an RFC 3161 timestamp token, and judge calls now emit OpenTelemetry GenAI attributes for cost/quality backends.

## Quick path

1. Start the local stack and run migrations:

   ```bash
   make dev
   make migrate-up
   ```

2. Seed/evaluate enough data to create evidence records, then verify the chain:

   ```bash
   make seed-dev
   go run ./cmd/ledger-verify -tenant-id <tenant_uuid>
   ```

3. Create a timestamped Merkle checkpoint:

   ```bash
   go run ./cmd/ledger-checkpoint \
     -tenant-id <tenant_uuid> \
     -tsa-url https://freetsa.org/tsr
   ```

   Expected output:

   ```text
   checkpoint created: tenant=<tenant_uuid> seq=1..N records=N root=<sha256> tsa=https://freetsa.org/tsr
   ```

## What reviewers should verify

| Area | What to check |
|------|---------------|
| Merkle root | `internal/ledger/checkpoint.go` uses RFC 6962-style domain separation: `0x00` for leaves, `0x01` for internal nodes. |
| Timestamp binding | The RFC 3161 request is over `MerkleCheckpoint.CanonicalBytes()`, not over mutable DB metadata. |
| Persistence | `merkle_checkpoints` stores the canonical body, TSA URL, token bytes, root hash, range, and chain head hash. |
| Daily job | `cmd/worker` registers the checkpoint worker only when `RFC3161_TSA_URL` is configured. |
| GenAI observability | `internal/judge/anthropic.go` emits `gen_ai.*` attributes around the judge call without transcript text or rationale PII. |

## Case-study story

Vigia already stores every evaluation as an append-only hash-chained `EvidenceRecord`. That proves tampering inside one tenant's chain, but an operator still needs a compact, externally timestamped statement that says: "as of this time, these exact records existed in this order."

The Merkle checkpoint is that statement. It commits to a contiguous range of evidence record hashes and stores the current chain head. The RFC 3161 token then provides third-party timestamp evidence for the checkpoint payload. In a demo, the reviewer can verify the chain, create a checkpoint, and show the checkpoint root plus timestamp token as the durable audit anchor.

For agentic visibility, the Anthropic judge span follows OpenTelemetry GenAI naming so an OTel/Langfuse-compatible backend can group model calls by tenant request context, model, rubric version, cache-read tokens, cache-creation tokens, verdict, and confidence. That gives operators the cost/quality lens without leaking transcripts into telemetry.

## Out of scope for this slice

- Certificate-chain trust-policy configuration for each TSA root.
- A hosted Langfuse deployment.
- A full frontend chart for the checkpoint table.
- Voice/STT files owned by issue #11's parallel worktree.
