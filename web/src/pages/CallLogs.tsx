import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Fragment, useState, useMemo } from "react";
import { UsageLog } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { Search, FilterX, ChevronDown, ChevronRight, BarChart3 } from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface ChargeLine { dimension: string; unit_name: string; quantity: number; unit_price: string; line_cost: string; }

export default function CallLogs() {
  const { data: usage, isLoading, isError, error, refetch } = useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=200");
  const logs = usage?.data ?? [];
  const [filter, setFilter] = useState({ model: "", status: "", requestId: "" });
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [chargesMap, setChargesMap] = useState<Record<string, ChargeLine[]>>({});
  const [chargesLoading, setChargesLoading] = useState(false);
  const [showStats, setShowStats] = useState(true);

  const toggleExpand = async (id: string) => {
    if (expandedId === id) { setExpandedId(null); return; }
    setExpandedId(id);
    if (!chargesMap[id]) {
      setChargesLoading(true);
      try { const r = await fetch("/api/console/usage/" + id + "/charge-lines", { credentials: "include" }); if (r.ok) { const d = await r.json() as { data: ChargeLine[] }; setChargesMap(p => ({ ...p, [id]: d.data ?? [] })); } }
      finally { setChargesLoading(false); }
    }
  };

  const filtered = logs.filter(l => (!filter.model || l.model.includes(filter.model)) && (!filter.status || l.status === filter.status) && (!filter.requestId || l.request_id.includes(filter.requestId)));

  const stats = useMemo(() => {
    const bm: Record<string, { tokens: number; cost: number; count: number }> = {};
    for (const l of filtered) { if (!bm[l.model]) bm[l.model] = { tokens: 0, cost: 0, count: 0 }; bm[l.model].tokens += (l.input_tokens || 0) + (l.output_tokens || 0); bm[l.model].cost += parseFloat(l.cost || "0"); bm[l.model].count += 1; }
    const models = Object.entries(bm).map(([k, v]) => ({ name: k.length > 18 ? k.slice(0, 18) + "…" : k, fullName: k, ...v })).sort((a, b) => b.cost - a.cost);
    return { models, totalCost: filtered.reduce((s, l) => s + parseFloat(l.cost || "0"), 0), totalRequests: filtered.length };
  }, [filtered]);

  if (isLoading) return <div><h2 className="text-2xl font-bold mb-6">调用记录</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3" /><p className="text-muted-foreground">加载调用记录...</p></CardContent></Card></div>;
  if (isError) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>调用记录</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><ErrorState error={error} onRetry={()=>refetch()} /></SectionPageLayout.Content></SectionPageLayout>;

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <div><h2 className="text-2xl font-bold">调用记录</h2><p className="text-sm text-muted-foreground mt-1">API 调用详情与模型用量统计</p></div>
        <Button variant={showStats ? "secondary" : "outline"} size="sm" onClick={() => setShowStats(!showStats)}><BarChart3 size={14} className="mr-1.5" />统计面板</Button>
      </div>

      {showStats && <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-5">
        <Card><CardHeader className="pb-3"><CardTitle className="text-sm">各模型调用分布</CardTitle></CardHeader>
          <CardContent><ResponsiveContainer width="100%" height={200}><BarChart data={stats.models.slice(0, 8)}><CartesianGrid strokeDasharray="3 3" stroke="#f5f5f5" /><XAxis dataKey="name" tick={{ fontSize: 10 }} angle={-25} textAnchor="end" height={50} /><YAxis tick={{ fontSize: 10 }} /><Tooltip formatter={(v: number) => v.toLocaleString() + " tokens"} /><Bar dataKey="tokens" fill="#6366f1" radius={[4, 4, 0, 0]} /></BarChart></ResponsiveContainer></CardContent></Card>
        <Card><CardHeader className="pb-3"><CardTitle className="text-sm">统计概览</CardTitle></CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-3 mb-4">
              {[["总请求", stats.totalRequests], ["总费用", formatAmount(stats.totalCost) + " CNY"], ["模型数", stats.models.length]].map(([l, v]) => <div key={l as string} className="text-center p-3 bg-muted rounded-xl"><p className="text-xs text-muted-foreground">{l as string}</p><p className="text-lg font-bold mt-1">{(l as string) === "总费用" ? <span className="text-primary">{v as string}</span> : (v as React.ReactNode)}</p></div>)}
            </div>
            <div className="space-y-2">{stats.models.slice(0, 6).map(m => <div key={m.fullName} className="flex items-center justify-between text-xs"><span className="text-muted-foreground truncate max-w-[160px]">{m.fullName}</span><span className="text-muted-foreground/60">{m.count}次 · {formatAmount(m.cost)} CNY</span></div>)}</div>
          </CardContent></Card>
      </div>}

      <Card className="mb-4"><CardContent className="p-4"><div className="flex gap-3 flex-wrap items-center">
        <Search size={15} className="text-muted-foreground shrink-0" />
        <Input placeholder="模型名称" value={filter.model} onChange={e => setFilter({ ...filter, model: e.target.value })} className="w-36 h-9 text-sm" />
        <Select value={filter.status || "all"} onValueChange={v => setFilter({ ...filter, status: v === "all" ? "" : v })}>
          <SelectTrigger className="w-28 h-9 text-sm"><SelectValue placeholder="状态" /></SelectTrigger>
          <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="completed">成功</SelectItem><SelectItem value="failed">失败</SelectItem></SelectContent>
        </Select>
        <Input placeholder="请求 ID" value={filter.requestId} onChange={e => setFilter({ ...filter, requestId: e.target.value })} className="w-48 font-mono h-9 text-sm" />
        {(filter.model || filter.status || filter.requestId) && <Button variant="ghost" size="sm" onClick={() => setFilter({ model: "", status: "", requestId: "" })}><FilterX size={14} className="mr-1" />重置</Button>}
        <span className="text-xs text-muted-foreground ml-auto">{filtered.length} 条记录</span></div></CardContent></Card>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader><TableRow><TableHead>模型</TableHead><TableHead>请求 ID</TableHead><TableHead>Token</TableHead><TableHead>状态</TableHead><TableHead className="text-right">费用</TableHead><TableHead className="text-right">时间</TableHead></TableRow></TableHeader>
          <TableBody>
            {filtered.length === 0 && <TableRow><TableCell colSpan={6}><EmptyState title="暂无调用记录" /></TableCell></TableRow>}
            {filtered.map(log => { const isExpanded = expandedId === log.id; const charges = chargesMap[log.id];
              return <Fragment key={log.id}>
                <TableRow className="hover:bg-muted/30">
                  <TableCell className="font-medium text-xs">{log.model}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground truncate max-w-[130px]">{log.request_id}</TableCell>
                  <TableCell className="text-xs">{(log.input_tokens || 0) + (log.output_tokens || 0)}</TableCell>
                  <TableCell><Badge variant={log.status === "completed" ? "success" : "destructive"}>{log.status === "completed" ? "成功" : "失败"}</Badge></TableCell>
                  <TableCell className="text-right"><Button variant="ghost" size="sm" onClick={() => toggleExpand(log.id)} className="font-mono text-xs h-auto py-0">{isExpanded ? <ChevronDown size={13} className="mr-1" /> : <ChevronRight size={13} className="mr-1" />}{formatAmount(log.cost)} CNY</Button></TableCell>
                  <TableCell className="text-right text-xs text-muted-foreground">{new Date(log.created_at).toLocaleString("zh-CN")}</TableCell>
                </TableRow>
                {isExpanded && <TableRow className="bg-muted/50"><TableCell colSpan={6} className="px-8 py-4">
                  {chargesLoading && !charges ? <p className="text-sm text-muted-foreground">加载费用明细...</p>
                  : charges && charges.length > 0 ? <table className="w-full max-w-lg text-xs"><thead><tr className="border-b"><th className="text-left py-1.5 font-medium">计费维度</th><th className="text-right py-1.5 font-medium">数量</th><th className="text-right py-1.5 font-medium">单价</th><th className="text-right py-1.5 font-medium">小计</th></tr></thead><tbody>{charges.map((c, i) => <tr key={i} className="border-b last:border-b-0"><td className="py-1.5 font-medium">{c.dimension}</td><td className="py-1.5 text-right">{c.quantity.toLocaleString()} {c.unit_name}</td><td className="py-1.5 text-right font-mono">{c.unit_price}</td><td className="py-1.5 text-right font-mono text-emerald-600">{formatAmount(c.line_cost)} CNY</td></tr>)}</tbody></table>
                  : <p className="text-sm text-muted-foreground">暂无费用明细</p>}
                </TableCell></TableRow>}
              </Fragment>;
            })}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
