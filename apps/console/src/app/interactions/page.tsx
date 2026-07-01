// Force dynamic rendering so this page is never pre-rendered at build time.
// The API key and live data are only available at request time.
export const dynamic = "force-dynamic";

import { getSummary, listInteractions } from "@/lib/api";

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

export default async function InteractionsPage() {
  const [interactions, summary] = await Promise.all([
    listInteractions(),
    getSummary(),
  ]);

  return (
    <main className="p-8">
      <h1 className="text-2xl font-semibold mb-4">Interactions</h1>

      {/* Out-of-hours tile: fed by the summary endpoint's SQL aggregate,
          never computed by summing rows rendered here. */}
      <div className="mb-6 inline-block rounded border border-slate-200 bg-slate-50 px-4 py-3">
        <div className="text-xs uppercase tracking-wide text-slate-500">
          Out-of-hours interactions
        </div>
        <div className="text-2xl font-semibold text-slate-800">
          {summary.out_of_hours_count}
        </div>
      </div>

      {interactions.length === 0 ? (
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
              </tr>
            </thead>
            <tbody>
              {interactions.map((interaction) => (
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
                  <td className="px-4 py-2 text-slate-600 text-xs max-w-xs truncate" title={interaction.reason ?? undefined}>
                    {interaction.reason ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
