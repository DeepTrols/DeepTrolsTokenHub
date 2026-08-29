import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { TrendingDown, TrendingUp, Trophy } from "lucide-react";
import { publicApi } from "../lib/api";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import "../i18n";
import { useTranslation } from "react-i18next";

export type RankingPeriod = "today" | "week" | "month" | "year";

export interface RankedModel {
  rank: number;
  previous_rank?: number;
  model_name: string;
  vendor: string;
  category: string;
  total_tokens: number;
  share: number;
  growth_pct: number;
}

export interface RankedVendor {
  rank: number;
  vendor: string;
  total_tokens: number;
  share: number;
  growth_pct: number;
  models_count: number;
  top_model: string;
}

export interface Mover {
  model_name: string;
  vendor: string;
  rank_delta: number;
  current_rank: number;
  growth_pct: number;
}

export interface HistoryPoint {
  ts: string;
  label: string;
  model: string;
  vendor: string;
  tokens: number;
}

export interface HistorySeries {
  points: HistoryPoint[];
  models: Array<{ name: string; vendor: string; total: number }>;
  buckets: number;
}

export interface VendorShareSeries {
  points: Array<{ ts: string; label: string; vendor: string; share: number; tokens: number }>;
  vendors: Array<{ name: string; total: number; share: number }>;
  buckets: number;
}

export interface RankingsSnapshot {
  models: RankedModel[];
  vendors: RankedVendor[];
  top_movers: Mover[];
  top_droppers: Mover[];
  models_history: HistorySeries;
  vendor_share_history: VendorShareSeries;
}

const PERIODS: Array<{ id: RankingPeriod; label: string }> = [
  { id: "today", label: "rankings.periodToday" },
  { id: "week", label: "rankings.periodWeek" },
  { id: "month", label: "rankings.periodMonth" },
  { id: "year", label: "rankings.periodYear" },
];

const CHART_COLORS = ["#4F6BED", "#0FA88B", "#D3A94E", "#8B6FE8", "#E5484D", "#35A7FF", "#FF9F1C", "#7C9885", "#B56576", "#6D597A"];

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function fmtPct(v: number): string {
  const s = v > 0 ? `+${v.toFixed(1)}%` : `${v.toFixed(1)}%`;
  return s;
}

function growthClass(v: number): string {
  return v > 0 ? "text-[#0C7A55]" : v < 0 ? "text-[#C4372C]" : "text-[#8C93A1]";
}

/** Flattens models_history points into a per-bucket stacked series for charts. */
function toStackedSeries(history: HistorySeries): Array<Record<string, number | string>> {
  const byLabel = new Map<string, Record<string, number | string>>();
  for (const p of history.points) {
    const row = byLabel.get(p.label) ?? { label: p.label };
    row[p.model] = ((row[p.model] as number) ?? 0) + p.tokens;
    byLabel.set(p.label, row);
  }
  return [...byLabel.values()];
}

/** Flattens vendor_share_history points into per-vendor share columns. */
function toVendorShareSeries(history: VendorShareSeries): Array<Record<string, number | string>> {
  const byLabel = new Map<string, Record<string, number | string>>();
  for (const p of history.points) {
    const row = byLabel.get(p.label) ?? { label: p.label };
    row[p.vendor] = p.share;
    byLabel.set(p.label, row);
  }
  return [...byLabel.values()];
}

export default function Rankings() {
  const { t } = useTranslation();
  const [period, setPeriod] = useState<RankingPeriod>("week");
  const query = useQuery({
    queryKey: ["public", "rankings", period],
    queryFn: () => publicApi.get<RankingsSnapshot>(`/rankings?period=${period}`),
    staleTime: 5 * 60_000,
  });

  const data = query.data;
  const stacked = data ? toStackedSeries(data.models_history) : [];
  const modelNames = data?.models_history.models.map((m) => m.name) ?? [];
  const vendorStacked = data ? toVendorShareSeries(data.vendor_share_history) : [];
  const vendorNames = data?.vendor_share_history.vendors.map((v) => v.name) ?? [];

  return (
    <div>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("rankings.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("rankings.subtitle")}</p>
        </div>
        <div className="glass-soft flex rounded-xl p-1 gap-1">
          {PERIODS.map((p) => (
            <button
              key={p.id}
              onClick={() => setPeriod(p.id)}
              className={`px-3.5 py-1.5 rounded-lg text-[13px] font-semibold transition-all ${
                period === p.id
                  ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white shadow-[0_8px_20px_rgba(79,107,237,0.3)]"
                  : "text-[#5C6472] hover:text-[#161A23]"
              }`}
            >
              {t(p.label)}
            </button>
          ))}
        </div>
      </div>

      {query.isLoading ? (
        <LoadingState message={t("rankings.loading")} />
      ) : query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} title={t("rankings.loadFailed")} />
      ) : !data || data.models.length === 0 ? (
        <EmptyState title={t("rankings.empty")} description={t("rankings.emptyDesc")} />
      ) : (
        <div className="space-y-4">
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <div className="glass rounded-[22px] p-5 lg:col-span-2">
              <h3 className="font-display font-semibold mb-4 flex items-center gap-2">
                <Trophy size={17} className="text-[#D3A94E]" />
                {t("rankings.modelTop", { count: data.models.length })}
              </h3>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-[11.5px] font-semibold uppercase tracking-[0.08em] text-[#8C93A1] border-b border-black/5">
                      <th className="py-2 pr-2">#</th>
                      <th className="py-2 pr-2">{t("rankings.thModel")}</th>
                      <th className="py-2 pr-2">{t("rankings.thVendor")}</th>
                      <th className="py-2 pr-2 text-right">{t("rankings.thToken")}</th>
                      <th className="py-2 pr-2 w-[160px]">{t("rankings.thShare")}</th>
                      <th className="py-2 pr-2 text-right">{t("rankings.thChange")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.models.map((m) => {
                      const prev = m.previous_rank;
                      const delta = prev !== undefined ? prev - m.rank : undefined;
                      return (
                        <tr key={m.model_name} className="border-b border-black/[0.04]">
                          <td className="py-2.5 pr-2 text-[#5C6472] font-semibold">{m.rank}</td>
                          <td className="py-2.5 pr-2 font-mono text-[12.5px]">{m.model_name}</td>
                          <td className="py-2.5 pr-2 text-[#5C6472]">{m.vendor}</td>
                          <td className="py-2.5 pr-2 text-right font-medium">{fmtTokens(m.total_tokens)}</td>
                          <td className="py-2.5 pr-2">
                            <div className="h-1.5 rounded-full bg-black/5 overflow-hidden">
                              <div
                                className="h-full rounded-full bg-gradient-to-r from-[#4F6BED] to-[#8B6FE8]"
                                style={{ width: `${Math.max(2, Math.min(100, m.share * 100))}%` }}
                              />
                            </div>
                          </td>
                          <td className="py-2.5 text-right">
                            <span className={`text-[12px] font-semibold ${growthClass(m.growth_pct)}`}>{fmtPct(m.growth_pct)}</span>
                            {delta !== undefined && delta !== 0 && (
                              <span className={`ml-1.5 text-[11px] ${delta > 0 ? "text-[#0C7A55]" : "text-[#C4372C]"}`}>
                                {delta > 0 ? "↑" : "↓"}{Math.abs(delta)}
                              </span>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="glass rounded-[22px] p-5">
              <h3 className="font-display font-semibold mb-4">{t("rankings.vendorShare")}</h3>
              <div className="space-y-3.5">
                {data.vendors.map((v) => (
                  <div key={v.vendor}>
                    <div className="flex items-center justify-between text-[13px] mb-1">
                      <span className="font-medium">{v.vendor}</span>
                      <span className="text-[#5C6472]">{fmtTokens(v.total_tokens)} · {(v.share * 100).toFixed(1)}%</span>
                    </div>
                    <div className="h-2 rounded-full bg-black/5 overflow-hidden">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-[#0FA88B] to-[#35A7FF]"
                        style={{ width: `${Math.max(2, Math.min(100, v.share * 100))}%` }}
                      />
                    </div>
                    <div className="mt-1 text-[11px] text-[#8C93A1]">
                      {t("rankings.modelsCount", { count: v.models_count })} · {t("rankings.topModel", { name: v.top_model })} · <span className={growthClass(v.growth_pct)}>{fmtPct(v.growth_pct)}</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className="glass rounded-[22px] p-5">
            <h3 className="font-display font-semibold mb-4">{t("rankings.tokenTrend")}</h3>
            {stacked.length === 0 ? (
              <p className="text-sm text-[#8C93A1] py-8 text-center">{t("rankings.noBucketData")}</p>
            ) : (
              <ResponsiveContainer width="100%" height={260}>
                <BarChart data={stacked} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="rgba(0,0,0,0.06)" />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
                  <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => fmtTokens(v)} />
                  <Tooltip formatter={(value) => fmtTokens(Number(value))} />
                  <Legend wrapperStyle={{ fontSize: 11 }} />
                  {modelNames.map((name, i) => (
                    <Bar key={name} dataKey={name} stackId="tokens" fill={CHART_COLORS[i % CHART_COLORS.length]} />
                  ))}
                </BarChart>
              </ResponsiveContainer>
            )}
          </div>

          {(data.top_movers.length > 0 || data.top_droppers.length > 0) && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="glass rounded-[22px] p-5">
                <h3 className="font-display font-semibold mb-3 flex items-center gap-2 text-[#0C7A55]">
                  <TrendingUp size={16} />
                  {t("rankings.rising")}
                </h3>
                <div className="space-y-2">
                  {data.top_movers.map((m) => (
                    <div key={m.model_name} className="flex items-center justify-between text-[13px]">
                      <span className="font-mono text-[12px]">{m.model_name}</span>
                      <span className="text-[#5C6472]">
                        <span className="text-[#0C7A55] font-semibold">↑{m.rank_delta}</span> · #{m.current_rank} · {fmtPct(m.growth_pct)}
                      </span>
                    </div>
                  ))}
                  {data.top_movers.length === 0 && <p className="text-[13px] text-[#8C93A1]">{t("rankings.noData")}</p>}
                </div>
              </div>
              <div className="glass rounded-[22px] p-5">
                <h3 className="font-display font-semibold mb-3 flex items-center gap-2 text-[#C4372C]">
                  <TrendingDown size={16} />
                  {t("rankings.falling")}
                </h3>
                <div className="space-y-2">
                  {data.top_droppers.map((m) => (
                    <div key={m.model_name} className="flex items-center justify-between text-[13px]">
                      <span className="font-mono text-[12px]">{m.model_name}</span>
                      <span className="text-[#5C6472]">
                        <span className="text-[#C4372C] font-semibold">↓{Math.abs(m.rank_delta)}</span> · #{m.current_rank} · {fmtPct(m.growth_pct)}
                      </span>
                    </div>
                  ))}
                  {data.top_droppers.length === 0 && <p className="text-[13px] text-[#8C93A1]">{t("rankings.noData")}</p>}
                </div>
              </div>
            </div>
          )}

          {vendorStacked.length > 0 && (
            <div className="glass rounded-[22px] p-5">
              <h3 className="font-display font-semibold mb-4">{t("rankings.vendorShareTrend")}</h3>
              <ResponsiveContainer width="100%" height={220}>
                <AreaChart data={vendorStacked} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="rgba(0,0,0,0.06)" />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
                  <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => `${(v * 100).toFixed(0)}%`} />
                  <Tooltip formatter={(value) => `${(Number(value) * 100).toFixed(1)}%`} />
                  <Legend wrapperStyle={{ fontSize: 11 }} />
                  {vendorNames.map((name, i) => (
                    <Area
                      key={name}
                      type="monotone"
                      dataKey={name}
                      name={name}
                      stackId="share"
                      stroke={CHART_COLORS[i % CHART_COLORS.length]}
                      fill={CHART_COLORS[i % CHART_COLORS.length]}
                      fillOpacity={0.35}
                    />
                  ))}
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
