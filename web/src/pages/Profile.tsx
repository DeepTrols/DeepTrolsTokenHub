import { useEffect, useState } from "react";
import { History, CheckCircle, XCircle, User, Building2, Gift, Copy, KeyRound } from "lucide-react";
import { toast } from "sonner";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { useAuth } from "../lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import "../i18n";
import { useTranslation } from "react-i18next";

interface MemberItem { id: string; name: string; email: string; role: string; }
interface EnterpriseInfo { tenant_id: string; tenant_name: string; credit_code: string; members: MemberItem[]; }
interface ProfileData { user: { id: string; email: string; name: string; role: string; status: string; user_type: string; phone: string; avatar_url: string; tenant_id: string; tenant_name: string; tenant_role: string; }; enterprise: EnterpriseInfo | null; }
interface LoginHistoryEntry { ip_address: string; user_agent: string; success: boolean; created_at: string; }
interface SessionRow { id: string; ip_address: string; user_agent: string; created_at: string; expires_at: string; current: boolean; }

function fmtTime(iso: string): string { try { return new Date(iso).toLocaleString("zh-CN", { year:"numeric", month:"2-digit", day:"2-digit", hour:"2-digit", minute:"2-digit" }); } catch { return iso; } }
function trunc(s: string): string { return s && s.length > 40 ? s.slice(0,40)+"..." : s; }
function roleKey(r: string): string { if (r === "owner") return "profile.roleOwner"; if (r === "admin") return "profile.roleAdmin"; if (r === "member") return "profile.roleMember"; return r; }

export function ProfileContent() {
  const { t } = useTranslation();
  const { user: authUser } = useAuth();
  const { data: profileData, isLoading: isLoadingProfile } = useConsoleQuery<ProfileData>("/profile");
  const { data: historyData, isLoading: isLoadingHistory } = useConsoleQuery<{ data: LoginHistoryEntry[] }>("/security/login-history");
  const { data: inviteData } = useConsoleQuery<{ invite_code: string; invited_count: number; invite_link: string }>("/invite");
  const sessionsQuery = useConsoleQuery<{ data: SessionRow[] }>("/sessions");
  const revokeSession = useConsoleMutation<{ ok: boolean }, { id: string }>("delete", (v) => `/sessions/${v.id}`, "/sessions");
  const revokeOthers = useConsoleMutation<{ ok: boolean }, void>("delete", "/sessions", "/sessions", {
    onSuccess: () => toast.success(t("profile.sessionsRevokedOthers")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("profile.sessionsRevokeFailed")),
  });
  const sessions = sessionsQuery.data?.data ?? [];

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
    <Tabs defaultValue="personal" className="max-w-2xl">
      <TabsList>
        <TabsTrigger value="personal">{t("profile.tabPersonal")}</TabsTrigger>
        <TabsTrigger value="security">{t("profile.tabSecurity")}</TabsTrigger>
        <TabsTrigger value="invite">{t("profile.tabInvite")}</TabsTrigger>
        {isEnterpriseUser && <TabsTrigger value="enterprise">{t("profile.tabEnterprise")}</TabsTrigger>}
      </TabsList>

        {/* Tab 1: Personal Info */}
        <TabsContent value="personal">
          <Card><CardContent className="p-5">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-primary/10 rounded-lg"><User size={20} className="text-primary" /></div>
              <div><h3 className="font-semibold">{t("profile.basicTitle")}</h3><p className="text-xs text-muted-foreground">{t("profile.basicDesc")}</p></div>
            </div>
            {isLoadingProfile ? (
              <div className="space-y-3 animate-pulse">{[1,2,3].map(i=><div key={i} className="h-10 bg-muted rounded" />)}</div>
            ) : (
              <div className="space-y-4">
                <div className="space-y-1.5"><label className="text-sm font-medium">{t("profile.email")}</label><Input value={profile?.email ?? ""} disabled className="opacity-60" /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">{t("profile.displayName")}</label><Input value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder={t("profile.namePlaceholder")} /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">{t("profile.phone")}</label><Input value={phone} onChange={e => setPhone(e.target.value)} placeholder={t("profile.phonePlaceholder")} /></div>
                <div className="space-y-1.5"><label className="text-sm font-medium">{t("profile.avatarUrl")}</label><Input value={avatarUrl} onChange={e => setAvatarUrl(e.target.value)} placeholder="https://..." /></div>
                {updateProfile.isError && <p className="text-sm text-destructive">{updateProfile.error instanceof Error ? updateProfile.error.message : t("profile.saveFailed")}</p>}
                {updateProfile.isSuccess && <p className="text-sm text-[#0C7A55]">{t("profile.saveSuccess")}</p>}
                <Button onClick={handleSaveProfile} disabled={updateProfile.isPending}>{updateProfile.isPending ? t("profile.saving") : t("profile.save")}</Button>
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
                <div><h3 className="font-semibold">{t("profile.historyTitle")}</h3><p className="text-xs text-muted-foreground">{t("profile.historyDesc")}</p></div>
              </div>
              {isLoadingHistory ? (
                <div className="space-y-3 animate-pulse">{[1,2,3].map(i=><div key={i} className="h-8 bg-muted rounded" />)}</div>
              ) : loginHistory.length===0 ? (
                <p className="text-sm text-muted-foreground text-center py-4">{t("profile.noHistory")}</p>
              ) : (
                <Table><TableHeader><TableRow><TableHead>{t("profile.thIp")}</TableHead><TableHead>{t("profile.thClient")}</TableHead><TableHead>{t("profile.thStatus")}</TableHead><TableHead>{t("profile.thTime")}</TableHead></TableRow></TableHeader>
                <TableBody>{loginHistory.map((h,i)=><TableRow key={i}><TableCell className="font-mono text-xs">{h.ip_address}</TableCell><TableCell className="text-xs text-muted-foreground max-w-40 truncate">{trunc(h.user_agent)}</TableCell><TableCell><Badge variant={h.success ? "success" : "destructive"}>{h.success ? <><CheckCircle size={12} className="mr-1"/>{t("profile.success")}</> : <><XCircle size={12} className="mr-1"/>{t("profile.failed")}</>}</Badge></TableCell><TableCell className="text-xs text-muted-foreground">{fmtTime(h.created_at)}</TableCell></TableRow>)}</TableBody></Table>
              )}
            </CardContent></Card>
            <Card><CardContent className="p-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-muted rounded-lg"><KeyRound size={20} /></div>
                <div>
                  <h3 className="font-semibold">{t("profile.sessionsTitle")}</h3>
                  <p className="text-xs text-muted-foreground">{t("profile.sessionsDesc")}</p>
                </div>
                {sessions.length > 1 && (
                  <Button variant="outline" size="sm" className="ml-auto" onClick={() => revokeOthers.mutate()} disabled={revokeOthers.isPending}>
                    {t("profile.sessionsRevokeOthers")}
                  </Button>
                )}
              </div>
              {sessions.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-4">{t("profile.sessionsEmpty")}</p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("profile.sessionsDevice")}</TableHead>
                      <TableHead>{t("profile.sessionsIp")}</TableHead>
                      <TableHead>{t("profile.sessionsTime")}</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {sessions.map((s) => (
                      <TableRow key={s.id}>
                        <TableCell className="text-xs max-w-48 truncate">{s.user_agent || "—"}</TableCell>
                        <TableCell className="font-mono text-xs">{s.ip_address || "—"}</TableCell>
                        <TableCell className="text-xs text-muted-foreground">{fmtTime(s.created_at)}</TableCell>
                        <TableCell className="text-right">
                          {s.current ? (
                            <Badge variant="success">{t("profile.sessionsCurrent")}</Badge>
                          ) : (
                            <Button variant="ghost" size="sm" onClick={() => revokeSession.mutate({ id: s.id })} disabled={revokeSession.isPending}>
                              {t("profile.sessionsRevoke")}
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent></Card>
          </div>
        </TabsContent>

        {/* Invite tab */}
        <TabsContent value="invite">
          <Card><CardContent className="p-5">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-primary/10 rounded-lg"><Gift size={20} className="text-primary" /></div>
              <div>
                <h3 className="font-semibold">{t("profile.inviteTitle")}</h3>
                <p className="text-xs text-muted-foreground">{t("profile.inviteDesc")}</p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <code className="flex-1 px-3 py-2 glass-soft rounded-xl font-mono text-sm">{inviteData?.invite_code ?? "…"}</code>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (inviteData?.invite_code) {
                    navigator.clipboard?.writeText(inviteData.invite_code).catch(() => undefined);
                    toast.success(t("profile.inviteCopied"));
                  }
                }}
              >
                <Copy size={13} className="mr-1" /> {t("profile.copy")}
              </Button>
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              {t("profile.invitedCount", { count: inviteData?.invited_count ?? 0 })} {" "}
              <code className="font-mono">{inviteData?.invite_link ?? ""}</code>
            </p>
          </CardContent></Card>
        </TabsContent>

        {/* Tab 3: Enterprise Info (conditional) */}
        {isEnterpriseUser && (
          <TabsContent value="enterprise">
            <Card><CardContent className="p-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-primary/10 rounded-lg"><Building2 size={20} className="text-primary" /></div>
                <div><h3 className="font-semibold">{t("profile.entTitle")}</h3><p className="text-xs text-muted-foreground">{t("profile.entDesc")}</p></div>
              </div>
              {enterprise ? (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1"><p className="text-xs text-muted-foreground">{t("profile.entName")}</p><p className="font-medium">{enterprise.tenant_name}</p></div>
                    <div className="space-y-1"><p className="text-xs text-muted-foreground">{t("profile.entCredit")}</p><p className="font-mono text-sm">{enterprise.credit_code || t("profile.notFilled")}</p></div>
                  </div>
                  <div className="space-y-1"><p className="text-xs text-muted-foreground">{t("profile.myRole")}</p><Badge variant={profile?.tenant_role === "owner" ? "default" : profile?.tenant_role === "admin" ? "secondary" : "outline"}>{t(roleKey(profile?.tenant_role ?? ""))}</Badge></div>
                  {enterprise.members.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium">{t("profile.teamMembers")}</p>
                      <Table><TableHeader><TableRow><TableHead>{t("profile.thName")}</TableHead><TableHead>{t("profile.thEmail")}</TableHead><TableHead>{t("profile.thRole")}</TableHead></TableRow></TableHeader>
                        <TableBody>{enterprise.members.map(m => <TableRow key={m.id}><TableCell className="font-medium text-sm">{m.name}</TableCell><TableCell className="text-sm text-muted-foreground">{m.email}</TableCell><TableCell><Badge variant={m.role === "owner" ? "default" : m.role === "admin" ? "secondary" : "outline"}>{t(roleKey(m.role))}</Badge></TableCell></TableRow>)}</TableBody></Table>
                    </div>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-4">{t("profile.loading")}</p>
              )}
            </CardContent></Card>
          </TabsContent>
        )}
    </Tabs>
  );
}
