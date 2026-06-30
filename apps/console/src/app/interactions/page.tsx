// Force dynamic rendering so this page is never pre-rendered at build time.
// The API key and live data are only available at request time.
export const dynamic = "force-dynamic";

import { listInteractions } from "@/lib/api";

export default async function InteractionsPage() {
  const interactions = await listInteractions();

  return (
    <main className="p-8">
      <h1 className="text-2xl font-semibold mb-6">Interactions</h1>
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
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
