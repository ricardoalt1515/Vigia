// Force dynamic rendering so this page is never pre-rendered at build time.
// The API key and live data are only available at request time.
export const dynamic = "force-dynamic";

import { getSummary, listInteractions } from "@/lib/api";
import { InteractionsTable } from "./InteractionsTable";

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

      <InteractionsTable interactions={interactions} />
    </main>
  );
}
