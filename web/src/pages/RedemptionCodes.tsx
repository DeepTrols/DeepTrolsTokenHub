import { useState } from "react";
import { toast } from "sonner";
import { Copy, TicketPlus, ClipboardCopy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import i18n from "../i18n";
import { useTranslation } from "react-i18next";

export interface RedemptionCode {
  code: string;
  amount: string;
  status: string;
  created_at: string;
  used_at?: string;
  used_by_email?: string;
}

const statusLabel: Record<string, string> = {
  active: "redemption.statusActive",
  used: "redemption.statusUsed",
  expired: "redemption.statusExpired",
};

function copyText(text: string) {
  navigator.clipboard?.writeText(text).catch(() => undefined);
  toast.success(i18n.t("redemption.copied", { text }));
}

/**
 * Admin redemption-code management (port of new-api's Redemptions feature:
 * create batch codes, list them with used-by info, copy codes to clipboard).
 */
export default function RedemptionCodes() {
  const { t } = useTranslation();
  const listQuery = useAdminQuery<{ codes: RedemptionCode[] }>("/redemption");
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState("10");
  const [count, setCount] = useState("10");

  const create = useAdminMutation<
    { created: number; codes: string[] },
    { amount: string; count: number }
  >("post", "/redemption", "/redemption", {
    onSuccess: (r) => {
      toast.success(t("redemption.createdN", { count: r.created }));
      setOpen(false);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : t("redemption.createFailed")),
  });

  const codes = listQuery.data?.codes ?? [];
  const activeCount = codes.filter((c) => c.status === "active").length;

  const handleCreate = () => {
    const n = Number(count);
    if (!amount || Number(amount) <= 0) {
      toast.error(t("redemption.invalidAmount"));
      return;
    }
    if (!Number.isInteger(n) || n <= 0 || n > 5000) {
      toast.error(t("redemption.invalidCount"));
      return;
    }
    create.mutate({ amount, count: n });
  };

  return (
    <div>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("redemption.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("redemption.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button
            variant="outline"
            onClick={() => copyText(codes.filter((c) => c.status === "active").map((c) => c.code).join("\n"))}
            disabled={activeCount === 0}
          >
            <ClipboardCopy size={15} className="mr-1.5" />
            {t("redemption.copyUnused", { count: activeCount })}
          </Button>
          <Button onClick={() => setOpen(true)}>
            <TicketPlus size={15} className="mr-1.5" />
            {t("redemption.generate")}
          </Button>
        </div>
      </div>

      {listQuery.isLoading ? (
        <LoadingState message={t("redemption.loading")} />
      ) : listQuery.isError ? (
        <ErrorState error={listQuery.error} onRetry={() => listQuery.refetch()} title={t("redemption.loadFailed")} />
      ) : codes.length === 0 ? (
        <EmptyState
          title={t("redemption.empty")}
          description={t("redemption.emptyDesc")}
          action={
            <Button onClick={() => setOpen(true)}>
              <TicketPlus size={15} className="mr-1.5" />
              {t("redemption.generate")}
            </Button>
          }
        />
      ) : (
        <div className="glass rounded-[22px] p-4 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("redemption.thCode")}</TableHead>
                <TableHead>{t("redemption.thAmount")}</TableHead>
                <TableHead>{t("redemption.thStatus")}</TableHead>
                <TableHead>{t("redemption.thUsedBy")}</TableHead>
                <TableHead>{t("redemption.thCreated")}</TableHead>
                <TableHead>{t("redemption.thUsedAt")}</TableHead>
                <TableHead className="text-right">{t("redemption.thActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {codes.map((c) => (
                <TableRow key={c.code}>
                  <TableCell className="font-mono text-[12.5px]">{c.code}</TableCell>
                  <TableCell>{t("redemption.amountYuan", { amount: c.amount })}</TableCell>
                  <TableCell>
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[11.5px] font-semibold ${
                        c.status === "active"
                          ? "bg-[#1BA878]/10 text-[#0C7A55]"
                          : c.status === "used"
                            ? "bg-[#F78B28]/10 text-primary-700"
                            : "bg-[#8C93A1]/10 text-[#5C6472]"
                      }`}
                    >
                      {t(statusLabel[c.status] ?? c.status)}
                    </span>
                  </TableCell>
                  <TableCell className="text-[#5C6472]">{c.used_by_email ?? "—"}</TableCell>
                  <TableCell className="text-[#5C6472]">{c.created_at}</TableCell>
                  <TableCell className="text-[#5C6472]">{c.used_at ?? "—"}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => copyText(c.code)}>
                      <Copy size={13} className="mr-1" />
                      {t("redemption.copy")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("redemption.dialogTitle")}</DialogTitle>
            <DialogDescription>{t("redemption.dialogDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="redemption-amount">{t("redemption.amountLabel")}</Label>
              <Input
                id="redemption-amount"
                type="number"
                min="0.01"
                step="0.01"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder="10"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="redemption-count">{t("redemption.countLabel")}</Label>
              <Input
                id="redemption-count"
                type="number"
                min="1"
                max="5000"
                value={count}
                onChange={(e) => setCount(e.target.value)}
                placeholder="10"
              />
              <p className="text-xs text-[#5C6472]">{t("redemption.countHint")}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={create.isPending}>
              {t("redemption.cancel")}
            </Button>
            <Button onClick={handleCreate} disabled={create.isPending}>
              {create.isPending ? t("redemption.generating") : t("redemption.generateBtn")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
