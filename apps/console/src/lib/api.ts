import "server-only";

// Canonical shape from GET /v1/interactions (internal/httpapi/httpapi.go):
//   { "interactions": [ { "id", "occurred_at", "channel", "direction",
//                          "outcome", "reason", "requires_hitl",
//                          "threat_flagged", "policy_bundle_version" }, ... ] }
// outcome/reason/requires_hitl/threat_flagged are null when the interaction
// has not yet been evaluated — the API never fabricates a PASS/BLOCK
// outcome or a false flag.
// policy_bundle_version follows the same convention but with a third,
// distinct state: null means no evaluation row exists yet; an empty string
// means an evaluation ran but no PolicyBundle was active at the time; any
// other string is the real stamped bundle version (issue #6).
// The loader also tolerates a bare array for forward compatibility.
export type Interaction = {
  id: string;
  occurred_at: string;
  channel: string;
  direction: string;
  outcome: string | null;
  reason: string | null;
  requires_hitl: boolean | null;
  threat_flagged: boolean | null;
  policy_bundle_version: string | null;
};

function apiConfig() {
  const base = process.env.VIGIA_API_BASE_URL ?? "http://localhost:8080";
  const key = process.env.VIGIA_API_KEY;
  if (!key) {
    throw new Error(
      "VIGIA_API_KEY environment variable is not set. " +
        "Copy .env.example to .env.local and set the value.",
    );
  }
  return { base, key };
}

export async function listInteractions(): Promise<Interaction[]> {
  const { base, key } = apiConfig();

  const res = await fetch(`${base}/v1/interactions`, {
    headers: {
      Authorization: `Bearer ${key}`,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    throw new Error(
      `GET /v1/interactions failed: ${res.status} ${res.statusText}`,
    );
  }

  const body: unknown = await res.json();

  // Canonical envelope: { "interactions": [...] }
  if (
    body !== null &&
    typeof body === "object" &&
    "interactions" in body &&
    Array.isArray((body as { interactions: unknown }).interactions)
  ) {
    return (body as { interactions: Interaction[] }).interactions;
  }

  // Tolerate bare array
  if (Array.isArray(body)) {
    return body as Interaction[];
  }

  return [];
}

// Canonical shape from GET /v1/summary (internal/httpapi/httpapi.go):
//   { "out_of_hours_count": <int> }
export type Summary = {
  out_of_hours_count: number;
};

export async function getSummary(): Promise<Summary> {
  const { base, key } = apiConfig();

  const res = await fetch(`${base}/v1/summary`, {
    headers: {
      Authorization: `Bearer ${key}`,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    throw new Error(`GET /v1/summary failed: ${res.status} ${res.statusText}`);
  }

  const body: unknown = await res.json();
  if (
    body !== null &&
    typeof body === "object" &&
    "out_of_hours_count" in body &&
    typeof (body as { out_of_hours_count: unknown }).out_of_hours_count ===
      "number"
  ) {
    return body as Summary;
  }

  return { out_of_hours_count: 0 };
}

// Canonical shape from GET /v1/dashboards/by-despacho
// (internal/httpapi/httpapi.go):
//   { "despachos": [ { "despacho_id", "despacho_name", "total",
//                       "violations", "violation_rate" }, ... ] }
// despacho_id is null for the synthetic "unattributed" bucket, which
// covers interactions with no despacho FK set.
export type DespachoRate = {
  despacho_id: string | null;
  despacho_name: string;
  total: number;
  violations: number;
  violation_rate: number;
};

export async function listByDespacho(): Promise<DespachoRate[]> {
  const { base, key } = apiConfig();

  const res = await fetch(`${base}/v1/dashboards/by-despacho`, {
    headers: {
      Authorization: `Bearer ${key}`,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    throw new Error(
      `GET /v1/dashboards/by-despacho failed: ${res.status} ${res.statusText}`,
    );
  }

  const body: unknown = await res.json();

  if (
    body !== null &&
    typeof body === "object" &&
    "despachos" in body &&
    Array.isArray((body as { despachos: unknown }).despachos)
  ) {
    return (body as { despachos: DespachoRate[] }).despachos;
  }

  return [];
}

// Canonical shape from GET /v1/dashboards/by-cause
// (internal/httpapi/httpapi.go):
//   { "causes": [ { "rule_code", "violations", "warnings" }, ... ] }
// warnings is a separate count of outcome = 'warn' rows (non-zero in
// practice only for MX-REDECO-03) and is never folded into violations.
export type CauseCount = {
  rule_code: string;
  violations: number;
  warnings: number;
};

export async function listByCause(): Promise<CauseCount[]> {
  const { base, key } = apiConfig();

  const res = await fetch(`${base}/v1/dashboards/by-cause`, {
    headers: {
      Authorization: `Bearer ${key}`,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    throw new Error(
      `GET /v1/dashboards/by-cause failed: ${res.status} ${res.statusText}`,
    );
  }

  const body: unknown = await res.json();

  if (
    body !== null &&
    typeof body === "object" &&
    "causes" in body &&
    Array.isArray((body as { causes: unknown }).causes)
  ) {
    return (body as { causes: CauseCount[] }).causes;
  }

  return [];
}

export type CostQualitySummary = {
  judged_interactions: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_input_tokens: number;
  cache_creation_input_tokens: number;
  billable_input_tokens: number;
  hitl_required: number;
  failed_interactions: number;
  average_confidence: number;
};

export async function getCostQuality(): Promise<CostQualitySummary> {
  const { base, key } = apiConfig();

  const res = await fetch(`${base}/v1/dashboards/cost-quality`, {
    headers: {
      Authorization: `Bearer ${key}`,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    throw new Error(
      `GET /v1/dashboards/cost-quality failed: ${res.status} ${res.statusText}`,
    );
  }

  const body: unknown = await res.json();
  if (body !== null && typeof body === "object" && "summary" in body) {
    return (body as { summary: CostQualitySummary }).summary;
  }

  return {
    judged_interactions: 0,
    input_tokens: 0,
    output_tokens: 0,
    cache_read_input_tokens: 0,
    cache_creation_input_tokens: 0,
    billable_input_tokens: 0,
    hitl_required: 0,
    failed_interactions: 0,
    average_confidence: 0,
  };
}
