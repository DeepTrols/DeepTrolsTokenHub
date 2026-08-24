import { useSearchParams } from "react-router-dom";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAuth } from "../lib/auth";
import { ProfileContent } from "./Profile";
import { TeamManagementContent } from "./TeamManagement";

export default function UserCenter() {
  const { user } = useAuth();
  const isEnterpriseAdmin = user?.tenant_role === "owner" || user?.tenant_role === "admin";
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = searchParams.get("tab");
  const activeTab = requested === "team" && isEnterpriseAdmin ? "team" : "profile";

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">用户中心</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">
          整合账户资料与账号体系管理，集中完成账户相关操作。
        </p>
      </div>

      <Tabs
        value={activeTab}
        onValueChange={(v) => setSearchParams(v === "team" ? { tab: "team" } : {})}
      >
        <TabsList>
          <TabsTrigger value="profile">账户资料</TabsTrigger>
          {isEnterpriseAdmin && <TabsTrigger value="team">团队管理</TabsTrigger>}
        </TabsList>
        <TabsContent value="profile">
          <ProfileContent />
        </TabsContent>
        {isEnterpriseAdmin && (
          <TabsContent value="team">
            <TeamManagementContent />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
