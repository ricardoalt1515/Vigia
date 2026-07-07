#!/usr/bin/env bash
set -euo pipefail

TENANT_ID="${1:-}"
TSA_URL="${RFC3161_TSA_URL:-https://freetsa.org/tsr}"

if [[ -z "$TENANT_ID" ]]; then
  cat >&2 <<'USAGE'
usage: scripts/demo-issue-12.sh <tenant_uuid>

Run after:
  make dev
  make migrate-up
  make seed-dev

Pass the tenant UUID printed or visible from the seeded local database.
USAGE
  exit 2
fi

echo "== Vigia issue #12 demo =="
echo "tenant: $TENANT_ID"
echo

echo "1/3 Verify append-only evidence chain"
go run ./cmd/ledger-verify -tenant-id "$TENANT_ID"
echo

echo "2/3 Create RFC3161 timestamped Merkle checkpoint"
go run ./cmd/ledger-checkpoint -tenant-id "$TENANT_ID" -tsa-url "$TSA_URL"
echo

echo "3/3 Verify chain after checkpoint"
go run ./cmd/ledger-verify -tenant-id "$TENANT_ID"
echo

echo "Demo complete. Open the console Cost & Quality page after starting cmd/api + apps/console."
