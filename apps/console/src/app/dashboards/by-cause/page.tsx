// Force dynamic rendering so this page is never pre-rendered at build time.
// The API key and live data are only available at request time.
export const dynamic = "force-dynamic";

import { listByCause } from "@/lib/api";

export default async function ByCausePage() {
  const causes = await listByCause();

  return (
    <main className="p-8">
      <h1 className="text-2xl font-semibold mb-4">Violations by REDECO Cause</h1>

      {causes.length === 0 ? (
        <p className="text-slate-500">No cause data found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full border border-slate-200 text-sm">
            <thead className="bg-slate-100">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Rule Code
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Violations
                </th>
                <th className="px-4 py-2 text-left font-medium text-slate-700 border-b border-slate-200">
                  Warnings
                </th>
              </tr>
            </thead>
            <tbody>
              {causes.map((cause) => (
                <tr
                  key={cause.rule_code}
                  className="border-b border-slate-100 hover:bg-slate-50"
                >
                  <td className="px-4 py-2 font-mono text-xs text-slate-700">
                    {cause.rule_code}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {cause.violations}
                  </td>
                  <td className="px-4 py-2 text-slate-700">
                    {cause.warnings}
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
