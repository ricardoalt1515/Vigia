# Business-Day Calendar Specification

## Purpose

Static, versioned Mexican statutory-holiday calendar (LFT Art. 74) plus weekends,
used to compute the 10-business-day SLA deadline for complaint cases. Legally
load-bearing; ambiguity MUST resolve toward the earlier deadline rather than
silently granting extra time.

## Requirements

### Requirement: Static Versioned Holiday Table

The system MUST seed a `business_day_holidays` table via migration containing the
LFT Art. 74 statutory holidays, versioned so future updates are auditable rather
than silently mutated in place.

#### Scenario: Holiday table is seeded on migration

- GIVEN the migration runs
- WHEN the database is queried
- THEN `business_day_holidays` contains the current LFT Art. 74 statutory holidays
  for the seeded calendar version

#### Scenario: Table is documented as pending counsel confirmation

- GIVEN the holiday table is legally load-bearing
- WHEN the migration or accompanying documentation is reviewed
- THEN it MUST include an explicit pending-counsel-confirmation note, anchored to the
  "Open legal items to confirm with counsel" section of docs/regulatory-ruleset.md
  (MX-REDECO-04A is a documentation-style precedent, not a substantive dependency)

### Requirement: Business-Day Deadline Computation

The system MUST compute the SLA due date by advancing 10 business days from the
complaint's creation timestamp, excluding weekends and seeded statutory holidays.

#### Scenario: Deadline skips a weekend

- GIVEN a complaint opens on a Thursday
- WHEN the 10-business-day deadline is computed
- THEN Saturday and Sunday occurring within the count are excluded from the
  business-day count

#### Scenario: Deadline skips a seeded holiday

- GIVEN a complaint opens such that a seeded statutory holiday falls within the
  10-business-day window
- WHEN the deadline is computed
- THEN the holiday date is excluded from the business-day count and the deadline
  falls on a later calendar date than it would without the holiday

#### Scenario: Deadline computation across weekends and holidays combined

- GIVEN a complaint opens such that both weekends and one or more seeded holidays
  fall within the counting window
- WHEN the deadline is computed
- THEN both weekends and holidays are excluded from the business-day count in the
  same computation, yielding a single unambiguous `sla_due_at`

### Requirement: Fail-Closed Ambiguity Resolution

The system MUST resolve any doubtful or unconfirmed calendar day toward counting it
as a business day, so the computed SLA deadline is never later than the true legal
deadline.

#### Scenario: Doubtful day counts as a business day

- GIVEN a calendar day's holiday status is unconfirmed or ambiguous per the
  pending-counsel-confirmation note
- WHEN the deadline is computed
- THEN that day is treated as a business day (counted toward the 10-day total)
- AND the resulting deadline is the earlier of the possible interpretations
