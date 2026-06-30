import "server-only";

// Canonical shape from GET /v1/interactions (internal/httpapi/httpapi.go):
//   { "interactions": [ { "id", "occurred_at", "channel", "direction" }, ... ] }
// The loader also tolerates a bare array for forward compatibility.
export type Interaction = {
  id: string;
  occurred_at: string;
  channel: string;
  direction: string;
};

export async function listInteractions(): Promise<Interaction[]> {
  const base = process.env.VIGIA_API_BASE_URL ?? "http://localhost:8080";
  const key = process.env.VIGIA_API_KEY;
  if (!key) {
    throw new Error(
      "VIGIA_API_KEY environment variable is not set. " +
        "Copy .env.example to .env.local and set the value.",
    );
  }

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
