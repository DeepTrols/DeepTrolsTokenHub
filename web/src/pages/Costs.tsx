import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { TrendingUp, BarChart3 } from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from "recharts";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
const COLORS=["#6366f1","#10b981","#f59e0b","#ef4444","#8b5cf6","#06b6d4","#f97316","#84cc16"];
interface CostSummary { model:string; request_count:number; final_cost:string; upstream_cost:string; profit:string; profit_margin:string; }
export default function Costs(){
  const{data:costData,isLoading,isError,error,refetch}=useAdminQuery<{data:CostSummary[]}>("/costs");
  const rows=costData?.data??[];
  const le=isError?(error instanceof Error?error.message:String(error)):"";
  const[mr,setMr]=useState("1.5");const[mm,setMm]=useState("");const[me,setMe]=useState("");
  const mu=useAdminMutation<{rows_updated:number},{markup_rate:string}>("post","/pricing/markup","");
  const hm=async()=>{setMe("");setMm("");try{const r=await mu.mutateAsync({markup_rate:mr});setMm("已应用 "+mr);refetch()}catch(e){setMe(e instanceof Error?e.message:"失败")}};
  const t=rows.reduce((a,r)=>{a.f+=parseFloat(r.final_cost||"0");a.u+=parseFloat(r.upstream_cost||"0");a.r+=r.request_count;return a},{f:0,u:0,r:0});
  const tp=t.f-t.u,tm=t.f>0?(tp/t.f)*100:0;
  const cd=rows.map(r=>({name:r.model.length>22?r.model.slice(0,22)+"...":r.model,fn:r.model,cost:parseFloat(r.final_cost||"0")}));
  if(isLoading)return <div><h2 className="text-2xl font-bold mb-6">数据看板</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载中...</p></CardContent></Card></div>;
  return <div>
    <div className="mb-4"><h2 className="text-2xl font-bold">数据看板</h2><p className="text-sm text-muted-foreground mt-1">全平台成本分析</p></div>
    <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-5">
      {[{l:"总调用",v:t.r.toLocaleString()},{l:"总消耗",v:t.f.toFixed(2)+" CNY"},{l:"上游成本",v:t.u.toFixed(2)+" CNY",c:"text-orange-500"},{l:"利润",v:tp.toFixed(2)+" CNY · "+tm.toFixed(1)+"%",c:tp>=0?"text-emerald-600":"text-destructive"}].map(c=><Card key={c.l}><CardContent className="p-5"><p className="text-sm text-muted-foreground">{c.l}</p><p className={"text-2xl font-bold mt-1 "+(c.c||"")}>{c.v}</p></CardContent></Card>)}
    </div>
    <Card className="mb-5"><CardContent className="p-5"><h3 className="font-semibold mb-2">加价率</h3><div className="flex items-end gap-3 max-w-md"><Input type="number" min="1" step="0.1" value={mr} onChange={e=>setMr(e.target.value)}/><Button onClick={hm} disabled={mu.isPending}>{mu.isPending?"应用中...":"应用"}</Button></div>{mm&&<p className="mt-2 text-sm text-emerald-600">{mm}</p>}{me&&<p className="mt-2 text-sm text-destructive">{me}</p>}</CardContent></Card>
    {le&&<Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><Button variant="destructive" size="sm" onClick={()=>refetch()}>重试</Button></CardContent></Card>}
    <Card className="overflow-hidden"><Table><TableHeader><TableRow><TableHead>模型</TableHead><TableHead className="text-right">调用</TableHead><TableHead className="text-right">售出</TableHead><TableHead className="text-right">成本</TableHead><TableHead className="text-right">利润</TableHead><TableHead className="text-right">率</TableHead></TableRow></TableHeader><TableBody>{rows.length===0&&<TableRow><TableCell colSpan={6} className="py-12 text-center text-muted-foreground"><TrendingUp size={32} className="mx-auto mb-3 opacity-30"/><p>暂无数据</p></TableCell></TableRow>}{rows.map(r=>{const p=parseFloat(r.profit||"0"),m=parseFloat(r.profit_margin||"0");return <TableRow key={r.model}><TableCell className="font-medium text-xs">{r.model}</TableCell><TableCell className="text-right text-xs">{r.request_count.toLocaleString()}</TableCell><TableCell className="text-right font-mono text-xs">{parseFloat(r.final_cost).toFixed(4)}</TableCell><TableCell className="text-right font-mono text-xs text-orange-500">{parseFloat(r.upstream_cost).toFixed(4)}</TableCell><TableCell className={"text-right font-mono text-xs "+(p>=0?"text-emerald-600":"text-destructive")}>{p.toFixed(4)}</TableCell><TableCell className={"text-right font-mono text-xs "+(m<0?"text-destructive":m<30?"text-yellow-600":"text-emerald-600")}>{r.profit_margin}</TableCell></TableRow>})}</TableBody></Table></Card>
  </div>;
}