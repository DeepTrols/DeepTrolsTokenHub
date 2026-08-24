import { TopupTable } from "@/components/TopupTable";
import { WalletSummary } from "@/components/WalletSummary";
import { ErrorState, LoadingState } from "@/components/StateViews";
import { useWalletData } from "../lib/hooks/use-wallet";

export default function Bills() {
  const { wallet, topups, isLoading, isError, errorMessage, refetch } = useWalletData();

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
        <ErrorState error={errorMessage} onRetry={() => refetch()} title="加载钱包数据失败" />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">账单</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">账户充值记录</p>
      </div>

      <WalletSummary wallet={wallet} />

      <div className="glass rounded-[22px] p-5">
        <h3 className="font-display font-semibold mb-4">充值记录</h3>
        <TopupTable topups={topups} />
      </div>
    </div>
  );
}
