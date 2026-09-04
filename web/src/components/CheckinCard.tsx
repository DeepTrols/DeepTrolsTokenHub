import { toast } from "sonner";
import { CalendarCheck2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface CheckinStatus {
  enabled: boolean;
  min_quota: string;
  max_quota: string;
  checked_in_today: boolean;
  total_days: number;
}

/**
 * Daily sign-in card (port of new-api's CheckinCalendarCard, simplified for
 * 智曜TokenHub's /checkin/status + /checkin endpoints). Shows today's state, the
 * configured reward range and a check-in action; credits land in the wallet.
 */
export default function CheckinCard() {
  const { t } = useTranslation();
  const statusQuery = useConsoleQuery<CheckinStatus>("/checkin/status", {
    staleTime: 30_000,
  });

  const checkin = useConsoleMutation<{ ok: boolean; amount: string }, void>(
    "post",
    "/checkin",
    "/wallet",
    {
      onSuccess: (r) => {
        toast.success(t("components.checkinSuccess", { amount: r.amount }));
        statusQuery.refetch();
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : t("components.checkinFailed")),
    },
  );

  const status = statusQuery.data;
  const enabled = status?.enabled ?? true;
  const range =
    status && status.min_quota !== status.max_quota
      ? t("components.checkinRewardRange", { range: `${status.min_quota} - ${status.max_quota} 元` })
      : status
        ? t("components.checkinRewardRange", { range: `${status.min_quota} 元` })
        : "";

  return (
    <div className="glass rounded-[22px] p-5">
      <div className="flex items-center justify-between gap-4 mb-4">
        <div className="flex items-center gap-3">
          <span className="nav-ic !w-10 !h-10 !rounded-xl bg-gradient-to-br from-[#F78B28] to-[#E85D3F] text-white border-0 shadow-[0_6px_16px_rgba(247,139,40,0.35)]">
            <CalendarCheck2 size={18} />
          </span>
          <div>
            <h3 className="font-display font-semibold">{t("components.checkinTitle")}</h3>
            <p className="text-xs text-[#5C6472]">
              {status
                ? range
                  ? range
                  : t("components.checkinClaim")
                : t("components.checkinClaim")}
            </p>
          </div>
        </div>
        <span className="text-[12px] font-semibold text-[#5C6472] bg-white/60 rounded-full px-3 py-1">
          {t("components.checkinCumulative", { count: status?.total_days ?? 0 })}
        </span>
      </div>

      {enabled ? (
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm text-[#5C6472]">
            {status?.checked_in_today
              ? t("components.checkinCheckedToday")
              : t("components.checkinPrompt")}
          </p>
          <Button
            variant={status?.checked_in_today ? "outline" : "default"}
            onClick={() => checkin.mutate()}
            disabled={checkin.isPending || status?.checked_in_today}
            className="shrink-0"
          >
            <Sparkles size={15} className="mr-1.5" />
            {checkin.isPending ? t("components.checkinChecking") : status?.checked_in_today ? t("components.checkinChecked") : t("components.checkinBtn")}
          </Button>
        </div>
      ) : (
        <p className="text-sm text-[#8C93A1]">{t("components.checkinDisabled")}</p>
      )}
    </div>
  );
}
