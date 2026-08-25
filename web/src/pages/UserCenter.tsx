import { ProfileContent } from "./Profile";

export default function UserCenter() {
  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">用户中心</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">
          整合账户资料与账号体系管理，集中完成账户相关操作。
        </p>
      </div>

      <ProfileContent />
    </div>
  );
}
