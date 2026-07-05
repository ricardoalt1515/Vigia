// Force dynamic rendering so this page is never pre-rendered at build time.
// The API key and live data are only available at request time.
export const dynamic = "force-dynamic";

import { listByDespacho } from "@/lib/api";

function formatRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

export default async function ByDespachoPage() {
  const despachos = await listByDespacho();

  return (
    <main className="p-8">
      <h1 className="text-2xl font-semibold mb-4">Violation Rate by Despacho</h1>

      {despachos.length === 0 ? (
        <p className="text-slate-500">No despacho data found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full border border-slate-200 text-sm">
            <thead className="bg-slate-100">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Despacho
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Total Interactions
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Violations
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Violation Rate
                </th>
              </tr>
            </thead>
            <tbody>
              {despachos.map((despacho) => (
                <tr
                  key={despacho.despacho_id ?? "unattributed"}
                  className="border-b border-slate-100 hover:bg-slate-50"
                >
                  <td className="px-4 py-2 text-slate-700">
                    {despacho.despacho_id === null ? (
                      <span className="italic text-slate-400">
                        {despacho.despacho_name}
                      </span>
                    ) : (
                      despacho.despacho_name
                    )}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {despacho.total}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {despacho.violations}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {formatRate(despacho.violation_rate)}
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
