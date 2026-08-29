import { TopupTable } from "@/components/TopupTable";
import { ErrorState, LoadingState } from "@/components/StateViews";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { Transaction } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Download } from "lucide-react";
import { toast } from "sonner";
import { useEffect, useState } from "react";
import "../i18n";
import { useTranslation } from "react-i18next";

interface MonthlyStatement {
  year: number;
  month: number;
  total_cost: string;
  total_topup: string;
  charge_count: number;
  by_model: Array<{ model: string; cost: string; count: number }>;
}

export default function Bills() {
  const { t } = useTranslation();
  const {
    data: txData,
    isLoading,
    isError,
    error,
    refetch,
  } = useConsoleQuery<{ data: Transaction[] }>("/wallet/transactions");
  const topups = (txData?.data ?? []).filter((t) => t.type === "topup");
  const alertQuery = useConsoleQuery<{ threshold: string }>("/wallet/alert");
  const [alertThreshold, setAlertThreshold] = useState("");
  const [month, setMonth] = useState(() => new Date(Date.now() + 8 * 3600 * 1000).toISOString().slice(0, 7));
  const [stmtYear, stmtMonth] = month.split("-").map(Number);
  const statementQuery = useConsoleQuery<MonthlyStatement>(
    `/billing/statement?year=${stmtYear}&month=${stmtMonth}`,
    { enabled: Number.isInteger(stmtYear) && Number.isInteger(stmtMonth) },
  );

  useEffect(() => {
    if (alertQuery.data) setAlertThreshold(alertQuery.data.threshold);
  }, [alertQuery.data]);

  const alertSave = useConsoleMutation<{ threshold: string }, { threshold: string }>(
    "put",
    "/wallet/alert",
    "/wallet",
    {
      onSuccess: (r) => {
        setAlertThreshold(r.threshold);
        toast.success(t("bills.alertSaved"));
      },
    },
  );

  if (isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("bills.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("bills.subtitle")}</p>
        </div>
        <LoadingState message={t("bills.loading")} />
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("bills.title")}</h2>
        </div>
        <ErrorState error={error} onRetry={() => refetch()} title={t("bills.loadFailed")} />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("bills.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("bills.subtitle")}</p>
        <Button variant="outline" size="sm" className="mt-2" onClick={() => exportCsv(topups, t)}>
          <Download size={14} className="mr-1.5" />{t("bills.exportCsv")}
        </Button>
      </div>

      <div className="glass rounded-[22px] p-5 mb-5">
        <h3 className="font-display font-semibold mb-1">{t("bills.alertTitle")}</h3>
        <p className="text-xs text-[#5C6472] mb-3">{t("bills.alertDesc")}</p>
        <div className="flex items-end gap-3 max-w-md">
          <div className="space-y-1.5 flex-1">
            <Label htmlFor="balance-alert-threshold">
              {alertQuery.data && alertQuery.data.threshold !== "0.00"
                ? t("bills.alertCurrent", { threshold: alertQuery.data.threshold })
                : t("bills.alertOff")}
            </Label>
            <Input
              id="balance-alert-threshold"
              type="number"
              min="0"
              step="0.01"
              value={alertThreshold}
              onChange={(e) => setAlertThreshold(e.target.value)}
              placeholder={t("bills.alertPlaceholder")}
              className="font-mono"
            />
          </div>
          <Button onClick={() => alertSave.mutate({ threshold: alertThreshold })} disabled={alertSave.isPending}>
            {t("bills.alertSave")}
          </Button>
        </div>
      </div>

      <div className="glass rounded-[22px] p-5 mb-5">
        <div className="flex items-center justify-between gap-4 mb-4">
          <div>
            <h3 className="font-display font-semibold">{t("bills.statementTitle")}</h3>
            <p className="text-xs text-[#5C6472] mt-0.5">{t("bills.statementDesc")}</p>
          </div>
          <input
            type="month"
            value={month}
            onChange={(e) => e.target.value && setMonth(e.target.value)}
            aria-label={t("bills.statementMonthLabel")}
            className="glass-soft rounded-lg px-3 py-2 text-sm"
          />
        </div>
        {statementQuery.data ? (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-4">
              <div className="glass-soft rounded-xl p-3">
                <p className="text-[11px] text-[#5C6472]">{t("bills.statementCost")}</p>
                <p className="font-mono text-[18px] font-semibold text-[#161A23]">¥{statementQuery.data.total_cost}</p>
              </div>
              <div className="glass-soft rounded-xl p-3">
                <p className="text-[11px] text-[#5C6472]">{t("bills.statementTopup")}</p>
                <p className="font-mono text-[18px] font-semibold text-[#0C7A55]">¥{statementQuery.data.total_topup}</p>
              </div>
              <div className="glass-soft rounded-xl p-3">
                <p className="text-[11px] text-[#5C6472]">{t("bills.statementCharges")}</p>
                <p className="font-mono text-[18px] font-semibold text-[#161A23]">{statementQuery.data.charge_count}</p>
              </div>
            </div>
            {statementQuery.data.by_model.length > 0 ? (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-[#5C6472] border-b border-black/10">
                    <th className="py-2 pr-3">{t("bills.statementModel")}</th>
                    <th className="py-2 pr-3 text-right">{t("bills.statementCount")}</th>
                    <th className="py-2 text-right">{t("bills.statementModelCost")}</th>
                  </tr>
                </thead>
                <tbody>
                  {statementQuery.data.by_model.map((m) => (
                    <tr key={m.model} className="border-b border-black/[0.04]">
                      <td className="py-2 pr-3 font-mono text-xs">{m.model}</td>
                      <td className="py-2 pr-3 text-right text-[#5C6472]">{m.count}</td>
                      <td className="py-2 text-right font-medium">¥{m.cost}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="text-sm text-[#5C6472]">{t("bills.statementEmpty")}</p>
            )}
          </>
        ) : (
          <p className="text-sm text-[#5C6472]">{statementQuery.isLoading ? t("common.loading") : "—"}</p>
        )}
      </div>

      <div className="glass rounded-[22px] p-5">
        <h3 className="font-display font-semibold mb-4">{t("bills.records")}</h3>
        <TopupTable topups={topups} />
      </div>
    </div>
  );
}

function exportCsv(rows: Transaction[], t: (key: string) => string) {
  const header = [
    t("bills.csvOrderNo"),
    t("bills.csvAmount"),
    t("bills.csvBalance"),
    t("bills.csvStatus"),
    t("bills.csvMethod"),
    t("bills.csvTime"),
  ];
  const lines = rows.map((t) =>
    [t.order_no, t.amount, t.balance_after, t.status, t.payment_method, t.created_at].map(esc).join(","),
  );
  const blob = new Blob(["\uFEFF" + [header.join(","), ...lines].join("\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = t("bills.csvFilename");
  a.click();
  URL.revokeObjectURL(url);
}

function esc(v: unknown): string {
  const s = String(v ?? "");
  return /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}
