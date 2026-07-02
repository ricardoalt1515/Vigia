-- name: LockChainHead :one
-- Insert-or-lock: first append inserts the genesis head; later appends take the
-- row lock via the no-op self-update. Either way returns the locked head.
INSERT INTO ledger_chain_heads (tenant_id, last_seq, last_hash)
VALUES ($1, 0, $2)                       -- $2 = GenesisPrevHash
ON CONFLICT (tenant_id)
DO UPDATE SET last_seq = ledger_chain_heads.last_seq
RETURNING last_seq, last_hash;

-- name: UpdateChainHead :exec
UPDATE ledger_chain_heads SET last_seq = $2, last_hash = $3 WHERE tenant_id = $1;
