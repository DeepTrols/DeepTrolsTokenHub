import { EmptyState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useAdminQuery } from "../lib/hooks/use-api";
import { BarChart3 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

interface ReconciliationRun {
  id: string; run_type: string; status: string; started_at: string; completed_at: string | null;
  total_usage_logs: number; matched_count: number; diff_count: number; period_start: string; period_end: string;
}

function fmtDT(iso: string): string { try { return new Date(iso).toLocaleString("zh-CN",{year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit"}); } catch { return iso; } }
const ll: Record<string,string> = { L0:"L0 · 原始用量", L1:"L1 · 计费对账" };
function sv(s: string): "success" | "secondary" | "destructive" | "outline" { if(s==="completed") return "success"; if(s==="running") return "secondary"; if(s==="failed") return "destructive"; return "outline"; }
const sl: Record<string,string> = { completed:"已完成", running:"运行中", failed:"失败" };
function mr(m:number,t:number):number { return t<=0?100:Math.round((m/t)*100); }
function rc(r:number):string { if(r>=99) return "text-[#0C7A55]"; if(r>=95) return "text-[#A06B12]"; return "text-destructive"; }

export default function Reconciliation() {
  const { data: runData, isLoading, isError, error, refetch } = useAdminQuery<{ data: ReconciliationRun[] }>("/reconciliation");
  const runs = Array.isArray(runData?.data) ? runData.data : [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";
  if (isLoading) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>对账管理</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><LoadingState message="加载对账数据..." /></SectionPageLayout.Content></SectionPageLayout>;
  const tr=runs.length, cr=runs.filter(r=>r.status==="completed").length, td=runs.reduce((s,r)=>s+r.diff_count,0);
  return (
    <div>
      <div className="mb-6"><h2 className="font-display text-[25px] font-bold tracking-tight">对账管理</h2><p className="text-[13px] text-[#5C6472] mt-1">查看最近的对账运行情况与差异统计</p></div>
      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">加载失败</p><p className="mt-1 text-xs break-all">{loadError}</p><Button variant="destructive" size="sm" className="mt-2" onClick={()=>refetch()}>重试</Button></CardContent></Card>}
      <div className="grid grid-cols-3 gap-4 mb-6">{[{l:"对账总数",v:tr},{l:"已完成",v:cr},{l:"累计差异",v:td}].map(c=><Card key={c.l}><CardContent className="p-5"><p className="text-[12px] font-semibold text-[#5C6472]">{c.l}</p><p className="font-mono text-[24px] font-semibold tracking-tight mt-1">{String(c.v)}</p></CardContent></Card>)}</div>
      <Card className="overflow-hidden"><Table>
        <TableHeader><TableRow><TableHead>级别</TableHead><TableHead>状态</TableHead><TableHead>时间段</TableHead><TableHead>开始</TableHead><TableHead>完成</TableHead><TableHead className="text-right">记录</TableHead><TableHead className="text-right">匹配</TableHead><TableHead className="text-right">差异</TableHead><TableHead className="text-right">率</TableHead></TableRow></TableHeader>
        <TableBody>{runs.length===0 && <TableRow><TableCell colSpan={9}><EmptyState icon={BarChart3} title="暂无对账记录" /></TableCell></TableRow>}
        {runs.map(r=>{const rate=mr(r.matched_count,r.total_usage_logs);return <TableRow key={r.id}><TableCell className="font-medium">{ll[r.run_type]||r.run_type}</TableCell><TableCell><Badge variant={sv(r.status)}>{sl[r.status]||r.status}</Badge></TableCell><TableCell className="text-muted-foreground text-xs">{fmtDT(r.period_start)} ~ {fmtDT(r.period_end)}</TableCell><TableCell className="text-xs">{fmtDT(r.started_at)}</TableCell><TableCell className="text-xs">{r.completed_at?fmtDT(r.completed_at):"-"}</TableCell><TableCell className="text-right font-mono text-xs">{r.total_usage_logs}</TableCell><TableCell className="text-right font-mono text-xs text-[#0C7A55]">{r.matched_count}</TableCell><TableCell className="text-right font-mono text-xs">{r.diff_count>0?<span className="text-destructive">{r.diff_count}</span>:<span>0</span>}</TableCell><TableCell className="text-right"><span className={"font-mono font-medium "+rc(rate)}>{rate}%</span></TableCell></TableRow>})}</TableBody>
      </Table></Card>
    </div>
  );
}
