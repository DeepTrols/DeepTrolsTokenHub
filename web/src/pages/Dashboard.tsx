import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useMemo } from "react";
import { WalletData, UsageLog } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { DollarSign, Activity, Zap, AlertTriangle, TrendingUp, BarChart3 } from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from "recharts";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
const COLORS = ["#6366f1","#10b981","#f59e0b","#ef4444","#8b5cf6","#06b6d4","#f97316","#84cc16"];
export default function Dashboard() {
  const { data: wallet, isLoading: wl, isError: we, error: weMsg, refetch: wr } = useConsoleQuery<WalletData>("/wallet");
  const { data: usage, isLoading: ul, isError: ue, error: ueMsg, refetch: ur } = useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=200");
  const logs = usage?.data ?? [];
  const isLoading = wl || ul;
  const stats = useMemo(() => {
    const today = new Date().toISOString().slice(0,10);
    const todayLogs = logs.filter(l=>l.created_at.startsWith(today));
    const bm:Record<string,{tokens:number;cost:number}>={};
    for(const l of logs){if(!bm[l.model])bm[l.model]={tokens:0,cost:0};bm[l.model].tokens+=(l.input_tokens||0)+(l.output_tokens||0);bm[l.model].cost+=parseFloat(l.cost||"0")}
    const models=Object.entries(bm).map(([k,v])=>({name:k.length>22?k.slice(0,22)+"...":k,fullName:k,...v})).sort((a,b)=>b.tokens-a.tokens);
    return {todayRequests:todayLogs.length,todayCost:todayLogs.reduce((s,l)=>s+parseFloat(l.cost||"0"),0),errors:todayLogs.filter(l=>l.status==="failed").length,models};
  },[logs]);
  if(isLoading)return <div><h2 className="text-2xl font-bold mb-6">数据看板</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载...</p></CardContent></Card></div>;
  if(we||ue)return <div><h2 className="text-2xl font-bold mb-6">数据看板</h2><Card className="border-destructive/20"><CardContent className="p-6 text-center"><p className="text-destructive mb-3">{(weMsg||ueMsg)instanceof Error?(weMsg||ueMsg as Error).message:String(weMsg||ueMsg||"")}</p><Button variant="destructive" size="sm" onClick={()=>{wr();ur()}}>重试</Button></CardContent></Card></div>;
  return <div>
    <div className="mb-6"><h2 className="text-2xl font-bold">数据看板</h2><p className="text-sm text-muted-foreground mt-1">API 调用概览与模型用量分析</p></div>
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      {[{label:"可用余额",value:wallet?.available+" "+(wallet?.currency||"CNY"),Icon:DollarSign,c:"text-emerald-600",bg:"bg-emerald-50"},{label:"今日请求",value:String(stats.todayRequests),Icon:Activity,c:"text-indigo-600",bg:"bg-indigo-50"},{label:"今日费用",value:stats.todayCost.toFixed(3)+" CNY",Icon:Zap,c:"text-amber-600",bg:"bg-amber-50"},{label:"异常请求",value:String(stats.errors),Icon:AlertTriangle,c:stats.errors>0?"text-rose-600":"text-muted-foreground",bg:stats.errors>0?"bg-rose-50":"bg-muted"}].map(c=><Card key={c.label}><CardContent className="p-5"><div className="flex items-center gap-3 mb-3"><div className={"p-2.5 rounded-xl "+c.bg}><c.Icon size={20} className={c.c}/></div><span className="text-sm text-muted-foreground font-medium">{c.label}</span></div><p className="text-2xl font-bold tracking-tight">{c.value}</p></CardContent></Card>)}
    </div>
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 mb-6">
      <Card><CardHeader className="pb-3"><CardTitle className="text-sm flex items-center gap-2"><BarChart3 size={18} className="text-indigo-500"/>各模型 Token 用量</CardTitle></CardHeader><CardContent><ResponsiveContainer width="100%" height={260}><BarChart data={stats.models.slice(0,10)}><CartesianGrid strokeDasharray="3 3" stroke="#f5f5f5"/><XAxis dataKey="name" tick={{fontSize:11}} angle={-30} textAnchor="end" height={60}/><YAxis tick={{fontSize:11}}/><Tooltip formatter={(v:number)=>v.toLocaleString()+" tokens"} labelFormatter={(_,p)=>p?.[0]?.payload?.fullName||""}/><Bar dataKey="tokens" fill="#6366f1" radius={[6,6,0,0]}/></BarChart></ResponsiveContainer></CardContent></Card>
      <Card><CardHeader className="pb-3"><CardTitle className="text-sm flex items-center gap-2"><TrendingUp size={18} className="text-emerald-500"/>模型费用占比</CardTitle></CardHeader><CardContent><ResponsiveContainer width="100%" height={260}><PieChart><Pie data={stats.models.filter(m=>m.cost>0).slice(0,8)} dataKey="cost" nameKey="name" cx="50%" cy="50%" outerRadius={95} innerRadius={50} label={({name,percent})=>name+" "+(percent*100).toFixed(0)+"%"}>{stats.models.filter(m=>m.cost>0).slice(0,8).map((_,i)=><Cell key={i} fill={COLORS[i%COLORS.length]}/>)}</Pie><Tooltip formatter={(v:number)=>v.toFixed(3)+" CNY"}/></PieChart></ResponsiveContainer></CardContent></Card>
    </div>
    <Card><CardHeader className="pb-3"><CardTitle className="text-sm">最近请求</CardTitle></CardHeader>
    <Table><TableHeader><TableRow><TableHead>模型</TableHead><TableHead>请求 ID</TableHead><TableHead>Token</TableHead><TableHead>状态</TableHead><TableHead className="text-right">费用</TableHead><TableHead className="text-right">时间</TableHead></TableRow></TableHeader><TableBody>{logs.length===0&&<TableRow><TableCell colSpan={6}><EmptyState title="暂无调用记录" /></TableCell></TableRow>}{logs.slice(0,8).map(log=><TableRow key={log.id} className="hover:bg-muted/30"><TableCell className="font-medium text-xs">{log.model}</TableCell><TableCell className="font-mono text-xs text-muted-foreground truncate max-w-[140px]">{log.request_id}</TableCell><TableCell className="text-xs">{(log.input_tokens||0)+(log.output_tokens||0)}</TableCell><TableCell><Badge variant={log.status==="completed"?"success":"destructive"}>{log.status==="completed"?"成功":"失败"}</Badge></TableCell><TableCell className="text-right font-mono text-xs">{log.cost}</TableCell><TableCell className="text-right text-xs text-muted-foreground">{new Date(log.created_at).toLocaleString("zh-CN")}</TableCell></TableRow>)}</TableBody></Table></Card>
  </div>;
}