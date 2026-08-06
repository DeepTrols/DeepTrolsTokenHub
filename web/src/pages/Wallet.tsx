import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { useState } from "react";
import { WalletData, Transaction } from "../lib/api";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import {
  ArrowUpRight,
  ArrowDownRight,
  RefreshCw,
  WalletIcon,
  Gift,
  Users,
  Copy,
  Check,
  CreditCard,
  Zap,
} from "lucide-react";

type TabKey = "payment" | "redeem" | "invite";

const PRESET_AMOUNTS = ["10", "50", "100", "200", "500"];

interface InviteData {
  invite_code: string;
  total_rewards: string;
  referral_count: number;
}

export default function Wallet() {
  // ---- wallet data -----------------------------------------------------------
  const {
    data: wallet,
    isLoading: walletLoading,
    isError: walletIsError,
    error: walletError,
    refetch: walletRefetch,
  } = useConsoleQuery<WalletData>("/wallet");
  const {
    data: txData,
    isLoading: txLoading,
    isError: txIsError,
    error: txError,
    refetch: txRefetch,
  } = useConsoleQuery<{ data: Transaction[] }>("/wallet/transactions");
  const txs = (txData?.data ?? []).filter((t: any) => t.type === "topup");

  const isLoading = walletLoading || txLoading;
  const isError = walletIsError || txIsError;
  const errorMessage = walletError || txError;
  const refetch = () => {
    walletRefetch();
    txRefetch();
  };

  // ---- invite data -----------------------------------------------------------
  const { data: inviteData, refetch: inviteRefetch } =
    useConsoleQuery<InviteData>("/invite");

  // ---- active tab ------------------------------------------------------------
  const [activeTab, setActiveTab] = useState<TabKey>("payment");

  // ---- payment form ----------------------------------------------------------
  const [selectedAmount, setSelectedAmount] = useState("50");
  const [customAmount, setCustomAmount] = useState("");
  const [paymentMethod, setPaymentMethod] = useState("epay");
  const [paymentError, setPaymentError] = useState("");
  const [paymentSuccess, setPaymentSuccess] = useState("");

  const topupMutation = useConsoleMutation<
    { data: { balance_after: string } },
    { amount: string }
  >("post", "/wallet/topup", "/wallet", {
    onSuccess: (result) => {
      txRefetch();
      inviteRefetch();
      const bal = result?.data?.balance_after ?? "?";
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
      await topupMutation.mutateAsync({ amount: amt });
    } catch (err) {
      setPaymentError(err instanceof Error ? err.message : "支付失败，请稍后重试");
    }
  };

  // ---- redeem form -----------------------------------------------------------
  const [redeemCode, setRedeemCode] = useState("");
  const [redeemError, setRedeemError] = useState("");
  const [redeemSuccess, setRedeemSuccess] = useState("");

  const redeemMutation = useConsoleMutation<
    { data: { amount: string; balance_after: string; message: string } },
    { code: string }
  >("post", "/wallet/redeem", "/wallet", {
    onSuccess: (result) => {
      txRefetch();
      inviteRefetch();
      const msg = result?.data?.message ?? "兑换成功";
      setRedeemSuccess(msg);
      setRedeemError("");
      setRedeemCode("");
    },
  });

  const handleRedeem = async () => {
    setRedeemError("");
    setRedeemSuccess("");
    if (!redeemCode.trim()) {
      setRedeemError("请输入兑换码");
      return;
    }
    try {
      await redeemMutation.mutateAsync({ code: redeemCode.trim() });
    } catch (err) {
      setRedeemError(err instanceof Error ? err.message : "兑换失败");
    }
  };

  // ---- invite section --------------------------------------------------------
  const [copied, setCopied] = useState(false);

  const transferMutation = useConsoleMutation<
    { data: { amount: string; balance_after: string; message: string } },
    Record<string, never>
  >("post", "/invite/transfer", "/wallet", {
    onSuccess: (result) => {
      txRefetch();
      inviteRefetch();
      setPaymentSuccess(result?.data?.message ?? "转入成功");
    },
  });

  const handleCopyInvite = async () => {
    if (!inviteData?.invite_code) return;
    await navigator.clipboard.writeText(inviteData.invite_code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleTransferRewards = async () => {
    try {
      await transferMutation.mutateAsync({});
    } catch {
      // error shown by mutation
    }
  };

  // ---- shared ----------------------------------------------------------------
  const txIcon = (type: string) => {
    switch (type) {
      case "topup":
        return <ArrowUpRight size={14} className="text-green-600" />;
      case "charge":
        return <ArrowDownRight size={14} className="text-red-600" />;
      default:
        return <RefreshCw size={14} className="text-gray-400" />;
    }
  };

  const txLabel = (type: string) => {
    switch (type) {
      case "topup":
        return "充值";
      case "charge":
        return "扣费";
      case "refund":
        return "退款";
      case "reserve":
        return "冻结";
      case "release":
        return "解冻";
      default:
        return type;
    }
  };

  // ---- loading / error -------------------------------------------------------
  if (isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="text-2xl font-bold">钱包管理</h2>
          <p className="text-sm text-gray-500 mt-1">
            余额管理与充值
          </p>
        </div>
        <div className="p-12 text-center bg-white rounded-xl border">
          <div className="animate-spin w-8 h-8 border-2 border-primary-600 border-t-transparent rounded-full mx-auto mb-3" />
          <p className="text-gray-500">加载钱包数据...</p>
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="text-2xl font-bold">钱包管理</h2>
        </div>
        <div className="bg-red-50 border border-red-200 rounded-xl p-6 text-center">
          <p className="text-red-600 mb-3">
            {errorMessage instanceof Error
              ? errorMessage.message
              : "加载钱包数据失败"}
          </p>
          <button
            onClick={() => refetch()}
            className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 text-sm font-medium"
          >
            重试
          </button>
        </div>
      </div>
    );
  }

  // ---- tabs ------------------------------------------------------------------
  const tabs: { key: TabKey; icon: React.ReactNode; label: string }[] = [
    { key: "payment", icon: <CreditCard size={16} />, label: "在线充值" },
    { key: "redeem", icon: <Gift size={16} />, label: "兑换码" },
    { key: "invite", icon: <Users size={16} />, label: "邀请返利" },
  ];

  return (
    <div>
      {/* ---- header ---------------------------------------------------------- */}
      <div className="mb-6">
        <h2 className="text-2xl font-bold">钱包管理</h2>
        <p className="text-sm text-gray-500 mt-1">
          余额管理与充值
        </p>
      </div>

      {/* ---- balance cards --------------------------------------------------- */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="bg-white rounded-xl border border-gray-200 p-5">
          <p className="text-sm text-gray-500 mb-1">可用余额</p>
          <p className="text-2xl font-bold text-green-600">
            {wallet?.available || "0.00"}{" "}
            <span className="text-sm font-normal text-gray-400">
              ￥
            </span>
          </p>
        </div>
        <div className="bg-white rounded-xl border border-gray-200 p-5">
          <p className="text-sm text-gray-500 mb-1">冻结金额</p>
          <p className="text-2xl font-bold text-orange-500">
            {wallet?.frozen || "0.00"}{" "}
            <span className="text-sm font-normal text-gray-400">
              ￥
            </span>
          </p>
        </div>
        <div className="bg-white rounded-xl border border-gray-200 p-5">
          <p className="text-sm text-gray-500 mb-1">累计消费</p>
          <p className="text-2xl font-bold text-gray-800">
            {wallet?.total_charged || "0.00"}{" "}
            <span className="text-sm font-normal text-gray-400">
              ￥
            </span>
          </p>
        </div>
      </div>

      {/* ---- tabs ------------------------------------------------------------ */}
      <div className="bg-white rounded-xl border border-gray-200 mb-6">
        {/* tab bar */}
        <div className="flex border-b border-gray-100">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`flex items-center gap-2 px-5 py-3 text-sm font-medium transition-colors ${
                activeTab === tab.key
                  ? "text-primary-600 border-b-2 border-primary-600 bg-primary-50/50"
                  : "text-gray-500 hover:text-gray-700"
              }`}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>

        {/* tab content */}
        <div className="p-5">
          {/* ---- 在线充值 ---------------------------------------------------- */}
          {activeTab === "payment" && (
            <div>
              <h3 className="font-semibold mb-4">在线支付充值</h3>

              {/* success banner */}
              {paymentSuccess && (
                <div className="mb-4 p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-700 flex items-center gap-2">
                  <Zap size={16} />
                  {paymentSuccess}
                </div>
              )}

              {/* error banner */}
              {paymentError && (
                <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">
                  {paymentError}
                </div>
              )}

              {/* amount selection */}
              <div className="mb-4">
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  选择充值金额
                </label>
                <div className="flex flex-wrap gap-2 mb-3">
                  {PRESET_AMOUNTS.map((amt) => (
                    <button
                      key={amt}
                      onClick={() => {
                        setSelectedAmount(amt);
                        setCustomAmount("");
                      }}
                      className={`px-4 py-2 rounded-lg text-sm font-medium border transition-colors ${
                        selectedAmount === amt && !customAmount
                          ? "border-primary-600 bg-primary-50 text-primary-700"
                          : "border-gray-200 hover:border-gray-300 text-gray-600"
                      }`}
                    >
                      {amt} ￥
                    </button>
                  ))}
                </div>
                <div className="max-w-[200px]">
                  <label className="block text-xs text-gray-500 mb-1">
                    或输入自定义金额
                  </label>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={customAmount}
                    onChange={(e) => {
                      setCustomAmount(e.target.value);
                      if (e.target.value) setSelectedAmount("");
                    }}
                    placeholder="自定义金额"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
                  />
                </div>
              </div>

              {/* payment method */}
              <div className="mb-4">
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  选择支付方式
                </label>
                <label className="flex items-center gap-3 p-3 border border-gray-200 rounded-lg cursor-pointer hover:border-primary-300 transition-colors max-w-sm">
                  <input
                    type="radio"
                    name="paymentMethod"
                    value="epay"
                    checked={paymentMethod === "epay"}
                    onChange={(e) => setPaymentMethod(e.target.value)}
                    className="text-primary-600"
                  />
                  <div>
                    <p className="text-sm font-medium">EPay 聚合支付</p>
                    <p className="text-xs text-gray-500">
                      支持支付宝、微信支付等
                    </p>
                  </div>
                </label>
              </div>

              {/* pay button */}
              <button
                onClick={handlePayment}
                disabled={topupMutation.isPending}
                className="flex items-center gap-2 px-6 py-2.5 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm font-medium disabled:opacity-50"
              >
                <WalletIcon size={16} />
                {topupMutation.isPending ? "处理中..." : "充值"}
              </button>
              <p className="text-xs text-gray-400 mt-2">
                点击「充值」后将跳转至支付平台完成付款
              </p>
            </div>
          )}

          {/* ---- 兑换码 ------------------------------------------------------ */}
          {activeTab === "redeem" && (
            <div>
              <h3 className="font-semibold mb-4">兑换码充值</h3>

              {redeemSuccess && (
                <div className="mb-4 p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-700 flex items-center gap-2">
                  <Zap size={16} />
                  {redeemSuccess}
                </div>
              )}

              {redeemError && (
                <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">
                  {redeemError}
                </div>
              )}

              <div className="max-w-md">
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  输入兑换码
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={redeemCode}
                    onChange={(e) => setRedeemCode(e.target.value)}
                    placeholder="粘贴或输入管理员提供的兑换码"
                    className="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500 font-mono"
                  />
                  <button
                    onClick={handleRedeem}
                    disabled={redeemMutation.isPending}
                    className="px-5 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm font-medium disabled:opacity-50"
                  >
                    {redeemMutation.isPending ? "兑换中..." : "兑换"}
                  </button>
                </div>
                <p className="text-xs text-gray-400 mt-2">
                  输入管理员生成的兑换码，直接增加配额
                </p>
              </div>
            </div>
          )}

          {/* ---- 邀请返利 ---------------------------------------------------- */}
          {activeTab === "invite" && (
            <div>
              <h3 className="font-semibold mb-4">邀请返利</h3>

              {/* invite code */}
              <div className="mb-5 max-w-md">
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  你的专属邀请码
                </label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 px-4 py-2.5 bg-gray-50 border border-gray-200 rounded-lg text-lg font-mono font-bold tracking-wider text-primary-700 select-all">
                    {inviteData?.invite_code || "加载中..."}
                  </code>
                  <button
                    onClick={handleCopyInvite}
                    className="flex items-center gap-1 px-3 py-2.5 border border-gray-200 rounded-lg hover:bg-gray-50 text-sm text-gray-600 transition-colors"
                  >
                    {copied ? (
                      <>
                        <Check size={14} className="text-green-600" />
                        <span className="text-green-600">已复制</span>
                      </>
                    ) : (
                      <>
                        <Copy size={14} />
                        <span>复制</span>
                      </>
                    )}
                  </button>
                </div>
                <p className="text-xs text-gray-400 mt-2">
                  分享给你的朋友，他们注册后你将获得返利奖励
                </p>
              </div>

              {/* rewards summary */}
              <div className="bg-amber-50 border border-amber-200 rounded-xl p-5 max-w-md mb-4">
                <div className="flex items-center justify-between mb-3">
                  <p className="text-sm font-medium text-amber-800">
                    累计返利配额
                  </p>
                  <p className="text-xl font-bold text-amber-700">
                    {inviteData?.total_rewards || "0.00"} ￥
                  </p>
                </div>
                <div className="flex items-center justify-between text-xs text-amber-600">
                  <span>
                    成功邀请 {inviteData?.referral_count || 0} 人
                  </span>
                </div>
                <button
                  onClick={handleTransferRewards}
                  disabled={
                    transferMutation.isPending ||
                    !inviteData?.total_rewards ||
                    inviteData.total_rewards === "0" ||
                    inviteData.total_rewards === "0.000"
                  }
                  className="mt-4 w-full flex items-center justify-center gap-2 px-4 py-2 bg-amber-600 text-white rounded-lg hover:bg-amber-700 text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <ArrowUpRight size={14} />
                  {transferMutation.isPending ? "转入中..." : "转入余额"}
                </button>
                {transferMutation.isError && (
                  <p className="mt-2 text-xs text-red-600">
                    {transferMutation.error instanceof Error
                      ? transferMutation.error.message
                      : "转入失败"}
                  </p>
                )}
              </div>

              <div className="text-xs text-gray-400 max-w-md space-y-1">
                <p>
                  <strong>注册即返利：</strong>
                  被邀请人使用你的邀请码注册成功后，你会立即获得系统配置的「邀请奖励配额」。
                </p>
                <p>
                  <strong>转入主余额：</strong>
                  点击「转入余额」按钮，即可将累计的返利配额转入你的账户主余额中进行使用。
                </p>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ---- transaction history --------------------------------------------- */}
      <div className="bg-white rounded-xl border border-gray-200 p-5">
        <h3 className="font-semibold mb-4">充值记录</h3>
        {txs.length === 0 && (
          <p className="py-8 text-center text-gray-400 text-sm">
            暂无充值记录
          </p>
        )}
        {txs.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100">
                  <th className="text-left py-2 text-gray-500 font-medium">订单编号</th><th className="text-left py-2 text-gray-500 font-medium">状态</th><th className="text-right py-2 text-gray-500 font-medium">金额</th><th className="text-left py-2 text-gray-500 font-medium">支付方式</th><th className="text-right py-2 text-gray-500 font-medium">时间
                  </th>
                </tr>
              </thead>
              <tbody>
                {txs.map((tx) => (<tr key={tx.id} className="border-b border-gray-50"><td className="py-2.5 font-mono text-xs">{tx.order_no || "—"}</td><td className="py-2.5"><span className={tx.status==="success"?"text-emerald-600":"text-red-600"}>{tx.status==="success"?"成功":tx.status||"—"}</span></td><td className="py-2.5 text-right font-mono text-xs text-emerald-600">+{tx.amount} ￥</td><td className="py-2.5 text-xs">{tx.payment_method==="alipay"?"支付宝":tx.payment_method==="wechat"?"微信":tx.payment_method||"—"}</td><td className="py-2.5 text-right text-xs text-muted-foreground">{new Date(tx.created_at).toLocaleString("zh-CN")}</td></tr>))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
