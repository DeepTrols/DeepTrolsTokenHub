import { useEffect, useMemo, useState } from "react";
import { APIKeyData, UsageLog, WalletData } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import RangePicker from "../components/RangePicker";
import { PresetKey, gmt8DayKey, rangeForPreset } from "../lib/gmt8";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Download, MoreVertical, RotateCw } from "lucide-react";

const PALETTE = ["#4F6BED", "#0FA88B", "#8B6FE8", "#D3A94E", "#E5484D", "#12A5B0", "#C9A96A"];

export { gmt8DayKey, gmt8DayStart, gmt8MonthStart, formatRangeLabel, rangeForPreset } from "../lib/gmt8";
export type { PresetKey } from "../lib/gmt8";

export interface UsageStats {
  cost: number;
  requests: number;
  tokens: number;
}

export interface DailyPoint {
  day: string;
  label: string;
  cost: number;
  requests: number;
  tokens: number;
  models: Record<string, number>;
  keys: Record<string, number>;
}

export function sumUsage(logs: UsageLog[]): UsageStats {
  let cost = 0;
  let requests = 0;
  let tokens = 0;
  for (const l of logs) {
    cost += parseFloat(l.cost || "0");
    requests += 1;
    tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
  }
  return { cost, requests, tokens };
}

export function aggregateDaily(logs: UsageLog[], from: Date, to: Date): DailyPoint[] {
  const days: DailyPoint[] = [];
  const cursor = new Date(from.getTime());
  while (cursor.getTime() <= to.getTime()) {
    const day = gmt8DayKey(cursor.toISOString());
    days.push({
      day,
      label: `${Number(day.slice(5, 7))}/${Number(day.slice(8, 10))}`,
      cost: 0,
      requests: 0,
      tokens: 0,
      models: {},
      keys: {},
    });
    cursor.setTime(cursor.getTime() + 24 * 60 * 60 * 1000);
  }
  const index = new Map(days.map((d, i) => [d.day, i]));
  for (const l of logs) {
    const i = index.get(gmt8DayKey(l.created_at));
    if (i === undefined) continue;
    const cost = parseFloat(l.cost || "0");
    const tokens = (l.input_tokens || 0) + (l.output_tokens || 0);
    const p = days[i];
    p.cost += cost;
    p.requests += 1;
    p.tokens += tokens;
    const model = l.model || "未知模型";
    p.models[model] = (p.models[model] || 0) + cost;
    const key = l.api_key_name || l.api_key_id || "未知";
    p.keys[key] = (p.keys[key] || 0) + cost;
  }
  return days;
}

export function topModelByCost(logs: UsageLog[]): string {
  const byModel: Record<string, number> = {};
  for (const l of logs) {
    const model = l.model || "未知模型";
    byModel[model] = (byModel[model] || 0) + parseFloat(l.cost || "0");
  }
  let best = "";
  let bestCost = 0;
  for (const [model, cost] of Object.entries(byModel)) {
    if (cost > bestCost) {
      best = model;
      bestCost = cost;
    }
  }
  return best;
}

function uniqueModels(daily: DailyPoint[]): string[] {
  const set = new Set<string>();
  for (const d of daily) for (const k of Object.keys(d.models)) set.add(k);
  return [...set];
}

function uniqueKeys(daily: DailyPoint[]): string[] {
  const set = new Set<string>();
  for (const d of daily) for (const k of Object.keys(d.keys)) set.add(k);
  return [...set];
}

function exportCSV(logs: UsageLog[]) {
  const header = ["日期(GMT+8)", "模型", "API Key", "请求ID", "状态", "输入Tokens", "输出Tokens", "费用(CNY)"];
  const rows = logs.map((l) => [
    gmt8DayKey(l.created_at),
    l.model,
    l.api_key_name || l.api_key_id || "",
    l.request_id,
    l.status,
    String(l.input_tokens || 0),
    String(l.output_tokens || 0),
    l.cost || "0",
  ]);
  const csv = [header, ...rows]
    .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(","))
    .join("\n");
  const blob = new Blob(["\ufeff" + csv], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "用量明细.csv";
  a.click();
  URL.revokeObjectURL(url);
}

export default function Dashboard() {
  const now = useMemo(() => new Date(), []);
  const [preset, setPreset] = useState<PresetKey>("7d");
  const [customRange, setCustomRange] = useState<{ from: Date; to: Date } | null>(null);
  const [apiKeyId, setApiKeyId] = useState("");
  const [chartGroup, setChartGroup] = useState<"model" | "apikey">("apikey");
  const [menuOpen, setMenuOpen] = useState(false);
  const [selectedModel, setSelectedModel] = useState("");

  const { data: wallet, isLoading: walletLoading, isError: walletError, refetch: refetchWallet } = useConsoleQuery<WalletData>("/wallet");
  const { data: keyData } = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const keys = keyData?.data ?? [];

  const range = useMemo(() => {
    if (preset === "custom" && customRange) return customRange;
    return rangeForPreset(preset, now);
  }, [preset, customRange, now]);

  const usagePath = useMemo(() => {
    const params = new URLSearchParams({
      from: range.from.toISOString(),
      to: range.to.toISOString(),
      limit: "10000",
    });
    if (apiKeyId) params.set("api_key_id", apiKeyId);
    return `/usage?${params.toString()}`;
  }, [range, apiKeyId]);

  const { data: usageData, isLoading: usageLoading, isError: usageError, refetch: refetchUsage } = useConsoleQuery<{ data: UsageLog[] }>(usagePath);
  const logs = usageData?.data ?? [];

  const stats = useMemo(() => sumUsage(logs), [logs]);
  const daily = useMemo(() => aggregateDaily(logs, range.from, range.to), [logs, range]);
  const modelList = useMemo(() => {
    const cost: Record<string, number> = {};
    for (const d of daily) for (const [m, c] of Object.entries(d.models)) cost[m] = (cost[m] || 0) + c;
    return uniqueModels(daily).sort((a, b) => (cost[b] || 0) - (cost[a] || 0));
  }, [daily]);
  useEffect(() => {
    if (!modelList.includes(selectedModel)) setSelectedModel(modelList[0] || "");
  }, [modelList, selectedModel]);
  const modelLogs = useMemo(
    () => (selectedModel ? logs.filter((l) => l.model === selectedModel) : []),
    [logs, selectedModel],
  );
  const modelStats = useMemo(() => sumUsage(modelLogs), [modelLogs]);
  const modelDaily = useMemo(() => aggregateDaily(modelLogs, range.from, range.to), [modelLogs, range]);
  const seriesNames = useMemo(
    () => (chartGroup === "model" ? uniqueModels(daily) : uniqueKeys(daily)),
    [daily, chartGroup],
  );

  const isLoading = walletLoading || usageLoading;
  const isError = walletError || usageError;

  if (isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">用量信息</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">所有日期均按 GMT+8 时间显示，数据可能有 5 分钟延迟。</p>
        </div>
        <div className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-12 text-center">
          <div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3" />
          <p className="text-muted-foreground">加载...</p>
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">用量信息</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">所有日期均按 GMT+8 时间显示，数据可能有 5 分钟延迟。</p>
        </div>
        <div className="rounded-lg bg-white border border-[#E5484D]/20 shadow-sm p-6 text-center">
          <p className="text-[#C4372C] mb-3">加载失败，请稍后重试</p>
          <button
            onClick={() => {
              refetchWallet();
              refetchUsage();
            }}
            className="rounded-lg bg-[#E5484D] px-4 py-2 text-sm font-semibold text-white hover:brightness-110"
          >
            重试
          </button>
        </div>
      </div>
    );
  }

  const statCards = [
    { label: "消费金额", value: `¥${formatAmount(stats.cost)} CNY` },
    { label: "API 请求次数", value: stats.requests.toLocaleString("en-US") },
    { label: "Tokens", value: stats.tokens.toLocaleString("en-US") },
  ];

  return (
    <div className="space-y-5">
      {/* 顶部标题区 */}
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">用量信息</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">所有日期均按 GMT+8 时间显示，数据可能有 5 分钟延迟。</p>
      </div>

      {/* 第一行：充值余额 + 累计消费金额 */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-5 flex items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-[13px] font-semibold text-[#5C6472]">充值余额</span>
            </div>
            <p className="font-mono text-[28px] font-semibold tracking-tight mt-2">
              ¥{formatAmount(wallet?.available)}{" "}
              <span className="text-[13px] font-sans font-normal text-[#5C6472]">CNY</span>
            </p>
          </div>
          <a href="/wallet" className="rounded-lg bg-neutral-900 text-white text-sm font-semibold px-5 py-2.5 hover:bg-neutral-800 shrink-0">
            去充值
          </a>
        </div>
        <div className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-5 flex items-center justify-between gap-4">
          <div>
            <span className="text-[13px] font-semibold text-[#5C6472]">累计消费金额</span>
            <p className="font-mono text-[28px] font-semibold tracking-tight mt-2">
              ¥{formatAmount(Math.abs(parseFloat(wallet?.total_charged || "0")))}{" "}
              <span className="text-[13px] font-sans font-normal text-[#5C6472]">CNY</span>
            </p>
          </div>
        </div>
      </div>

      {/* 筛选工具栏 */}
      <div className="flex flex-wrap items-center gap-3">
        <RangePicker
          from={range.from}
          to={range.to}
          preset={preset}
          now={now}
          onApply={({ from, to, preset: p }) => {
            if (p === "custom") setCustomRange({ from, to });
            setPreset(p);
          }}
        />
        <select
          value={apiKeyId}
          onChange={(e) => setApiKeyId(e.target.value)}
          className="glass-soft rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20"
        >
          <option value="">全部 API Key</option>
          {keys.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name || k.masked_key || k.id}
            </option>
          ))}
        </select>
        <button
          onClick={() => {
            setPreset("7d");
            setCustomRange(null);
            setApiKeyId("");
            setSelectedModel("");
          }}
          className="text-[13px] font-medium text-[#4F6BED] hover:underline"
        >
          清除筛选条件
        </button>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => exportCSV(logs)}
            className="rounded-lg bg-neutral-900 text-white text-sm font-semibold px-4 py-2 hover:bg-neutral-800 inline-flex items-center gap-1.5"
          >
            <Download size={14} />
            导出
          </button>
          <div className="relative">
            <button
              onClick={() => setMenuOpen((v) => !v)}
              className="p-2 rounded-lg hover:bg-black/5 text-[#5C6472]"
              aria-label="更多操作"
            >
              <MoreVertical size={16} />
            </button>
            {menuOpen && (
              <div className="absolute right-0 top-full mt-1 bg-white rounded-lg border border-black/10 shadow-lg py-1 z-20 min-w-[130px]">
                <button
                  onClick={() => {
                    exportCSV(logs);
                    setMenuOpen(false);
                  }}
                  className="w-full text-left px-3 py-1.5 text-[13px] hover:bg-black/5 inline-flex items-center gap-2"
                >
                  <Download size={13} />
                  导出 CSV
                </button>
                <button
                  onClick={() => {
                    refetchWallet();
                    refetchUsage();
                    setMenuOpen(false);
                  }}
                  className="w-full text-left px-3 py-1.5 text-[13px] hover:bg-black/5 inline-flex items-center gap-2"
                >
                  <RotateCw size={13} />
                  刷新数据
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* 第二行：3 个统计卡片 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {statCards.map((c) => (
          <div key={c.label} className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-5">
            <p className="text-[12px] font-semibold text-[#5C6472]">{c.label}</p>
            <p className="font-mono text-[26px] font-semibold tracking-tight mt-2">{c.value}</p>
          </div>
        ))}
      </div>

      {/* 大图表：消费金额柱状图 */}
      <div className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-5">
        <div className="flex items-center justify-between gap-3 flex-wrap mb-4">
          <h3 className="font-display text-[15px] font-bold">
            消费金额（CNY）
            <span className="ml-2 font-mono text-[#4F6BED]">¥{formatAmount(stats.cost)}</span>
          </h3>
          <div className="flex rounded-lg bg-black/5 p-0.5">
            {(["model", "apikey"] as const).map((g) => (
              <button
                key={g}
                onClick={() => setChartGroup(g)}
                className={`px-3 py-1 text-[13px] font-semibold rounded-md transition-colors ${
                  chartGroup === g ? "bg-white shadow-sm text-[#161A23]" : "text-[#5C6472] hover:text-[#161A23]"
                }`}
              >
                {g === "model" ? "模型" : "API Key"}
              </button>
            ))}
          </div>
        </div>
        {daily.length > 0 && seriesNames.length > 0 ? (
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={daily} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
              <CartesianGrid stroke="rgba(22,26,35,0.08)" vertical={false} />
              <XAxis dataKey="label" tick={{ fontSize: 12, fill: "#5C6472" }} axisLine={false} tickLine={false} />
              <YAxis
                tick={{ fontSize: 11, fill: "#5C6472" }}
                axisLine={false}
                tickLine={false}
                width={46}
                domain={[0, (max: number) => Math.ceil(max * 1.2)]}
              />
              <Tooltip
                cursor={{ fill: "rgba(79,107,237,0.06)" }}
                contentStyle={{
                  background: "rgba(255,255,255,0.95)",
                  border: "1px solid rgba(0,0,0,0.08)",
                  borderRadius: 8,
                  fontSize: 12.5,
                }}
                formatter={(v: number, name: string) => [`¥${formatAmount(v)}`, name]}
              />
              {seriesNames.map((name, i) => (
                <Bar
                  key={name}
                  dataKey={chartGroup === "model" ? `models.${name}` : `keys.${name}`}
                  name={name}
                  stackId="cost"
                  fill={PALETTE[i % PALETTE.length]}
                  radius={i === seriesNames.length - 1 ? [3, 3, 0, 0] : [0, 0, 0, 0]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        ) : (
          <div className="py-16 text-center text-[#5C6472]">暂无数据</div>
        )}
      </div>

      {/* 底部：按模型细分 */}
      {modelList.length > 0 && selectedModel ? (
        <div className="rounded-lg bg-white border border-black/[0.06] shadow-sm p-5">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="text-[13px] font-semibold text-[#5C6472]">按模型查看</span>
            <select
              aria-label="选择模型"
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              className="glass-soft rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20"
            >
              {modelList.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
            <div className="rounded-lg bg-[#F7F9FC] p-4">
              <h4 className="text-[13px] font-semibold text-[#5C6472]">
                API 请求次数{" "}
                <span className="ml-1 font-mono text-[#161A23]">{modelStats.requests.toLocaleString("en-US")}</span>
              </h4>
              <ResponsiveContainer width="100%" height={170}>
                <AreaChart data={modelDaily} margin={{ top: 8, right: 4, left: -18, bottom: 0 }}>
                  <defs>
                    <linearGradient id="reqGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#4F6BED" stopOpacity={0.28} />
                      <stop offset="100%" stopColor="#4F6BED" stopOpacity={0.03} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid stroke="rgba(22,26,35,0.08)" vertical={false} />
                  <XAxis dataKey="label" tick={{ fontSize: 11, fill: "#5C6472" }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11, fill: "#5C6472" }} axisLine={false} tickLine={false} width={46} />
                  <Tooltip
                    contentStyle={{
                      background: "rgba(255,255,255,0.95)",
                      border: "1px solid rgba(0,0,0,0.08)",
                      borderRadius: 8,
                      fontSize: 12.5,
                    }}
                    formatter={(v: number) => [v.toLocaleString("en-US"), "请求次数"]}
                  />
                  <Area type="monotone" dataKey="requests" stroke="#4F6BED" strokeWidth={2} fill="url(#reqGrad)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
            <div className="rounded-lg bg-[#F7F9FC] p-4">
              <h4 className="text-[13px] font-semibold text-[#5C6472]">
                Tokens{" "}
                <span className="ml-1 font-mono text-[#161A23]">{modelStats.tokens.toLocaleString("en-US")}</span>
              </h4>
              <ResponsiveContainer width="100%" height={170}>
                <BarChart data={modelDaily} margin={{ top: 8, right: 4, left: -18, bottom: 0 }}>
                  <CartesianGrid stroke="rgba(22,26,35,0.08)" vertical={false} />
                  <XAxis dataKey="label" tick={{ fontSize: 11, fill: "#5C6472" }} axisLine={false} tickLine={false} />
                  <YAxis
                    tick={{ fontSize: 11, fill: "#5C6472" }}
                    axisLine={false}
                    tickLine={false}
                    width={52}
                    tickFormatter={(v: number) => (v >= 1_000_000 ? `${Math.round(v / 1_000_000)}M` : v >= 1000 ? `${Math.round(v / 1000)}K` : String(v))}
                  />
                  <Tooltip
                    cursor={{ fill: "rgba(79,107,237,0.06)" }}
                    contentStyle={{
                      background: "rgba(255,255,255,0.95)",
                      border: "1px solid rgba(0,0,0,0.08)",
                      borderRadius: 8,
                      fontSize: 12.5,
                    }}
                    formatter={(v: number) => [v.toLocaleString("en-US"), "Tokens"]}
                  />
                  <Bar dataKey="tokens" fill="#8FB0F5" radius={[3, 3, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
