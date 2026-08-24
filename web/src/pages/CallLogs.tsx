import { EmptyState, ErrorState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useMemo, useState } from "react";
import { APIKeyData, UsageLog } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { ChevronDown, ChevronRight, Key } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";

interface ModelUsage {
  model: string;
  count: number;
  tokens: number;
  cost: number;
}

interface KeyUsage {
  id: string;
  name: string;
  count: number;
  tokens: number;
  cost: number;
  models: ModelUsage[];
}

type RangeKey = "today" | "7d" | "30d" | "all";

const RANGES: { key: RangeKey; label: string }[] = [
  { key: "today", label: "今天" },
  { key: "7d", label: "近7天" },
  { key: "30d", label: "近30天" },
  { key: "all", label: "全部" },
];

export default function CallLogs() {
  const [range, setRange] = useState<RangeKey>("all");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // Recompute the window only when the range changes. Reading a fresh "now"
  // during render would churn the query key every render and refetch forever.
  const usageUrl = useMemo(() => {
    const params = new URLSearchParams({ limit: "200" });
    if (range !== "all") {
      const now = new Date();
      const from =
        range === "today"
          ? new Date(now.getFullYear(), now.getMonth(), now.getDate())
          : new Date(now.getTime() - (range === "7d" ? 7 : 30) * 86_400_000);
      params.set("from", from.toISOString());
      params.set("to", now.toISOString());
    }
    return "/usage?" + params.toString();
  }, [range]);

  const { data: usage, isLoading, isError, error, refetch } =
    useConsoleQuery<{ data: UsageLog[] }>(usageUrl);
  const logs = useMemo(() => usage?.data ?? [], [usage]);
  const { data: keysData } = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const apiKeys = useMemo(() => keysData?.data ?? [], [keysData]);

  // Aggregate totals across every key touched by the fetched logs.
  const summary = useMemo(() => {
    let totalRequests = 0;
    let totalCost = 0;
    const models = new Set<string>();
    for (const l of logs) {
      totalRequests += 1;
      totalCost += parseFloat(l.cost || "0");
      models.add(l.model);
    }
    return { totalRequests, totalCost, modelCount: models.size };
  }, [logs]);

  // Rows = every user API key (zero-usage keys stay visible but greyed) merged
  // with per-model aggregates from the logs; active keys first, then by count.
  const rows: KeyUsage[] = useMemo(() => {
    type Acc = Omit<KeyUsage, "models"> & { models: Record<string, ModelUsage> };
    const acc: Record<string, Acc> = {};
    for (const l of logs) {
      const id = l.api_key_id || "";
      if (!acc[id]) acc[id] = { id, name: "", count: 0, tokens: 0, cost: 0, models: {} };
      const k = acc[id];
      if (!k.name && l.api_key_name) k.name = l.api_key_name;
      k.count += 1;
      k.tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
      k.cost += parseFloat(l.cost || "0");
      if (!k.models[l.model]) k.models[l.model] = { model: l.model, count: 0, tokens: 0, cost: 0 };
      const m = k.models[l.model];
      m.count += 1;
      m.tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
      m.cost += parseFloat(l.cost || "0");
    }

    const merged: Record<string, KeyUsage> = {};
    for (const ak of apiKeys) {
      merged[ak.id] = { id: ak.id, name: ak.name, count: 0, tokens: 0, cost: 0, models: [] };
    }
    for (const [id, k] of Object.entries(acc)) {
      const base = merged[id] ?? {
        id,
        name: id.slice(0, 8) + "…",
        count: 0,
        tokens: 0,
        cost: 0,
        models: [],
      };
      merged[id] = {
        ...base,
        name: base.name || k.name || id.slice(0, 8) + "…",
        count: k.count,
        tokens: k.tokens,
        cost: k.cost,
        models: Object.values(k.models).sort((a, b) => b.count - a.count),
      };
    }
    return Object.values(merged).sort(
      (a, b) =>
        (b.count > 0 ? 1 : 0) - (a.count > 0 ? 1 : 0) ||
        b.count - a.count ||
        a.name.localeCompare(b.name),
    );
  }, [logs, apiKeys]);

  if (isLoading) {
    return (
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight mb-6">调用记录</h2>
        <Card>
          <CardContent className="p-12 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3" />
            <p className="text-muted-foreground">加载调用记录...</p>
          </CardContent>
        </Card>
      </div>
    );
  }
  if (isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>调用记录</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <ErrorState error={error} onRetry={() => refetch()} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <div>
      <div className="flex items-end justify-between gap-3 mb-6 flex-wrap">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">调用记录</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">
            按 API 密钥查看调用量与模型分布 · 最多展示最近 200 条
          </p>
        </div>
        <div
          className="flex items-center gap-0.5 glass-soft rounded-full p-1"
          role="group"
          aria-label="时间范围"
        >
          {RANGES.map(r => (
            <button
              key={r.key}
              type="button"
              onClick={() => setRange(r.key)}
              aria-pressed={range === r.key}
              className={
                "px-3.5 py-1.5 text-sm rounded-full transition-all " +
                (range === r.key
                  ? "bg-white/85 text-foreground font-semibold shadow-[0_4px_14px_rgba(63,76,128,0.12)]"
                  : "text-muted-foreground hover:text-foreground")
              }
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        {[
          { label: "总请求", value: String(summary.totalRequests) },
          { label: "总费用", value: `${formatAmount(summary.totalCost)} CNY`, accent: true },
          { label: "涉及模型", value: String(summary.modelCount) },
          { label: "API 密钥", value: String(rows.length) },
        ].map(s => (
          <Card key={s.label}>
            <CardContent className="p-5">
              <p className="text-xs text-muted-foreground">{s.label}</p>
              <p className={"mt-1 font-mono text-[22px] font-semibold tracking-tight " + (s.accent ? "text-[#4F6BED]" : "")}>
                {s.value}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>

      {rows.length === 0 && (
        <Card>
          <CardContent className="p-12">
            <EmptyState title="暂无调用记录" />
          </CardContent>
        </Card>
      )}

      <div className="space-y-3">
        {rows.map(k => {
          const isExpanded = expandedId === k.id;
          const hasUsage = k.count > 0;
          return (
            <Card key={k.id || k.name} className={"overflow-hidden " + (hasUsage ? "" : "opacity-70")}>
              <button
                type="button"
                disabled={!hasUsage}
                aria-expanded={isExpanded}
                onClick={() => setExpandedId(isExpanded ? null : k.id)}
                className={
                  "w-full flex items-center justify-between gap-4 px-5 py-4 text-left transition-colors " +
                  (hasUsage ? "cursor-pointer hover:bg-muted/40" : "cursor-default")
                }
              >
                <span className="flex items-center gap-2.5 min-w-0">
                  <Key size={16} className="text-muted-foreground shrink-0" />
                  <span className="font-medium truncate">{k.name}</span>
                </span>
                <span className="flex items-center gap-4 shrink-0">
                  {hasUsage ? (
                    <span className="text-sm text-muted-foreground whitespace-nowrap">
                      {k.count} 次调用 · {k.tokens.toLocaleString()} tokens ·{" "}
                      {formatAmount(k.cost)} CNY
                    </span>
                  ) : (
                    <span className="text-sm text-muted-foreground/60 whitespace-nowrap">
                      暂无调用
                    </span>
                  )}
                  {hasUsage &&
                    (isExpanded ? (
                      <ChevronDown size={16} className="text-muted-foreground" />
                    ) : (
                      <ChevronRight size={16} className="text-muted-foreground" />
                    ))}
                </span>
              </button>

              {hasUsage && isExpanded && (
                <div className="border-t px-5 py-2">
                  {k.models.map(m => (
                    <div key={m.model} className="flex items-center justify-between gap-4 py-2 text-sm">
                      <span className="text-muted-foreground truncate">{m.model}</span>
                      <span className="text-muted-foreground/70 whitespace-nowrap">
                        {m.count} 次 · {m.tokens.toLocaleString()} tokens ·{" "}
                        {formatAmount(m.cost)} CNY
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          );
        })}
      </div>
    </div>
  );
}
