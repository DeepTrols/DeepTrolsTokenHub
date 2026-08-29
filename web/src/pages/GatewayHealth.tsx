import { useAdminQuery } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Activity, RefreshCw } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

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

const STRATEGY_LABELS: Record<string, string> = {
  priority_only: "gatewayhealth.strategyPriority",
  cost: "gatewayhealth.strategyCost",
  quality: "gatewayhealth.strategyQuality",
  "": "gatewayhealth.strategyPriority",
};

export default function GatewayHealth() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: HealthRow[] }>("/gateway/health");
  const rows = data?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("gatewayhealth.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("gatewayhealth.subtitle")}</p>
        </div>
        <Button variant="outline" onClick={() => refetch()}><RefreshCw size={14} className="mr-1.5" />{t("gatewayhealth.refresh")}</Button>
      </div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">{loadError}</CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center text-muted-foreground">{t("gatewayhealth.loading")}</CardContent></Card>}

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("gatewayhealth.thChannel")}</TableHead>
              <TableHead>{t("gatewayhealth.thModel")}</TableHead>
              <TableHead>{t("gatewayhealth.thHealth")}</TableHead>
              <TableHead>{t("gatewayhealth.thStatus")}</TableHead>
              <TableHead>{t("gatewayhealth.thStrategy")}</TableHead>
              <TableHead>{t("gatewayhealth.thInstance")}</TableHead>
              <TableHead className="text-right">{t("gatewayhealth.thLoad")}</TableHead>
              <TableHead>{t("gatewayhealth.thCooldown")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 && (
              <TableRow><TableCell colSpan={8} className="py-12 text-center text-muted-foreground flex flex-col items-center gap-2">
                <Activity size={28} className="opacity-30" />{t("gatewayhealth.empty")}
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
                <TableCell>{t(STRATEGY_LABELS[r.strategy] ?? r.strategy)}{r.sticky_session ? t("gatewayhealth.sticky") : ""}</TableCell>
                <TableCell className="font-mono text-xs max-w-[200px] truncate">{r.base_url || "—"}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {r.current_load !== undefined ? `${r.current_load}/${r.concurrency_limit ?? "∞"}` : "—"}
                </TableCell>
                <TableCell className="text-xs">
                  {r.cooldown_until ? <span className="text-destructive">{t("gatewayhealth.until", { time: r.cooldown_until.replace("T", " ").slice(0, 16) })}</span> : "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
