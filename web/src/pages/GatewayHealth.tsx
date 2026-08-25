import { useAdminQuery } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Activity, RefreshCw } from "lucide-react";

interface HealthRow {
  channel_id: string;
  channel_name: string;
  model_code: string;
  pool_type: string;
  health_score: number;
  health_status: string;
  channel_status: string;
  strategy: string;
  sticky_session: boolean;
  weight: number;
  instance_id?: string;
  base_url?: string;
  current_load?: number;
  concurrency_limit?: number;
  cooldown_until?: string;
  last_checked_at?: string;
}

const STRATEGY_LABELS: Record<string, string> = { priority_only: "优先级", cost: "成本", quality: "质量", "": "优先级" };

export default function GatewayHealth() {
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: HealthRow[] }>("/gateway/health");
  const rows = data?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">网关健康</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">渠道与实例的实时健康、负载与冷却状态</p>
        </div>
        <Button variant="outline" onClick={() => refetch()}><RefreshCw size={14} className="mr-1.5" />刷新</Button>
      </div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">{loadError}</CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center text-muted-foreground">加载中...</CardContent></Card>}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>渠道</TableHead>
              <TableHead>模型</TableHead>
              <TableHead>健康</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>策略</TableHead>
              <TableHead>实例</TableHead>
              <TableHead className="text-right">负载/上限</TableHead>
              <TableHead>冷却</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 && (
              <TableRow><TableCell colSpan={8} className="py-12 text-center text-muted-foreground flex flex-col items-center gap-2">
                <Activity size={28} className="opacity-30" />暂无渠道
              </TableCell></TableRow>
            )}
            {rows.map((r) => (
              <TableRow key={r.channel_id + (r.instance_id ?? "")}>
                <TableCell className="font-medium">{r.channel_name}</TableCell>
                <TableCell className="font-mono text-xs">{r.model_code}</TableCell>
                <TableCell>
                  <span className={r.health_score >= 70 ? "text-[#0C7A55]" : r.health_score >= 30 ? "text-[#A06B12]" : "text-destructive"}>
                    {r.health_score} · {r.health_status}
                  </span>
                </TableCell>
                <TableCell><Badge variant={r.channel_status === "active" ? "success" : "secondary"}>{r.channel_status}</Badge></TableCell>
                <TableCell>{STRATEGY_LABELS[r.strategy] ?? r.strategy}{r.sticky_session ? " · 粘性" : ""}</TableCell>
                <TableCell className="font-mono text-xs max-w-[200px] truncate">{r.base_url || "—"}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {r.current_load !== undefined ? `${r.current_load}/${r.concurrency_limit ?? "∞"}` : "—"}
                </TableCell>
                <TableCell className="text-xs">
                  {r.cooldown_until ? <span className="text-destructive">至 {r.cooldown_until.replace("T", " ").slice(0, 16)}</span> : "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
