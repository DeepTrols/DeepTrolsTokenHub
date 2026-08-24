import { WalletData } from "../lib/api";
import { formatAmount } from "../lib/format";

/** Two balance cards shared by the 充值 / 账单 pages. */
export function WalletSummary({ wallet }: { wallet?: WalletData }) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
      <div className="glass rounded-[22px] p-5 relative overflow-hidden">
        <div className="absolute w-[110px] h-[110px] rounded-full blur-[28px] opacity-50 -top-[38px] -right-[28px] bg-[#1BA878]/40 pointer-events-none" />
        <p className="text-[12px] font-semibold text-[#5C6472] mb-1 relative">可用余额</p>
        <p className="font-mono text-[26px] font-semibold tracking-tight text-[#0C7A55] relative">
          {formatAmount(wallet?.available)}{" "}
          <span className="text-sm font-normal text-[#5C6472]">￥</span>
        </p>
      </div>
      <div className="glass rounded-[22px] p-5 relative overflow-hidden">
        <div className="absolute w-[110px] h-[110px] rounded-full blur-[28px] opacity-50 -top-[38px] -right-[28px] bg-[#4F6BED]/35 pointer-events-none" />
        <p className="text-[12px] font-semibold text-[#5C6472] mb-1 relative">累计消费</p>
        <p className="font-mono text-[26px] font-semibold tracking-tight text-[#161A23] relative">
          {/* 累计消费是累计扣费总额，API 可能返回负值，展示时取绝对值 */}
          {formatAmount(wallet?.total_charged?.replace(/^-/, ""))}{" "}
          <span className="text-sm font-normal text-[#5C6472]">￥</span>
        </p>
      </div>
    </div>
  );
}
