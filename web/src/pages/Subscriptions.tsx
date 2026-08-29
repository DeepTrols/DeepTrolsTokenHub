import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, Crown, Wallet } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { PaymentQrDialog } from "@/components/PaymentQrDialog";
import { Switch } from "@/components/ui/switch";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface SubscriptionPlan {
  id: string;
  name: string;
  description: string;
  price: string;
  duration_days: number;
  group_name?: string;
  token_quota?: number;
  sort_order: number;
}

export interface UserSubscription {
  id: string;
  plan_id: string;
  plan_name: string;
  price: string;
  starts_at: string;
  expires_at: string;
  status: string;
  auto_renew: boolean;
}

function durationLabel(days: number, t: (key: string, opts?: Record<string, unknown>) => string): string {
  if (days % 365 === 0) return t("subscriptions.durationYear", { n: days / 365 });
  if (days % 30 === 0) return t("subscriptions.durationMonth", { n: days / 30 });
  if (days % 7 === 0) return t("subscriptions.durationWeek", { n: days / 7 });
  return t("subscriptions.durationDay", { n: days });
}

export default function Subscriptions() {
  const { t } = useTranslation();
  const plansQuery = useConsoleQuery<{ data: SubscriptionPlan[] }>("/subscription/plans");
  const selfQuery = useConsoleQuery<{
    subscriptions: UserSubscription[];
    all_subscriptions: UserSubscription[];
  }>("/subscription/self");
  const walletQuery = useConsoleQuery<{ balance: string }>("/wallet");
  const [pendingPlan, setPendingPlan] = useState<SubscriptionPlan | null>(null);
  const [payOrder, setPayOrder] = useState<{ orderNo: string; payURL: string; paid: boolean } | null>(null);
  const [autoRenewConsent, setAutoRenewConsent] = useState(false);

  const purchase = useConsoleMutation<
    { ok: boolean; plan_name: string; price: string; expires_at: string },
    { plan_id: string; auto_renew?: boolean }
  >(
    "post",
    "/subscription/purchase",
    "/subscription/self",
    {
      onSuccess: (r) => {
        toast.success(t("subscriptions.purchaseSuccess", { name: r.plan_name, date: new Date(r.expires_at).toLocaleDateString() }));
        setPendingPlan(null);
        selfQuery.refetch();
        walletQuery.refetch();
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : t("subscriptions.purchaseFailed")),
    },
  );
  const setAutoRenew = useConsoleMutation<{ ok: boolean; enabled: boolean }, { enabled: boolean }>(
    "post",
    "/subscription/auto-renew",
    "/subscription/self",
    {
      onSuccess: (r) => toast.success(r.enabled ? t("subscriptions.renewEnabled") : t("subscriptions.renewDisabled")),
      onError: (e) => toast.error(e instanceof Error ? e.message : t("subscriptions.renewUpdateFailed")),
    },
  );

  const createOrder = useConsoleMutation<
    { order_no: string; pay_url: string },
    { plan_id: string; pay_method: string }
  >("post", "/subscription/order", "/subscription/self", {
    onSuccess: (r) => {
      setPayOrder({ orderNo: r.order_no, payURL: r.pay_url, paid: false });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : t("subscriptions.orderFailed")),
  });

  const ordersQuery = useConsoleQuery<{ orders: Array<{ order_no: string; status: string }> }>("/payment/orders", {
    refetchInterval: payOrder && !payOrder.paid ? 3000 : false,
  });

  useEffect(() => {
    if (!payOrder || payOrder.paid) return;
    const found = ordersQuery.data?.orders?.find((o) => o.order_no === payOrder.orderNo);
    if (found && found.status === "paid") {
      setPayOrder((prev) => (prev ? { ...prev, paid: true } : prev));
      toast.success(t("subscriptions.paySuccess"));
      selfQuery.refetch();
      walletQuery.refetch();
    }
  }, [ordersQuery.data, payOrder, selfQuery, walletQuery]);

  const plans = plansQuery.data?.data ?? [];
  const active = (selfQuery.data?.subscriptions ?? []).filter((s) => s.status === "active");
  const latestActive = active[active.length - 1];
  const history = selfQuery.data?.all_subscriptions ?? [];

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("subscriptions.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("subscriptions.subtitle")}</p>
      </div>

      {plansQuery.isLoading || selfQuery.isLoading ? (
        <LoadingState message={t("subscriptions.loading")} />
      ) : plansQuery.isError ? (
        <ErrorState error={plansQuery.error} onRetry={() => plansQuery.refetch()} title={t("subscriptions.loadFailed")} />
      ) : (
        <>
          {active.length > 0 && (
            <div className="mb-5 p-4 glass-soft rounded-2xl border-[#0FA88B]/30">
              <div className="flex items-center gap-3">
                <span className="grid w-10 h-10 place-items-center rounded-xl bg-gradient-to-br from-[#0FA88B] to-[#35A7FF] text-white">
                  <Crown size={17} />
                </span>
                <div>
                  <div className="text-sm font-bold text-[#0C7A55]">
                    {t("subscriptions.currentPlan", { name: active[active.length - 1].plan_name })}
                  </div>
                  <div className="text-[12px] text-[#5C6472]">
                    {t("subscriptions.validUntil", { date: new Date(active[active.length - 1].expires_at).toLocaleDateString() })}
                  </div>
                </div>
                <div className="ml-auto text-[12px] text-[#5C6472] flex items-center gap-1">
                  <Wallet size={13} /> {t("subscriptions.balance", { balance: walletQuery.data?.balance ?? "—" })}
                </div>
              </div>
              {latestActive && (
                <div className="mt-3 flex items-center justify-between gap-4 border-t border-black/5 pt-3">
                  <div>
                    <div className="text-[13px] font-semibold text-[#161A23]">{t("subscriptions.autoRenew")}</div>
                    <div className="text-[11.5px] text-[#5C6472]">{t("subscriptions.autoRenewDesc")}</div>
                  </div>
                  <Switch
                    checked={latestActive.auto_renew}
                    onCheckedChange={(v) => setAutoRenew.mutate({ enabled: v })}
                    disabled={setAutoRenew.isPending}
                  />
                </div>
              )}
            </div>
          )}

          {plans.length === 0 ? (
            <EmptyState title={t("subscriptions.emptyPlans")} description={t("subscriptions.emptyPlansDesc")} />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              {plans.map((p) => (
                <div key={p.id} className="glass rounded-[22px] p-6 flex flex-col">
                  <div className="flex items-center justify-between">
                    <h3 className="font-display font-semibold text-[17px]">{p.name}</h3>
                    <span className="text-[11px] font-semibold text-[#8C93A1] bg-white/60 rounded-full px-2.5 py-1">
                      {durationLabel(p.duration_days, t)}
                    </span>
                  </div>
                  <div className="mt-3 flex items-baseline gap-1">
                    <span className="text-[28px] font-bold tracking-tight">¥{p.price}</span>
                    <span className="text-[12px] text-[#5C6472]">/ {durationLabel(p.duration_days, t)}</span>
                  </div>
                  {p.group_name && (
                    <span className="mt-2 inline-flex w-fit items-center rounded-full bg-[#4F6BED]/10 text-[#4F6BED] text-[11.5px] font-semibold px-2.5 py-0.5">
                      {t("subscriptions.includesGroup", { name: p.group_name })}
                    </span>
                  )}
                  {p.token_quota ? (
                    <span className="mt-2 inline-flex w-fit items-center rounded-full bg-[#1BA878]/10 text-[#0C7A55] text-[11.5px] font-semibold px-2.5 py-0.5">
                      {t("subscriptions.freeTokens", { n: (p.token_quota / 10000).toFixed(0) })}
                    </span>
                  ) : null}
                  <p className="mt-3 text-[13px] text-[#5C6472] flex-1">{p.description || t("subscriptions.defaultDesc")}</p>
                  <Button
                    className="mt-5"
                    variant={active.length > 0 ? "outline" : "default"}
                    onClick={() => setPendingPlan(p)}
                  >
                    <Check size={15} className="mr-1.5" />
                    {active.length > 0 ? t("subscriptions.renewAdd") : t("subscriptions.activateNow")}
                  </Button>
                  <Button
                    className="mt-2"
                    variant="outline"
                    onClick={() => createOrder.mutate({ plan_id: p.id, pay_method: "alipay" })}
                    disabled={createOrder.isPending}
                  >
                    <Wallet size={15} className="mr-1.5" />
                    {createOrder.isPending ? t("subscriptions.ordering") : t("subscriptions.epayBuy")}
                  </Button>
                </div>
              ))}
            </div>
          )}

          {history.length > 0 && (
            <div className="glass rounded-[22px] p-5 mt-5">
              <h3 className="font-display font-semibold mb-3">{t("subscriptions.history")}</h3>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-[11.5px] font-semibold uppercase tracking-[0.08em] text-[#8C93A1] border-b border-black/5">
                      <th className="py-2 pr-3">{t("subscriptions.thPlan")}</th>
                      <th className="py-2 pr-3">{t("subscriptions.thPrice")}</th>
                      <th className="py-2 pr-3">{t("subscriptions.thStart")}</th>
                      <th className="py-2 pr-3">{t("subscriptions.thEnd")}</th>
                      <th className="py-2">{t("subscriptions.thStatus")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((s) => (
                      <tr key={s.id} className="border-b border-black/[0.04]">
                        <td className="py-2.5 pr-3 font-medium">{s.plan_name}</td>
                        <td className="py-2.5 pr-3">¥{s.price}</td>
                        <td className="py-2.5 pr-3 text-[#5C6472]">{new Date(s.starts_at).toLocaleDateString()}</td>
                        <td className="py-2.5 pr-3 text-[#5C6472]">{new Date(s.expires_at).toLocaleDateString()}</td>
                        <td className="py-2.5">
                          <span className={`text-[12px] font-semibold ${s.status === "active" ? "text-[#0C7A55]" : "text-[#8C93A1]"}`}>
                            {s.status === "active" ? t("subscriptions.statusActive") : s.status}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </>
      )}

      <Dialog open={!!pendingPlan} onOpenChange={(open) => !open && setPendingPlan(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("subscriptions.openTitle", { name: pendingPlan?.name ?? "" })}</DialogTitle>
            <DialogDescription>
              {t("subscriptions.openDesc", {
                price: pendingPlan?.price ?? "",
                duration: pendingPlan ? durationLabel(pendingPlan.duration_days, t) : "",
              })}
            </DialogDescription>
          </DialogHeader>
          <label className="flex items-center gap-2 text-sm text-[#5C6472]">
            <input
              type="checkbox"
              checked={autoRenewConsent}
              onChange={(e) => setAutoRenewConsent(e.target.checked)}
              className="accent-[#4F6BED]"
            />
            {t("subscriptions.autoRenewConsent")}
          </label>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingPlan(null)} disabled={purchase.isPending}>
              {t("subscriptions.cancel")}
            </Button>
            <Button
              onClick={() => pendingPlan && purchase.mutate({ plan_id: pendingPlan.id, auto_renew: autoRenewConsent })}
              disabled={purchase.isPending}
            >
              {purchase.isPending ? t("subscriptions.buying") : t("subscriptions.confirmPay")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <PaymentQrDialog
        open={!!payOrder}
        onClose={() => setPayOrder(null)}
        payURL={payOrder?.payURL ?? ""}
        orderNo={payOrder?.orderNo ?? ""}
        paid={payOrder?.paid ?? false}
      />
    </div>
  );
}
