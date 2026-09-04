import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AlipayIcon, WechatIcon } from "@/components/PaymentIcons";
import { WalletSummary } from "@/components/WalletSummary";
import { ErrorState, LoadingState } from "@/components/StateViews";
import { PaymentQrDialog } from "@/components/PaymentQrDialog";
import CheckinCard from "@/components/CheckinCard";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import {
  WalletData,
  PaymentMethodsInfo,
  PaymentOrder,
  CreatePaymentOrderResponse,
} from "../lib/api";
import { WalletIcon, Zap } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

function PayIcon({ kind }: { kind: "alipay" | "wechat" }) {
  const bg = kind === "alipay" ? "#1677FF" : "#07C160";
  return (
    <span
      className="grid w-9 h-9 place-items-center rounded-xl text-white shrink-0"
      style={{ background: bg }}
      aria-hidden="true"
    >
      {kind === "alipay" ? <AlipayIcon size={22} /> : <WechatIcon size={22} />}
    </span>
  );
}

export default function Recharge() {
  const { t } = useTranslation();
  const walletQuery = useConsoleQuery<WalletData>("/wallet");
  const methodsQuery = useConsoleQuery<PaymentMethodsInfo>("/payment/methods");

  const [selectedAmount, setSelectedAmount] = useState("50");
  const [customAmount, setCustomAmount] = useState("");
  const [paymentMethod, setPaymentMethod] = useState("alipay");
  const [paymentError, setPaymentError] = useState("");
  const [paymentSuccess, setPaymentSuccess] = useState("");
  const [order, setOrder] = useState<{ orderNo: string; payURL: string; paid: boolean } | null>(null);

  const createOrder = useConsoleMutation<CreatePaymentOrderResponse, { amount: string; pay_method: string }>(
    "post",
    "/payment/order",
    "/payment/orders",
    {
      onSuccess: (res) => {
        setOrder({ orderNo: res.order_no, payURL: res.pay_url, paid: false });
        setPaymentError("");
      },
      onError: (e) => setPaymentError(e instanceof Error ? e.message : t("recharge.orderFailed")),
    },
  );
  const redeem = useConsoleMutation<{ ok: boolean; amount: string }, { code: string }>(
    "post",
    "/redemption/redeem",
    "/wallet",
    {
      onSuccess: (r) => {
        setPaymentSuccess(t("recharge.redeemSuccess", { amount: r.amount }));
        walletQuery.refetch();
      },
      onError: (e) => setPaymentError(e instanceof Error ? e.message : t("recharge.redeemFailed")),
    },
  );
  const [redeemCode, setRedeemCode] = useState("");

  // Poll orders while a payment dialog is open to auto-detect success.
  const ordersQuery = useConsoleQuery<{ orders: PaymentOrder[] }>("/payment/orders", {
    refetchInterval: order && !order.paid ? 3000 : false,
  });

  useEffect(() => {
    if (!order || order.paid) return;
    const found = ordersQuery.data?.orders?.find((o) => o.order_no === order.orderNo);
    if (found && found.status === "paid") {
      setOrder((prev) => (prev ? { ...prev, paid: true } : prev));
      setPaymentSuccess(t("recharge.paidSuccess"));
      walletQuery.refetch();
    }
  }, [ordersQuery.data, order, walletQuery]);

  const methods = methodsQuery.data;
  const amountOptions = methods?.amount_options?.length ? methods.amount_options : ["10", "50", "100", "200", "500"];
  const payMethods = methods?.pay_methods ?? [];

  const handlePayment = async () => {
    setPaymentError("");
    setPaymentSuccess("");
    const amt = customAmount.trim() || selectedAmount;
    if (!amt || Number(amt) <= 0) {
      setPaymentError(t("recharge.invalidAmount"));
      return;
    }
    const method = payMethods.find((m) => m.type === paymentMethod)?.type ?? (payMethods[0]?.type ?? "alipay");
    await createOrder.mutateAsync({ amount: amt, pay_method: method });
  };

  if (walletQuery.isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("recharge.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("recharge.subtitle")}</p>
        </div>
        <LoadingState message={t("recharge.loading")} />
      </div>
    );
  }

  if (walletQuery.isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("recharge.title")}</h2>
        </div>
        <ErrorState error={walletQuery.error} onRetry={() => walletQuery.refetch()} title={t("recharge.loadFailed")} />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("recharge.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("recharge.subtitle")}</p>
      </div>

      <WalletSummary wallet={walletQuery.data} />

      <div className="glass rounded-[22px] p-5 mb-6">
        <h3 className="font-display font-semibold mb-4">{t("recharge.onlineTitle")}</h3>

        {!methods?.enabled && (
          <div className="mb-4 p-3 glass-soft rounded-xl border-[#E9A23B]/40 text-sm text-[#8a6d1f]">
            {t("recharge.onlineDisabled")}
          </div>
        )}

        {paymentSuccess && (
          <div className="mb-4 p-3 glass-soft rounded-xl border-[#1BA878]/35 text-sm text-[#0C7A55] flex items-center gap-2">
            <Zap size={16} />
            {paymentSuccess}
          </div>
        )}

        {paymentError && (
          <div className="mb-4 p-3 glass-soft rounded-xl border-[#E5484D]/30 text-sm text-[#C4372C]">
            {paymentError}
          </div>
        )}

        <div className="mb-4">
          <label className="block text-sm font-medium text-[#161A23] mb-2">{t("recharge.chooseAmount")}</label>
          <div className="flex flex-wrap gap-2 mb-3">
            {amountOptions.map((amt) => (
              <button
                key={amt}
                onClick={() => {
                  setSelectedAmount(amt);
                  setCustomAmount("");
                }}
                className={`px-4 py-2 rounded-xl text-sm font-semibold border transition-all ${
                  selectedAmount === amt && !customAmount
                    ? "bg-white/85 border-[#F78B28]/60 text-primary-700 shadow-[0_6px_16px_rgba(247,139,40,0.18)]"
                    : "glass-soft text-[#5C6472] hover:text-[#161A23]"
                }`}
              >
                {fmtAmt(amt)} ￥
              </button>
            ))}
          </div>
          <div className="max-w-[200px]">
            <label className="block text-xs text-[#5C6472] mb-1">{t("recharge.customAmount")}</label>
            <Input
              type="number"
              min="0"
              step="0.01"
              value={customAmount}
              onChange={(e) => {
                setCustomAmount(e.target.value);
                if (e.target.value) setSelectedAmount("");
              }}
              placeholder={t("recharge.customPlaceholder")}
            />
          </div>
        </div>

        <div className="mb-4">
          <label className="block text-sm font-medium text-[#161A23] mb-2">{t("recharge.chooseMethod")}</label>
          <div className="flex flex-wrap gap-3">
            {payMethods.length > 0 ? (
              payMethods.map((pm) => (
                <label
                  key={pm.type}
                  className={`flex items-center gap-3 p-3 rounded-xl cursor-pointer transition-colors border ${
                    paymentMethod === pm.type
                      ? "border-[#F78B28]/50 bg-white/80"
                      : "glass-soft border-transparent hover:border-[#F78B28]/30"
                  }`}
                >
                  <input
                    type="radio"
                    name="paymentMethod"
                    value={pm.type}
                    checked={paymentMethod === pm.type}
                    onChange={(e) => setPaymentMethod(e.target.value)}
                    className="accent-[#F78B28]"
                  />
                  <PayIcon kind={pm.type === "wxpay" ? "wechat" : "alipay"} />
                  <div>
                    <p className="text-sm font-medium">{pm.name}</p>
                  </div>
                </label>
              ))
            ) : (
              <p className="text-xs text-[#5C6472]">{t("recharge.noMethods")}</p>
            )}
          </div>
        </div>

        <Button onClick={handlePayment} disabled={createOrder.isPending || !methods?.enabled} className="px-6">
          <WalletIcon size={16} className="mr-1.5" />
          {createOrder.isPending ? t("recharge.processing") : t("recharge.submit")}
        </Button>
        <p className="text-xs text-[#5C6472]/80 mt-2">{t("recharge.hint")}</p>
      </div>

      <PaymentQrDialog
        open={!!order}
        onClose={() => setOrder(null)}
        payURL={order?.payURL ?? ""}
        orderNo={order?.orderNo ?? ""}
        paid={order?.paid ?? false}
      />

      <div className="glass rounded-[22px] p-5 mb-6">
        <h3 className="font-display font-semibold mb-4">{t("recharge.redeemTitle")}</h3>
        <div className="flex gap-3">
          <Input
            value={redeemCode}
            onChange={(e) => setRedeemCode(e.target.value)}
            placeholder={t("recharge.redeemPlaceholder")}
            className="max-w-[280px] font-mono"
          />
          <Button variant="outline" onClick={() => redeem.mutate({ code: redeemCode.trim() })} disabled={redeem.isPending}>
            {redeem.isPending ? t("recharge.redeeming") : t("recharge.redeem")}
          </Button>
        </div>
      </div>

      <CheckinCard />
    </div>
  );
}

function fmtAmt(v: string) {
  const n = Number(v);
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}
