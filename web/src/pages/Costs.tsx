import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useAdminQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { TrendingUp, BarChart3 } from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
const COLORS=["#6366f1","#10b981","#f59e0b","#ef4444","#8b5cf6","#06b6d4","#f97316","#84cc16"];
interface CostSummary { model:string; request_count:number; final_cost:string; upstream_cost:string; profit:string; profit_margin:string; }
export default function Costs(){
  const{data:costData,isLoading,isError,error,refetch}=useAdminQuery<{data:CostSummary[]}>("/costs");
  const rows=costData?.data??[];
  const le=isError?(error instanceof Error?error.message:String(error)):"";
  const t=rows.reduce((a,r)=>{a.f+=parseFloat(r.final_cost||"0");a.u+=parseFloat(r.upstream_cost||"0");a.r+=r.request_count;return a},{f:0,u:0,r:0});
  const tp=t.f-t.u,tm=t.f>0?(tp/t.f)*100:0;
  const cd=rows.map(r=>({name:r.model.length>22?r.model.slice(0,22)+"...":r.model,fn:r.model,cost:parseFloat(r.final_cost||"0")}));
  if(isLoading)return <div><h2 className="font-display text-[25px] font-bold tracking-tight mb-6">成本核算</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载中...</p></CardContent></Card></div>;
  return <div>
    <div className="mb-4"><h2 className="font-display text-[25px] font-bold tracking-tight">数据看板</h2><p className="text-[13px] text-[#5C6472] mt-1">全平台成本分析</p></div>
    <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-5">
      {[{l:"总调用",v:t.r.toLocaleString()},{l:"总消耗",v:formatAmount(t.f)+" CNY"},{l:"上游成本",v:formatAmount(t.u)+" CNY",c:"text-[#D97706]"},{l:"利润",v:formatAmount(tp)+" CNY · "+tm.toFixed(1)+"%",c:tp>=0?"text-[#0C7A55]":"text-destructive"}].map(c=><Card key={c.l}><CardContent className="p-5"><p className="text-sm text-muted-foreground">{c.l}</p><p className={"font-mono text-[22px] font-semibold tracking-tight mt-1 "+(c.c||"")}>{c.v}</p></CardContent></Card>)}
    </div>
    {le&&<Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><Button variant="destructive" size="sm" onClick={()=>refetch()}>重试</Button></CardContent></Card>}
    <Card className="overflow-hidden"><Table><TableHeader><TableRow><TableHead>模型</TableHead><TableHead className="text-right">调用</TableHead><TableHead className="text-right">售出</TableHead><TableHead className="text-right">成本</TableHead><TableHead className="text-right">利润</TableHead><TableHead className="text-right">率</TableHead></TableRow></TableHeader><TableBody>{rows.length===0&&<TableRow><TableCell colSpan={6} className="py-12 text-center text-muted-foreground"><TrendingUp size={32} className="mx-auto mb-3 opacity-30"/><p>暂无数据</p></TableCell></TableRow>}{rows.map(r=>{const p=parseFloat(r.profit||"0"),m=parseFloat(r.profit_margin||"0");return <TableRow key={r.model}><TableCell className="font-medium text-xs">{r.model}</TableCell><TableCell className="text-right text-xs">{r.request_count.toLocaleString()}</TableCell><TableCell className="text-right font-mono text-xs">{formatAmount(r.final_cost)} CNY</TableCell><TableCell className="text-right font-mono text-xs text-[#D97706]">{formatAmount(r.upstream_cost)} CNY</TableCell><TableCell className={"text-right font-mono text-xs "+(p>=0?"text-[#0C7A55]":"text-destructive")}>{formatAmount(p)} CNY</TableCell><TableCell className={"text-right font-mono text-xs "+(m<0?"text-destructive":m<30?"text-[#A06B12]":"text-[#0C7A55]")}>{r.profit_margin}</TableCell></TableRow>})}</TableBody></Table></Card>
  </div>;
}
