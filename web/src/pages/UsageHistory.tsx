import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { UsageLog } from "../lib/api";
import { Download } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

const PAGE = 50;

export default function UsageHistory() {
  const { t } = useTranslation();
  const [offset, setOffset] = useState(0);
  const { data, isLoading, refetch } = useConsoleQuery<{ data: UsageLog[] }>(`/usage?limit=${PAGE}&offset=${offset}`);
  const logs = data?.data ?? [];

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("usagehistory.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("usagehistory.subtitle")}</p>
      </div>
      <div className="mb-4">
        <Button variant="outline" size="sm" onClick={() => exportCsv(logs, t)}>
          <Download size={14} className="mr-1.5" />{t("usagehistory.exportCsv")}
        </Button>
      </div>
      <Card className="overflow-hidden">
        <CardContent className="p-0">
          {isLoading ? (
            <p className="p-10 text-center text-muted-foreground">{t("usagehistory.loading")}</p>
          ) : logs.length === 0 ? (
            <p className="p-10 text-center text-muted-foreground">{t("usagehistory.empty")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-[#5C6472]">
                    <th className="px-4 py-2">{t("usagehistory.thTime")}</th>
                    <th className="px-4 py-2">{t("usagehistory.thModel")}</th>
                    <th className="px-4 py-2">{t("usagehistory.thInOut")}</th>
                    <th className="px-4 py-2">{t("usagehistory.thCost")}</th>
                    <th className="px-4 py-2">{t("usagehistory.thStatus")}</th>
                    <th className="px-4 py-2">Request ID</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-black/10">
                  {logs.map((l) => (
                    <tr key={l.request_id}>
                      <td className="px-4 py-2 text-xs">{l.created_at?.replace("T", " ").slice(0, 19)}</td>
                      <td className="px-4 py-2 font-mono text-xs">{l.model}</td>
                      <td className="px-4 py-2">{l.input_tokens} / {l.output_tokens}</td>
                      <td className="px-4 py-2">{l.cost}</td>
                      <td className="px-4 py-2 text-xs">{l.status}</td>
                      <td className="px-4 py-2 font-mono text-xs">{l.request_id?.slice(0, 12)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      <div className="mt-4 flex gap-3">
        <Button variant="outline" size="sm" disabled={offset === 0 || isLoading} onClick={() => setOffset(Math.max(0, offset - PAGE))}>
          {t("usagehistory.prev")}
        </Button>
        <Button variant="outline" size="sm" disabled={logs.length < PAGE} onClick={() => setOffset(offset + PAGE)}>
          {t("usagehistory.next")}
        </Button>
      </div>
    </div>
  );
}

function exportCsv(rows: UsageLog[], t: (key: string) => string) {
  const header = [
    t("usagehistory.csvTime"),
    t("usagehistory.csvModel"),
    t("usagehistory.csvInput"),
    t("usagehistory.csvOutput"),
    t("usagehistory.csvCost"),
    t("usagehistory.csvStatus"),
    t("usagehistory.csvReq"),
  ];
  const lines = rows.map((l) =>
    [l.created_at, l.model, l.input_tokens, l.output_tokens, l.cost, l.status, l.request_id].map(escCsv).join(","),
  );
  const blob = new Blob(["\uFEFF" + [header.join(","), ...lines].join("\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = t("usagehistory.csvFilename");
  a.click();
  URL.revokeObjectURL(url);
}

function escCsv(v: unknown): string {
  const s = String(v ?? "");
  return /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}
