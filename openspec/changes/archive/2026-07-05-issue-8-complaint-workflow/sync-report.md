# Sync Report: issue-8-complaint-workflow

## Status

PASS

## Summary

Canonical OpenSpec specs were created for the two change domains and synced from the active change specs:

- `openspec/specs/business-day-calendar/spec.md`
- `openspec/specs/complaint-workflow/spec.md`

No existing canonical spec blocks were present for either domain, so this was a full-spec creation sync.

## Synced domains

### business-day-calendar

Created full canonical spec from the change spec.

Requirements:
- Static Versioned Holiday Table
- Business-Day Deadline Computation
- Fail-Closed Ambiguity Resolution

### complaint-workflow

Created full canonical spec from the change spec.

Requirements:
- Case State Machine
- Idempotent Case Creation
- Poll-Triggered Review Request
- SLA Poll and Escalation
- HITL Resume via Poll
- Idempotent Transitions
- Atomic Evidence Append with State Transition
- Tenant Isolation

## Warnings

- None.
- No destructive merge was needed.
- No other active change under `openspec/changes/*/specs/{domain}/spec.md` touched these domains.
