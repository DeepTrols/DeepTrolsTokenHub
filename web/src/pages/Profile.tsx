import { useEffect, useState } from "react";
import { History, CheckCircle, XCircle, User, Building2 } from "lucide-react";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { useAuth } from "../lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface MemberItem { id: string; name: string; email: string; role: string; }
interface EnterpriseInfo { tenant_id: string; tenant_name: string; credit_code: string; members: MemberItem[]; }
interface ProfileData { user: { id: string; email: string; name: string; role: string; status: string; user_type: string; phone: string; avatar_url: string; tenant_id: string; tenant_name: string; tenant_role: string; }; enterprise: EnterpriseInfo | null; }
interface LoginHistoryEntry { ip_address: string; user_agent: string; success: boolean; created_at: string; }

function fmtTime(iso: string): string { try { return new Date(iso).toLocaleString("zh-CN", { year:"numeric", month:"2-digit", day:"2-digit", hour:"2-digit", minute:"2-digit" }); } catch { return iso; } }
function trunc(s: string): string { return s && s.length > 40 ? s.slice(0,40)+"..." : s; }
function roleLabel(r: string): string { if (r === "owner") return "拥有者"; if (r === "admin") return "管理员"; if (r === "member") return "成员"; return r; }

export default function Profile() {
  const { user: authUser } = useAuth();
  const { data: profileData, isLoading: isLoadingProfile } = useConsoleQuery<ProfileData>("/profile");
  const { data: historyData, isLoading: isLoadingHistory } = useConsoleQuery<{ data: LoginHistoryEntry[] }>("/security/login-history");

  const profile = profileData?.user;
  const enterprise = profileData?.enterprise;
  const loginHistory = historyData?.data ?? [];
  const isEnterpriseUser = authUser?.user_type === "enterprise" || !!profile?.tenant_id;

  // Profile update form state — hydrate once data arrives
  const [displayName, setDisplayName] = useState("");
  const [phone, setPhone] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");
  const [didInit, setDidInit] = useState(false);
  useEffect(() => {
    if (profile && !didInit) {
      setDisplayName(profile.name || "");
      setPhone(profile.phone || "");
      setAvatarUrl(profile.avatar_url || "");
      setDidInit(true);
    }
  }, [profile, didInit]);
  const updateProfile = useConsoleMutation("put", "/me/profile", "/profile");

  async function handleSaveProfile() {
    try {
      await updateProfile.mutateAsync({ display_name: displayName, phone, avatar_url: avatarUrl });
    } catch { /* mutation state shows error */ }
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold">个人设置</h2>
        <p className="text-sm text-muted-foreground mt-1">管理您的账户信息和安全设置</p>
      </div>

      <Tabs defaultValue="personal" className="max-w-2xl">
        <TabsList>
          <TabsTrigger value="personal">个人信息</TabsTrigger>
          <TabsTrigger value="security">登录记录</TabsTrigger>
          {isEnterpriseUser && <TabsTrigger value="enterprise">企业信息</TabsTrigger>}
        </TabsList>

        {/* Tab 1: Personal Info */}
        <TabsContent value="personal">
          <Card><CardContent className="p-5">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-primary/10 rounded-lg"><User size={20} className="text-primary" /></div>
              <div><h3 className="font-semibold">基本信息</h3><p className="text-xs text-muted-foreground">修改您的显示名称和联系方式</p></div>
            </div>
            {isLoadingProfile ? (
              <div className="space-y-3 animate-pulse">{[1,2,3].map(i=><div key={i} className="h-10 bg-muted rounded" />)}</div>
            ) : (
              <div className="space-y-4">
                <div className="space-y-1.5"><label className="text-sm font-medium">邮箱</label><Input value={profile?.email ?? ""} disabled className="opacity-60" /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">显示名称</label><Input value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="输入显示名称" /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">手机号</label><Input value={phone} onChange={e => setPhone(e.target.value)} placeholder="输入手机号" /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">头像 URL</label><Input value={avatarUrl} onChange={e => setAvatarUrl(e.target.value)} placeholder="https://..." /></div>
                {updateProfile.isError && <p className="text-sm text-destructive">{updateProfile.error instanceof Error ? updateProfile.error.message : "保存失败"}</p>}
                {updateProfile.isSuccess && <p className="text-sm text-emerald-600">保存成功</p>}
                <Button onClick={handleSaveProfile} disabled={updateProfile.isPending}>{updateProfile.isPending ? "保存中..." : "保存修改"}</Button>
              </div>
            )}
          </CardContent></Card>
        </TabsContent>

        {/* Tab 2: Security */}
        <TabsContent value="security">
          <div className="space-y-4">
            <Card><CardContent className="p-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-muted rounded-lg"><History size={20} /></div>
                <div><h3 className="font-semibold">登录记录</h3><p className="text-xs text-muted-foreground">最近登录活动</p></div>
              </div>
              {isLoadingHistory ? (
                <div className="space-y-3 animate-pulse">{[1,2,3].map(i=><div key={i} className="h-8 bg-muted rounded" />)}</div>
              ) : loginHistory.length===0 ? (
                <p className="text-sm text-muted-foreground text-center py-4">暂无登录记录</p>
              ) : (
                <Table><TableHeader><TableRow><TableHead>IP 地址</TableHead><TableHead>客户端</TableHead><TableHead>状态</TableHead><TableHead>时间</TableHead></TableRow></TableHeader>
                <TableBody>{loginHistory.map((h,i)=><TableRow key={i}><TableCell className="font-mono text-xs">{h.ip_address}</TableCell><TableCell className="text-xs text-muted-foreground max-w-40 truncate">{trunc(h.user_agent)}</TableCell><TableCell><Badge variant={h.success ? "success" : "destructive"}>{h.success ? <><CheckCircle size={12} className="mr-1"/>成功</> : <><XCircle size={12} className="mr-1"/>失败</>}</Badge></TableCell><TableCell className="text-xs text-muted-foreground">{fmtTime(h.created_at)}</TableCell></TableRow>)}</TableBody></Table>
              )}
            </CardContent></Card>
          </div>
        </TabsContent>

        {/* Tab 3: Enterprise Info (conditional) */}
        {isEnterpriseUser && (
          <TabsContent value="enterprise">
            <Card><CardContent className="p-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-primary/10 rounded-lg"><Building2 size={20} className="text-primary" /></div>
                <div><h3 className="font-semibold">企业信息</h3><p className="text-xs text-muted-foreground">您的企业账户详情</p></div>
              </div>
              {enterprise ? (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1"><p className="text-xs text-muted-foreground">企业名称</p><p className="font-medium">{enterprise.tenant_name}</p></div>
                    <div className="space-y-1"><p className="text-xs text-muted-foreground">统一社会信用代码</p><p className="font-mono text-sm">{enterprise.credit_code || "未填写"}</p></div>
                  </div>
                  <div className="space-y-1"><p className="text-xs text-muted-foreground">我的角色</p><Badge variant={profile?.tenant_role === "owner" ? "default" : profile?.tenant_role === "admin" ? "secondary" : "outline"}>{roleLabel(profile?.tenant_role ?? "")}</Badge></div>
                  {enterprise.members.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium">团队成员</p>
                      <Table><TableHeader><TableRow><TableHead>姓名</TableHead><TableHead>邮箱</TableHead><TableHead>角色</TableHead></TableRow></TableHeader>
                        <TableBody>{enterprise.members.map(m => <TableRow key={m.id}><TableCell className="font-medium text-sm">{m.name}</TableCell><TableCell className="text-sm text-muted-foreground">{m.email}</TableCell><TableCell><Badge variant={m.role === "owner" ? "default" : m.role === "admin" ? "secondary" : "outline"}>{roleLabel(m.role)}</Badge></TableCell></TableRow>)}</TableBody></Table>
                    </div>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-4">加载中...</p>
              )}
            </CardContent></Card>
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
