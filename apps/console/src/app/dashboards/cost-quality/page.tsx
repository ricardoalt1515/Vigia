export const dynamic = "force-dynamic";

import { getCostQuality } from "@/lib/api";

function formatRate(numerator: number, denominator: number) {
  if (denominator === 0) return "0.0%";
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

export default async function CostQualityPage() {
  const summary = await getCostQuality();

  return (
    <main className="p-6">
      <h1 className="text-2xl font-semibold mb-2">Cost and Quality</h1>
      <p className="text-sm text-slate-500 mb-6">
        Tenant-scoped GenAI usage and quality signals from judged interactions.
      </p>

      <section className="grid gap-4 md:grid-cols-3">
        <MetricCard label="Judged interactions" value={summary.judged_interactions} />
        <MetricCard label="Billable input tokens" value={summary.billable_input_tokens} />
        <MetricCard label="Output tokens" value={summary.output_tokens} />
        <MetricCard label="Cache reads" value={summary.cache_read_input_tokens} />
        <MetricCard label="HITL rate" value={formatRate(summary.hitl_required, summary.judged_interactions)} />
        <MetricCard label="Average confidence" value={summary.average_confidence.toFixed(4)} />
      </section>
    </main>
  );
}

function MetricCard({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div className="text-sm text-slate-500">{label}</div>
      <div className="mt-2 text-2xl font-semibold text-slate-900">{value}</div>
    </div>
  );
}
