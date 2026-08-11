import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { History, CheckCircle, XCircle } from "lucide-react";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

interface LoginHistoryEntry { ip_address: string; user_agent: string; success: boolean; created_at: string; }

export default function Security() {
  const { data: historyData, isLoading: isLoadingHistory, isError: isHistoryError, error: historyError, refetch: refetchHistory } = useConsoleQuery<{ data: LoginHistoryEntry[] }>("/security/login-history");
  const loginHistory = historyData?.data ?? [];

  function fmtTime(iso: string): string { try { return new Date(iso).toLocaleString("zh-CN", { year:"numeric", month:"2-digit", day:"2-digit", hour:"2-digit", minute:"2-digit" }); } catch { return iso; } }
  function trunc(s: string): string { return s && s.length > 40 ? s.slice(0,40)+"..." : s; }

  return (
    <div>
      <div className="mb-6"><h2 className="text-2xl font-bold">安全设置</h2><p className="text-sm text-muted-foreground mt-1">管理账户安全，查看登录记录</p></div>
      <div className="space-y-4 max-w-2xl">
        <Card><CardContent className="p-5">
          <div className="flex items-center gap-3 mb-4"><div className="p-2 bg-muted rounded-lg"><History size={20} /></div><div><h3 className="font-semibold">登录记录</h3><p className="text-xs text-muted-foreground">最近登录活动</p></div></div>
          {isLoadingHistory ? <div className="space-y-3 animate-pulse">{[1,2,3].map(i=><div key={i} className="h-8 bg-muted rounded" />)}</div> :
           isHistoryError ? <div className="text-center py-4"><p className="text-destructive text-sm mb-2">{historyError instanceof Error ? historyError.message : "加载失败"}</p><Button variant="outline" size="sm" onClick={()=>refetchHistory()}>重试</Button></div> :
           loginHistory.length===0 ? <p className="text-sm text-muted-foreground text-center py-4">暂无登录记录</p> :
           <Table><TableHeader><TableRow><TableHead>IP 地址</TableHead><TableHead>客户端</TableHead><TableHead>状态</TableHead><TableHead>时间</TableHead></TableRow></TableHeader>
           <TableBody>{loginHistory.map((h,i)=><TableRow key={i}><TableCell className="font-mono text-xs">{h.ip_address}</TableCell><TableCell className="text-xs text-muted-foreground max-w-40 truncate">{trunc(h.user_agent)}</TableCell><TableCell><Badge variant={h.success ? "success" : "destructive"}>{h.success ? <><CheckCircle size={12} className="mr-1"/>成功</> : <><XCircle size={12} className="mr-1"/>失败</>}</Badge></TableCell><TableCell className="text-xs text-muted-foreground">{fmtTime(h.created_at)}</TableCell></TableRow>)}</TableBody></Table>}
        </CardContent></Card>
      </div>
    </div>
  );
}
