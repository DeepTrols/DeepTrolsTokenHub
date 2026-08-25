import { useState } from "react";
import { adminApi } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { GitBranch, Loader2 } from "lucide-react";

interface SimulatedRoute {
  channel_id: string;
  channel_name: string;
  health_score: number;
  health_status: string;
  strategy: string;
  sticky_session: boolean;
  instance_id: string;
  base_url: string;
  upstream_model: string;
  current_load: number;
}

const STRATEGY_LABELS: Record<string, string> = {
  priority_only: "优先级",
  cost: "成本",
  quality: "质量",
  "": "优先级",
};

export default function RoutingSimulator() {
  const [model, setModel] = useState("deepseek-chat");
  const [tenantId, setTenantId] = useState("");
  const [routes, setRoutes] = useState<SimulatedRoute[] | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const run = async () => {
    setLoading(true);
    setError("");
    try {
      const res = await adminApi.post<{ data: SimulatedRoute[] }>("/routing/simulate", {
        model,
        tenant_id: tenantId.trim() || undefined,
      });
      setRoutes(res.data ?? []);
    } catch (e) {
      setRoutes(null);
      setError(e instanceof Error ? e.message : "模拟失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="mb-5">
        <h2 className="font-display text-[25px] font-bold tracking-tight">路由模拟器</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">预览模型（可选租户）在当前策略下的有序路由候选</p>
      </div>

      <Card className="mb-5">
        <CardContent className="p-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 items-end">
            <div className="space-y-2">
              <Label>模型</Label>
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="deepseek-chat" />
            </div>
            <div className="space-y-2">
              <Label>租户 ID（可选）</Label>
              <Input value={tenantId} onChange={(e) => setTenantId(e.target.value)} placeholder="留空 = 平台视角" />
            </div>
            <Button onClick={run} disabled={loading || !model.trim()}>
              {loading ? <Loader2 size={16} className="mr-1.5 animate-spin" /> : <GitBranch size={16} className="mr-1.5" />}
              模拟路由
            </Button>
          </div>
        </CardContent>
      </Card>

      {error && <Card className="mb-5 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">{error}</CardContent></Card>}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>#</TableHead>
              <TableHead>渠道</TableHead>
              <TableHead>健康</TableHead>
              <TableHead>策略</TableHead>
              <TableHead>粘性</TableHead>
              <TableHead>上游模型</TableHead>
              <TableHead className="text-right">Base URL</TableHead>
              <TableHead className="text-right">负载</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {routes === null && (
              <TableRow><TableCell colSpan={8} className="py-10 text-center text-muted-foreground">输入模型后点击「模拟路由」</TableCell></TableRow>
            )}
            {routes !== null && routes.length === 0 && (
              <TableRow><TableCell colSpan={8} className="py-10 text-center text-muted-foreground">无可路由渠道</TableCell></TableRow>
            )}
            {(routes ?? []).map((r, i) => (
              <TableRow key={r.channel_id + i}>
                <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                <TableCell className="font-medium">{r.channel_name || r.channel_id.slice(0, 8)}</TableCell>
                <TableCell>
                  <span className={r.health_score >= 70 ? "text-[#0C7A55]" : r.health_score >= 30 ? "text-[#A06B12]" : "text-destructive"}>
                    {r.health_score} · {r.health_status}
                  </span>
                </TableCell>
                <TableCell>{STRATEGY_LABELS[r.strategy] ?? r.strategy}</TableCell>
                <TableCell>{r.sticky_session ? "是" : "—"}</TableCell>
                <TableCell className="font-mono text-xs">{r.upstream_model}</TableCell>
                <TableCell className="text-right font-mono text-xs max-w-[220px] truncate">{r.base_url}</TableCell>
                <TableCell className="text-right tabular-nums">{r.current_load}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
