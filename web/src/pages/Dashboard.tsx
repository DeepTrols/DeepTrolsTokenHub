import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { useMemo } from "react";
import { WalletData, UsageLog } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { DollarSign, Activity, Zap, AlertTriangle, Plus } from "lucide-react";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const MODEL_DOTS: Record<string, string> = {
  gpt: "bg-[#E5484D]",
  claude: "bg-[#D3A94E]",
  deep: "bg-[#0FA88B]",
  qwen: "bg-[#8B6FE8]",
  glm: "bg-[#C9A96A]",
};

export default function Dashboard() {
  const { data: wallet, isLoading: wl, isError: we, error: weMsg, refetch: wr } = useConsoleQuery<WalletData>("/wallet");
  const { data: usage, isLoading: ul, isError: ue, error: ueMsg, refetch: ur } = useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=200");
  const logs = usage?.data ?? [];
  const isLoading = wl || ul;
  const stats = useMemo(() => {
    const today = new Date().toISOString().slice(0,10);
    const todayLogs = logs.filter(l=>l.created_at.startsWith(today));
    const byDay: Record<string, { tokens: number; cost: number }> = {};
    for (const l of logs) {
      const day = l.created_at.slice(0, 10);
      if (!byDay[day]) byDay[day] = { tokens: 0, cost: 0 };
      byDay[day].tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
      byDay[day].cost += parseFloat(l.cost || "0");
    }
    const dayLabels = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
    const daily = [];
    for (let i = 6; i >= 0; i--) {
      const d = new Date(Date.now() - i * 86400000).toISOString().slice(0, 10);
      const v = byDay[d] || { tokens: 0, cost: 0 };
      daily.push({ day: d, label: dayLabels[6 - i], tokens: v.tokens, cost: Number(v.cost.toFixed(2)) });
    }
    const todayTokens = todayLogs.reduce((s, l) => s + (l.input_tokens || 0) + (l.output_tokens || 0), 0);
    return { todayRequests: todayLogs.length, todayCost: todayLogs.reduce((s,l)=>s+parseFloat(l.cost||"0"),0), errors: todayLogs.filter(l=>l.status==="failed").length, todayTokens, daily };
  },[logs]);

  if (isLoading) return <div><h2 className="font-display text-[25px] font-bold tracking-tight mb-6">数据看板</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载...</p></CardContent></Card></div>;
  if (we || ue) return <div><h2 className="font-display text-[25px] font-bold tracking-tight mb-6">数据看板</h2><Card className="border-[#E5484D]/20"><CardContent className="p-6 text-center"><p className="text-[#C4372C] mb-3">{(weMsg||ueMsg)instanceof Error?(weMsg||ueMsg as Error).message:String(weMsg||ueMsg||"")}</p><Button variant="destructive" size="sm" onClick={()=>{wr();ur()}}>重试</Button></CardContent></Card></div>;

  const statCards = [
    { label: "可用余额", value: formatAmount(wallet?.available) + " " + (wallet?.currency || "CNY"), Icon: DollarSign, iconColor: "text-[#C9A96A]", halo: "bg-[#C9A96A]/50", delta: "今日充值 · 实时到账" },
    { label: "今日请求", value: String(stats.todayRequests), Icon: Activity, iconColor: "text-[#4F6BED]", halo: "bg-[#4F6BED]/45", delta: "近 7 日趋势平稳" },
    { label: "今日费用", value: formatAmount(stats.todayCost) + " CNY", Icon: Zap, iconColor: "text-[#8B6FE8]", halo: "bg-[#8B6FE8]/45", delta: "较昨日 · 按量计费" },
    { label: "异常请求", value: String(stats.errors), Icon: AlertTriangle, iconColor: stats.errors > 0 ? "text-[#E5484D]" : "text-[#0FA88B]", halo: stats.errors > 0 ? "bg-[#E5484D]/40" : "bg-[#0FA88B]/45", delta: stats.errors > 0 ? "需要关注" : "全部正常" },
  ];

  return <div className="space-y-6">
    <div className="flex items-end justify-between gap-3 flex-wrap">
      <div>
        <h2 className="text-[25px] font-bold">数据看板</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">API 调用概览与模型用量分析</p>
      </div>
      <Button asChild><a href="/api-keys"><Plus size={16} />新建 API 密钥</a></Button>
    </div>

    {/* Token 流速 */}
    <div className="glass rounded-[22px] p-[24px] pb-[20px] relative overflow-hidden">
      <div className="absolute w-[260px] h-[260px] rounded-full bg-[radial-gradient(circle,rgba(139,92,246,0.16),transparent_65%)] -top-[90px] -right-[60px] pointer-events-none" />
      <div className="flex justify-between items-baseline gap-3 flex-wrap mb-4 relative">
        <div>
          <span className="font-display text-[17px] font-bold">Token 流速 · 实时</span>
          <span className="text-[12.5px] text-[#5C6472] ml-2.5">近 7 日调用量与费用</span>
        </div>
        <div>
          <span className="font-mono text-[32px] font-semibold tracking-tight text-[#4F6BED]">{stats.todayTokens.toLocaleString()}</span>
          <span className="text-[12.5px] text-[#5C6472] ml-2.5">今日 Tokens</span>
        </div>
      </div>
      <ResponsiveContainer width="100%" height={210}>
        <AreaChart data={stats.daily} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="tokGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#4F6BED" stopOpacity={0.26} />
              <stop offset="100%" stopColor="#8B6FE8" stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(22,26,35,0.08)" vertical={false} />
          <XAxis dataKey="label" tick={{ fontSize: 12, fill: "#5C6472" }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fontSize: 11, fill: "#5C6472" }} axisLine={false} tickLine={false} width={42} tickFormatter={(v: number) => (v >= 1000 ? `${Math.round(v / 1000)}k` : String(v))} />
          <YAxis yAxisId="cost" hide domain={["auto", "auto"]} />
          <Tooltip
            contentStyle={{ background: "rgba(255,255,255,0.9)", backdropFilter: "blur(12px)", border: "1px solid rgba(255,255,255,0.9)", borderRadius: 14, boxShadow: "0 12px 30px rgba(63,76,128,0.15)", fontSize: 12.5 }}
            formatter={(v: number, name: string) => (name === "tokens" ? `${v.toLocaleString()} tokens` : `${formatAmount(v)} CNY`)}
            labelFormatter={(_, p) => p?.[0]?.payload?.day || ""}
          />
          <Area type="monotone" dataKey="tokens" stroke="#4F6BED" strokeWidth={2.5} fill="url(#tokGrad)" />
          <Area yAxisId="cost" type="monotone" dataKey="cost" stroke="#0FA88B" strokeWidth={2} fill="none" />
        </AreaChart>
      </ResponsiveContainer>
      <div className="flex gap-2.5 mt-2.5">
        <span className="glass-soft inline-flex items-center gap-[7px] rounded-full px-[13px] py-1.5 text-[12px] font-semibold text-[#5C6472]"><i className="w-2 h-2 rounded-full bg-[#4F6BED]" />调用量</span>
        <span className="glass-soft inline-flex items-center gap-[7px] rounded-full px-[13px] py-1.5 text-[12px] font-semibold text-[#5C6472]"><i className="w-2 h-2 rounded-full bg-[#0FA88B]" />费用</span>
      </div>
    </div>

    {/* 统计卡 */}
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      {statCards.map((c) => (
        <div key={c.label} className="glass rounded-2xl p-5 relative overflow-hidden">
          <div className={`absolute w-[130px] h-[130px] rounded-full blur-[32px] opacity-50 -top-[46px] -right-[34px] ${c.halo}`} />
          <div className="flex items-center gap-3 relative">
            <span className={`nav-ic ${c.iconColor}`}><c.Icon size={18} /></span>
            <span className="text-[12px] font-semibold text-[#5C6472]">{c.label}</span>
          </div>
          <p className="font-mono text-[24px] font-semibold tracking-tight mt-2.5 relative">{c.value}</p>
          <div className="text-[12px] font-semibold mt-2.5 text-[#5C6472] relative">{c.delta}</div>
        </div>
      ))}
    </div>

    {/* 最近调用 */}
    <div className="glass rounded-[22px] p-[22px]">
      <div className="flex justify-between items-center gap-3 flex-wrap mb-4">
        <span className="font-display text-[16px] font-bold">最近调用</span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>模型</TableHead>
            <TableHead>请求 ID</TableHead>
            <TableHead className="text-right">Token</TableHead>
            <TableHead>状态</TableHead>
            <TableHead className="text-right">费用</TableHead>
            <TableHead className="text-right">时间</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.length === 0 && (
            <TableRow><TableCell colSpan={6}><EmptyState title="暂无调用记录" /></TableCell></TableRow>
          )}
          {logs.slice(0, 8).map((log) => {
            const dotKey = Object.keys(MODEL_DOTS).find((k) => log.model.toLowerCase().includes(k));
            const dot = dotKey ? MODEL_DOTS[dotKey] : "bg-[#4F6BED]";
            const ok = log.status === "completed";
            return (
              <TableRow key={log.id}>
                <TableCell><span className="inline-flex items-center gap-2 font-semibold"><i className={`w-[9px] h-[9px] rounded-[3px] ${dot}`} />{log.model}</span></TableCell>
                <TableCell className="font-mono text-[12.5px] text-[#5C6472] truncate max-w-[140px]">{log.request_id}</TableCell>
                <TableCell className="text-right font-mono text-[13px]">{(log.input_tokens || 0) + (log.output_tokens || 0)}</TableCell>
                <TableCell><span className={`status-pill ${ok ? "ok" : "fail"}`}><i />{ok ? "成功" : "失败"}</span></TableCell>
                <TableCell className="text-right font-mono text-[13px]">{formatAmount(log.cost)} CNY</TableCell>
                <TableCell className="text-right text-[13px] text-[#5C6472]">{new Date(log.created_at).toLocaleString("zh-CN")}</TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  </div>;
}
