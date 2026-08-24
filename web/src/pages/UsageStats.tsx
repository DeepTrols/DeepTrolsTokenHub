import { ErrorState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useMemo } from "react";
import { BarChart3, TrendingUp } from "lucide-react";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { type UsageLog } from "../lib/api";
import { Card, CardContent } from "@/components/ui/card";

interface ModelBreakdown { model: string; tokens: number; cost: number; pct: number; }
interface DailyTrend { date: string; cost: number; }

function buildStats(logs: UsageLog[]): { models: ModelBreakdown[]; trends: DailyTrend[] } {
  const byModel: Record<string,{tokens:number;cost:number}> = {};
  for (const log of logs) { if(!byModel[log.model]) byModel[log.model]={tokens:0,cost:0}; byModel[log.model].tokens+=(log.input_tokens||0)+(log.output_tokens||0); byModel[log.model].cost+=parseFloat(log.cost||"0"); }
  const totalTokens = Object.values(byModel).reduce((s,m)=>s+m.tokens,0);
  const models: ModelBreakdown[] = Object.entries(byModel).map(([model,stats])=>({model,tokens:stats.tokens,cost:stats.cost,pct:totalTokens>0?Math.round((stats.tokens/totalTokens)*100):0})).sort((a,b)=>b.tokens-a.tokens);
  const byDate: Record<string,number> = {};
  for (const log of logs) { const date=log.created_at.slice(0,10); byDate[date]=(byDate[date]||0)+parseFloat(log.cost||"0"); }
  const trends: DailyTrend[] = Object.entries(byDate).map(([date,cost])=>({date,cost})).sort((a,b)=>a.date.localeCompare(b.date)).slice(-7);
  return {models,trends};
}

export default function UsageStats() {
  const { data: usage, isLoading, isError, error, refetch } = useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=200");
  const { models, trends } = useMemo(()=>buildStats(usage?.data??[]),[usage]);
  if (isLoading) return <div><div className="mb-6"><h2 className="font-display text-[25px] font-bold tracking-tight">用量统计</h2></div><div className="grid grid-cols-1 lg:grid-cols-2 gap-6">{[1,2].map(i=><Card key={i}><CardContent className="p-5 animate-pulse"><div className="h-5 bg-muted rounded w-1/3 mb-4"/><div className="space-y-3">{[1,2,3].map(j=><div key={j} className="h-4 bg-muted rounded"/>)}</div></CardContent></Card>)}</div></div>;
  if (isError) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>用量统计</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><ErrorState error={error} onRetry={()=>refetch()} /></SectionPageLayout.Content></SectionPageLayout>;
  const totalCost = trends.reduce((s,d)=>s+d.cost,0);
  if (models.length===0 && trends.length===0) return <div><div className="mb-6"><h2 className="font-display text-[25px] font-bold tracking-tight">用量统计</h2></div><Card><CardContent className="p-12 text-center"><p className="text-muted-foreground text-lg">暂无用量数据</p></CardContent></Card></div>;
  return (
    <div>
      <div className="mb-6"><h2 className="font-display text-[25px] font-bold tracking-tight">用量统计</h2><p className="text-[13px] text-[#5C6472] mt-1">Token 消耗趋势与模型费用分布</p></div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card><CardContent className="p-5"><div className="flex items-center gap-2 mb-4"><BarChart3 size={18} className="text-primary"/><h3 className="font-semibold">各模型 Token 用量</h3></div><div className="space-y-4">{models.map(m=><div key={m.model}><div className="flex items-center justify-between mb-1"><span className="text-sm font-medium">{m.model}</span><span className="text-xs text-muted-foreground">{m.tokens.toLocaleString()} tokens · {formatAmount(m.cost)} CNY</span></div><div className="w-full bg-muted rounded-full h-2"><div className="bg-primary h-2 rounded-full" style={{width:m.pct+'%'}}/></div></div>)}</div></CardContent></Card>
        <Card><CardContent className="p-5"><div className="flex items-center gap-2 mb-4"><TrendingUp size={18} className="text-primary"/><h3 className="font-semibold">费用趋势（近7日）</h3></div><div className="space-y-3">{trends.map(d=><div key={d.date} className="flex items-center justify-between py-1"><span className="text-sm text-muted-foreground">{d.date}</span><span className="text-sm font-mono font-medium">{formatAmount(d.cost)} CNY</span></div>)}</div>{totalCost>0 && <div className="mt-4 pt-4 border-t flex items-center justify-between"><span className="text-sm text-muted-foreground">{trends.length} 日合计</span><span className="text-lg font-bold text-primary">{formatAmount(totalCost)} CNY</span></div>}</CardContent></Card>
      </div>
    </div>
  );
}
