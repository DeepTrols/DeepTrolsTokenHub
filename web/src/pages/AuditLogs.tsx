import { useAdminQuery } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { ScrollText } from "lucide-react";

interface AuditLog {
  id: string;
  actor_type: string;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id: string;
  new_value: unknown;
  reason: string;
  ip_address: string;
  created_at: string;
}

function summarize(value: unknown): string {
  if (value == null) return "—";
  const s = JSON.stringify(value);
  return s && s.length > 80 ? s.slice(0, 80) + "…" : s || "—";
}

export default function AuditLogs() {
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: AuditLog[] }>("/audit");
  const logs = data?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  if (isLoading) {
    return <Card><CardContent className="p-12 text-center text-muted-foreground">加载审计日志...</CardContent></Card>;
  }
  return (
    <div>
      <div className="mb-5">
        <h2 className="font-display text-[25px] font-bold tracking-tight">审计日志</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">全量管理员操作与内容策略拦截记录</p>
      </div>
      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">
        {loadError} <button onClick={() => refetch()} className="underline ml-2">重试</button>
      </CardContent></Card>}
      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>时间</TableHead>
              <TableHead>操作者</TableHead>
              <TableHead>动作</TableHead>
              <TableHead>资源</TableHead>
              <TableHead>详情</TableHead>
              <TableHead>IP</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {logs.length === 0 && (
              <TableRow><TableCell colSpan={6} className="py-12 text-center text-muted-foreground flex flex-col items-center gap-2">
                <ScrollText size={28} className="opacity-30" />暂无审计记录
              </TableCell></TableRow>
            )}
            {logs.map((l) => (
              <TableRow key={l.id}>
                <TableCell className="text-xs whitespace-nowrap">{l.created_at?.replace("T", " ").slice(0, 19)}</TableCell>
                <TableCell className="text-sm">{l.actor_email || l.actor_type}</TableCell>
                <TableCell><Badge variant="secondary">{l.action}</Badge></TableCell>
                <TableCell className="text-xs">{l.resource_type}{l.resource_id ? ":" + l.resource_id.slice(0, 8) : ""}</TableCell>
                <TableCell className="text-xs font-mono max-w-[260px] truncate">{summarize(l.new_value)}</TableCell>
                <TableCell className="text-xs font-mono">{l.ip_address || "—"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
