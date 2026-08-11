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
import { formatAmount } from "../lib/format";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import {
  ArrowUpRight,
  ArrowDownRight,
  RefreshCw,
  WalletIcon,
  Zap,
} from "lucide-react";

const PRESET_AMOUNTS = ["10", "50", "100", "200", "500"];

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
      const bal = result?.data?.balance_after
        ? formatAmount(result.data.balance_after)
        : "?";
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
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div className="bg-white rounded-xl border border-gray-200 p-5">
          <p className="text-sm text-gray-500 mb-1">可用余额</p>
          <p className="text-2xl font-bold text-green-600">
            {formatAmount(wallet?.available)}{" "}
            <span className="text-sm font-normal text-gray-400">
              ￥
            </span>
          </p>
        </div>
        <div className="bg-white rounded-xl border border-gray-200 p-5">
          <p className="text-sm text-gray-500 mb-1">累计消费</p>
          <p className="text-2xl font-bold text-gray-800">
            {/* 累计消费是累计扣费总额，API 可能返回负值，展示时取绝对值 */}
            {formatAmount(wallet?.total_charged?.replace(/^-/, ""))}{" "}
            <span className="text-sm font-normal text-gray-400">
              ￥
            </span>
          </p>
        </div>
      </div>

      {/* ---- 在线充值 ------------------------------------------------------- */}
      <div className="bg-white rounded-xl border border-gray-200 p-5 mb-6">
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
                {txs.map((tx) => (<tr key={tx.id} className="border-b border-gray-50"><td className="py-2.5 font-mono text-xs">{tx.order_no || "—"}</td><td className="py-2.5"><span className={tx.status==="success"?"text-emerald-600":"text-red-600"}>{tx.status==="success"?"成功":tx.status||"—"}</span></td><td className="py-2.5 text-right font-mono text-xs text-emerald-600">+{formatAmount(tx.amount)} ￥</td><td className="py-2.5 text-xs">{tx.payment_method==="alipay"?"支付宝":tx.payment_method==="wechat"?"微信":tx.payment_method||"—"}</td><td className="py-2.5 text-right text-xs text-muted-foreground">{new Date(tx.created_at).toLocaleString("zh-CN")}</td></tr>))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
