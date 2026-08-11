import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState, useMemo } from "react";
import { useAdminQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { Search, FilterX, BarChart3 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";

interface AdminUsageLog {
  id: string; model: string; request_id: string; api_key_id: string;
  user_email: string; status: string; cost: string;
  input_tokens: number; output_tokens: number; created_at: string;
}

export default function AuditLogs() {
  const { data, isLoading, isError, error, refetch } =
    useAdminQuery<{ data: AdminUsageLog[]; total: number }>("/admin-usage");
  const logs = data?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  const [query, setQuery] = useState("");
  const [filterUser, setFilterUser] = useState("");
  const [filterModel, setFilterModel] = useState("");
  const [filterStatus, setFilterStatus] = useState("");

  const filtered = useMemo(() => {
    let r = logs;
    if (query) { const q = query.toLowerCase(); r = r.filter(l => l.user_email.includes(q) || l.model.includes(q) || l.request_id.includes(q)); }
    if (filterUser) r = r.filter(l => l.user_email.includes(filterUser));
    if (filterModel) r = r.filter(l => l.model.includes(filterModel));
    if (filterStatus) r = r.filter(l => l.status === filterStatus);
    return r;
  }, [logs, query, filterUser, filterModel, filterStatus]);

  const stats = useMemo(() => {
    const bu: Record<string, { calls: number; cost: number }> = {};
    for (const l of filtered) {
      if (!bu[l.user_email]) bu[l.user_email] = { calls: 0, cost: 0 };
      bu[l.user_email].calls++; bu[l.user_email].cost += parseFloat(l.cost || "0");
    }
    return { totalCost: filtered.reduce((s, l) => s + parseFloat(l.cost || "0"), 0), users: Object.entries(bu).sort((a, b) => b[1].cost - a[1].cost) };
  }, [filtered]);

  if (isLoading) return <div><h2 className="text-2xl font-bold mb-6">调用日志</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3" /><p className="text-muted-foreground">加载全平台调用日志...</p></CardContent></Card></div>;

  return (
    <div>
      <div className="mb-4"><h2 className="text-2xl font-bold">调用日志</h2><p className="text-sm text-muted-foreground mt-1">全平台 API 调用记录（管理员视角）</p></div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">加载失败</p><p className="mt-1">{loadError}</p><Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>重试</Button></CardContent></Card>}

      {stats.users.length > 0 && <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        {[{ label: "总调用次数", v: filtered.length }, { label: "总费用", v: formatAmount(stats.totalCost) + " CNY" }, { label: "用户数", v: stats.users.length }].map(c => <Card key={c.label}><CardContent className="p-4 text-center"><p className="text-xs text-muted-foreground">{c.label}</p><p className="text-xl font-bold mt-1">{String(c.v)}</p></CardContent></Card>)}
        <Card><CardContent className="p-4"><p className="text-xs text-muted-foreground mb-2">各用户消费</p>
          {stats.users.slice(0, 5).map(([email, u]) => <div key={email} className="flex justify-between text-xs mb-1"><span className="text-muted-foreground truncate max-w-[140px]">{email}</span><span className="font-mono">{formatAmount(u.cost)} CNY</span></div>)}</CardContent></Card>
      </div>}

      <div className="flex gap-2 mb-4 flex-wrap">
        <div className="relative flex-1 max-w-[200px]"><Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" /><Input placeholder="搜索" value={query} onChange={e => setQuery(e.target.value)} className="pl-8 h-9 text-sm" /></div>
        <Input placeholder="用户邮箱" value={filterUser} onChange={e => setFilterUser(e.target.value)} className="w-40 h-9 text-sm" />
        <Input placeholder="模型" value={filterModel} onChange={e => setFilterModel(e.target.value)} className="w-32 h-9 text-sm" />
        <Select value={filterStatus || "all"} onValueChange={v => setFilterStatus(v === "all" ? "" : v)}>
          <SelectTrigger className="w-28 h-9 text-sm"><SelectValue placeholder="状态" /></SelectTrigger>
          <SelectContent><SelectItem value="all">全部</SelectItem><SelectItem value="completed">成功</SelectItem><SelectItem value="failed">失败</SelectItem></SelectContent>
        </Select>
        {(query || filterUser || filterModel || filterStatus) && <Button variant="ghost" size="sm" onClick={() => { setQuery(""); setFilterUser(""); setFilterModel(""); setFilterStatus(""); }}><FilterX size={14} className="mr-1" />重置</Button>}
        <span className="text-xs text-muted-foreground ml-auto self-center">{filtered.length} 条</span>
      </div>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader><TableRow><TableHead>用户</TableHead><TableHead>模型</TableHead><TableHead>请求 ID</TableHead><TableHead className="text-right">Token</TableHead><TableHead>状态</TableHead><TableHead className="text-right">费用</TableHead><TableHead className="text-right">时间</TableHead></TableRow></TableHeader>
          <TableBody>
            {filtered.length === 0 && <TableRow><TableCell colSpan={7}><EmptyState icon={BarChart3} title="暂无调用记录" /></TableCell></TableRow>}
            {filtered.map(l => <TableRow key={l.id} className="hover:bg-muted/30">
              <TableCell className="text-xs truncate max-w-[150px]">{l.user_email}</TableCell>
              <TableCell className="font-medium text-xs">{l.model}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground truncate max-w-[120px]">{l.request_id}</TableCell>
              <TableCell className="text-right text-xs">{l.input_tokens + l.output_tokens}</TableCell>
              <TableCell><Badge variant={l.status === "completed" ? "success" : "destructive"}>{l.status === "completed" ? "成功" : "失败"}</Badge></TableCell>
              <TableCell className="text-right font-mono text-xs">{formatAmount(l.cost)} CNY</TableCell>
              <TableCell className="text-right text-xs text-muted-foreground">{new Date(l.created_at).toLocaleString("zh-CN")}</TableCell>
            </TableRow>)}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
