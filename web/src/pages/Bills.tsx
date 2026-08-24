import { TopupTable } from "@/components/TopupTable";
import { ErrorState, LoadingState } from "@/components/StateViews";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { Transaction } from "../lib/api";

export default function Bills() {
  const {
    data: txData,
    isLoading,
    isError,
    error,
    refetch,
  } = useConsoleQuery<{ data: Transaction[] }>("/wallet/transactions");
  const topups = (txData?.data ?? []).filter((t) => t.type === "topup");

  if (isLoading) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">账单</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">账户充值记录</p>
        </div>
        <LoadingState message="加载钱包数据..." />
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">账单</h2>
        </div>
        <ErrorState error={error} onRetry={() => refetch()} title="加载钱包数据失败" />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">账单</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">账户充值记录</p>
      </div>

      <div className="glass rounded-[22px] p-5">
        <h3 className="font-display font-semibold mb-4">充值记录</h3>
        <TopupTable topups={topups} />
      </div>
    </div>
  );
}
