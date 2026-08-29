import { toast } from "sonner";
import { Ban } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { useAdminMutation, useAdminQuery } from "@/lib/hooks/use-api";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface AdminSubscriptionRow {
  id: string;
  user_email: string;
  plan_name: string;
  price: string;
  starts_at: string;
  expires_at: string;
  status: string;
}

const STATUS_LABEL: Record<string, string> = {
  active: "adminsubs.statusActive",
  expired: "adminsubs.statusExpired",
  cancelled: "adminsubs.statusCancelled",
};

export default function AdminSubscriptions() {
  const { t } = useTranslation();
  const listQuery = useAdminQuery<{ data: AdminSubscriptionRow[] }>("/subscriptions");
  const cancel = useAdminMutation<{ ok: boolean }, { id: string }>(
    "post",
    (v) => `/subscriptions/${v.id}/cancel`,
    "/subscriptions",
    {
      onSuccess: () => toast.success(t("adminsubs.cancelled")),
      onError: (e) => toast.error(e instanceof Error ? e.message : t("adminsubs.cancelFailed")),
    },
  );

  const rows = listQuery.data?.data ?? [];

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("adminsubs.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("adminsubs.subtitle")}</p>
      </div>

      {listQuery.isLoading ? (
        <LoadingState message={t("adminsubs.loading")} />
      ) : listQuery.isError ? (
        <ErrorState error={listQuery.error} onRetry={() => listQuery.refetch()} title={t("adminsubs.loadFailed")} />
      ) : rows.length === 0 ? (
        <EmptyState title={t("adminsubs.empty")} />
      ) : (
        <div className="glass rounded-[22px] p-4 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("adminsubs.thUser")}</TableHead>
                <TableHead>{t("adminsubs.thPlan")}</TableHead>
                <TableHead>{t("adminsubs.thPrice")}</TableHead>
                <TableHead>{t("adminsubs.thStart")}</TableHead>
                <TableHead>{t("adminsubs.thEnd")}</TableHead>
                <TableHead>{t("adminsubs.thStatus")}</TableHead>
                <TableHead className="text-right">{t("adminsubs.thActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((s) => (
                <TableRow key={s.id}>
                  <TableCell className="text-[#5C6472]">{s.user_email}</TableCell>
                  <TableCell className="font-medium">{s.plan_name}</TableCell>
                  <TableCell>¥{s.price}</TableCell>
                  <TableCell className="text-[#5C6472]">{new Date(s.starts_at).toLocaleDateString()}</TableCell>
                  <TableCell className="text-[#5C6472]">{new Date(s.expires_at).toLocaleDateString()}</TableCell>
                  <TableCell>
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[11.5px] font-semibold ${
                        s.status === "active"
                          ? "bg-[#1BA878]/10 text-[#0C7A55]"
                          : s.status === "expired"
                            ? "bg-[#8C93A1]/10 text-[#5C6472]"
                            : "bg-[#E5484D]/10 text-[#C4372C]"
                      }`}
                    >
                      {t(STATUS_LABEL[s.status] ?? s.status)}
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
                    {s.status === "active" && (
                      <Button variant="ghost" size="sm" onClick={() => cancel.mutate({ id: s.id })} disabled={cancel.isPending}>
                        <Ban size={13} className="mr-1 text-[#C4372C]" /> {t("adminsubs.cancel")}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
