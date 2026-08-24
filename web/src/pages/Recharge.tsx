import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AlipayIcon, WechatIcon } from "@/components/PaymentIcons";
import { TopupTable } from "@/components/TopupTable";
import { WalletSummary } from "@/components/WalletSummary";
import { ErrorState, LoadingState } from "@/components/StateViews";
import { useConsoleMutation } from "../lib/hooks/use-api";
import { useWalletData } from "../lib/hooks/use-wallet";
import { formatAmount } from "../lib/format";
import { WalletIcon, Zap } from "lucide-react";

const PRESET_AMOUNTS = ["10", "50", "100", "200", "500"];

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
  const { wallet, topups, isLoading, isError, errorMessage, refetch, txRefetch } = useWalletData();

  const [selectedAmount, setSelectedAmount] = useState("50");
  const [customAmount, setCustomAmount] = useState("");
  const [paymentMethod, setPaymentMethod] = useState("alipay");
  const [paymentError, setPaymentError] = useState("");
  const [paymentSuccess, setPaymentSuccess] = useState("");

  const topupMutation = useConsoleMutation<
    { data: { balance_after: string } },
    { amount: string; payment_method: string }
  >("post", "/wallet/topup", "/wallet", {
    onSuccess: (result) => {
      txRefetch();
      const bal = result?.data?.balance_after ? formatAmount(result.data.balance_after) : "?";
      setPaymentSuccess(`支付成功！当前余额 ${bal} ￥`);
      setPaymentError("");
    },
  });

  const handlePayment = async () => {
    setPaymentError("");
    setPaymentSuccess("");
    const amt = customAmount.trim() || selectedAmount;
    if (!amt || Number(amt) <= 0) {
      setPaymentError("请选择或输入有效的充值金额");
      return;
    }
    try {
      await topupMutation.mutateAsync({ amount: amt, payment_method: paymentMethod });
    } catch (err) {
      setPaymentError(err instanceof Error ? err.message : "支付失败，请稍后重试");
    }
  };

  if (isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">充值</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">余额管理与充值</p>
        </div>
        <LoadingState message="加载钱包数据..." />
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">充值</h2>
        </div>
        <ErrorState error={errorMessage} onRetry={() => refetch()} title="加载钱包数据失败" />
      </div>
    );
  }

  return (
    <div>
      {/* ---- header ---------------------------------------------------------- */}
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">充值</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">余额管理与充值</p>
      </div>

      <WalletSummary wallet={wallet} />

      {/* ---- 在线充值 ------------------------------------------------------- */}
      <div className="glass rounded-[22px] p-5 mb-6">
        <h3 className="font-display font-semibold mb-4">在线支付充值</h3>

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
          <label className="block text-sm font-medium text-[#161A23] mb-2">选择充值金额</label>
          <div className="flex flex-wrap gap-2 mb-3">
            {PRESET_AMOUNTS.map((amt) => (
              <button
                key={amt}
                onClick={() => {
                  setSelectedAmount(amt);
                  setCustomAmount("");
                }}
                className={`px-4 py-2 rounded-xl text-sm font-semibold border transition-all ${
                  selectedAmount === amt && !customAmount
                    ? "bg-white/85 border-[#4F6BED]/60 text-[#4F6BED] shadow-[0_6px_16px_rgba(79,107,237,0.18)]"
                    : "glass-soft text-[#5C6472] hover:text-[#161A23]"
                }`}
              >
                {amt} ￥
              </button>
            ))}
          </div>
          <div className="max-w-[200px]">
            <label className="block text-xs text-[#5C6472] mb-1">或输入自定义金额</label>
            <Input
              type="number"
              min="0"
              step="0.01"
              value={customAmount}
              onChange={(e) => {
                setCustomAmount(e.target.value);
                if (e.target.value) setSelectedAmount("");
              }}
              placeholder="自定义金额"
            />
          </div>
        </div>

        <div className="mb-4">
          <label className="block text-sm font-medium text-[#161A23] mb-2">选择支付方式</label>
          <div className="flex flex-wrap gap-3">
            <label
              className={`flex items-center gap-3 p-3 rounded-xl cursor-pointer transition-colors border ${
                paymentMethod === "alipay"
                  ? "border-[#1677FF]/50 bg-white/80"
                  : "glass-soft border-transparent hover:border-[#1677FF]/30"
              }`}
            >
              <input
                type="radio"
                name="paymentMethod"
                value="alipay"
                checked={paymentMethod === "alipay"}
                onChange={(e) => setPaymentMethod(e.target.value)}
                className="accent-[#1677FF]"
              />
              <PayIcon kind="alipay" />
              <div>
                <p className="text-sm font-medium">支付宝</p>
                <p className="text-xs text-[#5C6472]">推荐使用</p>
              </div>
            </label>
            <label
              className={`flex items-center gap-3 p-3 rounded-xl cursor-pointer transition-colors border ${
                paymentMethod === "wechat"
                  ? "border-[#07C160]/50 bg-white/80"
                  : "glass-soft border-transparent hover:border-[#07C160]/30"
              }`}
            >
              <input
                type="radio"
                name="paymentMethod"
                value="wechat"
                checked={paymentMethod === "wechat"}
                onChange={(e) => setPaymentMethod(e.target.value)}
                className="accent-[#07C160]"
              />
              <PayIcon kind="wechat" />
              <div>
                <p className="text-sm font-medium">微信支付</p>
                <p className="text-xs text-[#5C6472]">安全便捷</p>
              </div>
            </label>
          </div>
        </div>

        <Button onClick={handlePayment} disabled={topupMutation.isPending} className="px-6">
          <WalletIcon size={16} className="mr-1.5" />
          {topupMutation.isPending ? "处理中..." : "充值"}
        </Button>
        <p className="text-xs text-[#5C6472]/80 mt-2">
          点击「充值」后将跳转至支付平台完成付款
        </p>
      </div>

      {/* ---- transaction history --------------------------------------------- */}
      <div className="glass rounded-[22px] p-5">
        <h3 className="font-display font-semibold mb-4">充值记录</h3>
        <TopupTable topups={topups} />
      </div>
    </div>
  );
}
