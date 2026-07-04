"use client";

import { useState } from "react";
import type { Interaction } from "@/lib/api";

function OutcomeBadge({ outcome }: { outcome: string | null }) {
  if (outcome === null) {
    return <span className="text-slate-400">—</span>;
  }
  const isBlock = outcome === "BLOCK";
  return (
    <span
      className={
        isBlock
          ? "inline-block rounded px-2 py-0.5 text-xs font-semibold bg-red-100 text-red-700"
          : "inline-block rounded px-2 py-0.5 text-xs font-semibold bg-green-100 text-green-700"
      }
    >
      {outcome}
    </span>
  );
}

// FlagsBadges renders a red THREAT badge when threat_flagged and an amber
// HITL badge when requires_hitl. Both are nil-safe: an unevaluated
// interaction (both fields null) renders no badges, never a fabricated
// "not flagged" state.
function FlagsBadges({ interaction }: { interaction: Interaction }) {
  const badges: React.ReactNode[] = [];
  if (interaction.threat_flagged) {
    badges.push(
      <span
        key="threat"
        className="inline-block rounded px-2 py-0.5 text-xs font-semibold bg-red-100 text-red-700"
      >
        THREAT
      </span>,
    );
  }
  if (interaction.requires_hitl) {
    badges.push(
      <span
        key="hitl"
        className="inline-block rounded px-2 py-0.5 text-xs font-semibold bg-amber-100 text-amber-800"
      >
        HITL
      </span>,
    );
  }
  if (badges.length === 0) {
    return <span className="text-slate-400">—</span>;
  }
  return <div className="flex gap-1">{badges}</div>;
}

function isFlagged(interaction: Interaction): boolean {
  return Boolean(interaction.threat_flagged || interaction.requires_hitl);
}

// PolicyBundleVersionCell distinguishes null (no evaluation row exists yet)
// from an empty string (an evaluation ran but no PolicyBundle was active at
// the time) — both render as a dash-like placeholder, but with different
// text so the state is still visible on hover/inspection, never collapsing
// the two into one indistinguishable "—" (issue #6).
function PolicyBundleVersionCell({
  version,
}: {
  version: string | null;
}) {
  if (version === null) {
    return <span className="text-slate-400">—</span>;
  }
  if (version === "") {
    return (
      <span className="text-slate-400" title="Evaluated with no active policy bundle">
        (none)
      </span>
    );
  }
  return <span className="font-mono text-xs text-slate-700">{version}</span>;
}

export function InteractionsTable({
  interactions,
}: {
  interactions: Interaction[];
}) {
  const [showOnlyFlagged, setShowOnlyFlagged] = useState(false);

  const visible = showOnlyFlagged
    ? interactions.filter(isFlagged)
    : interactions;

  return (
    <div>
      <div className="mb-3 flex items-center gap-2">
        <label className="flex items-center gap-2 text-sm text-slate-700">
          <input
            type="checkbox"
            checked={showOnlyFlagged}
            onChange={(e) => setShowOnlyFlagged(e.target.checked)}
          />
          Show only flagged
        </label>
      </div>

      {visible.length === 0 ? (
        <p className="text-slate-500">No interactions found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full border border-slate-200 text-sm">
            <thead className="bg-slate-100">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  ID
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Occurred At
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Channel
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Direction
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Outcome
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Reason
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Flags
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Bundle Version
                </th>
              </tr>
            </thead>
            <tbody>
              {visible.map((interaction) => (
                <tr
                  key={interaction.id}
                  className="border-b border-slate-100 hover:bg-slate-50"
                >
                  <td className="px-4 py-2 font-mono text-xs text-slate-600">
                    {interaction.id}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {interaction.occurred_at}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {interaction.channel}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {interaction.direction}
                  </td>
                  <td className="px-4 py-2">
                    <OutcomeBadge outcome={interaction.outcome} />
                  </td>
                  <td
                    className="px-4 py-2 text-slate-600 text-xs max-w-xs truncate"
                    title={interaction.reason ?? undefined}
                  >
                    {interaction.reason ?? "—"}
                  </td>
                  <td className="px-4 py-2">
                    <FlagsBadges interaction={interaction} />
                  </td>
                  <td className="px-4 py-2">
                    <PolicyBundleVersionCell
                      version={interaction.policy_bundle_version}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
