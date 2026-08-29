import { useAdminQuery } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { ScrollText } from "lucide-react";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import "../i18n";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: AuditLog[] }>("/audit");
  const logs = data?.data ?? [];
  const [q, setQ] = useState("");
  const filtered = logs.filter((l) => {
    if (!q) return true;
    const s = q.toLowerCase();
    return (l.action + l.resource_type + (l.actor_email || "") + l.ip_address).toLowerCase().includes(s);
  });
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  if (isLoading) {
    return <Card><CardContent className="p-12 text-center text-muted-foreground">{t("auditlogs.loading")}</CardContent></Card>;
  }
  return (
    <div>
      <div className="mb-5">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("auditlogs.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("auditlogs.subtitle")}</p>
      </div>
      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">
        {loadError} <button onClick={() => refetch()} className="underline ml-2">{t("auditlogs.retry")}</button>
      </CardContent></Card>}
      <div className="mb-4">
        <Input placeholder={t("auditlogs.searchPlaceholder")} value={q} onChange={(e) => setQ(e.target.value)} className="max-w-[360px]" />
      </div>
      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("auditlogs.thTime")}</TableHead>
              <TableHead>{t("auditlogs.thActor")}</TableHead>
              <TableHead>{t("auditlogs.thAction")}</TableHead>
              <TableHead>{t("auditlogs.thResource")}</TableHead>
              <TableHead>{t("auditlogs.thDetail")}</TableHead>
              <TableHead>{t("auditlogs.thIp")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 && (
              <TableRow><TableCell colSpan={6} className="py-12 text-center text-muted-foreground flex flex-col items-center gap-2">
                <ScrollText size={28} className="opacity-30" />{t("auditlogs.empty")}
              </TableCell></TableRow>
            )}
            {filtered.map((l) => (
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
