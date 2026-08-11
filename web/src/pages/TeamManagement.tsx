import { useState } from "react";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { useAuth } from "../lib/auth";
import { Search, Trash2, Edit, Users, X, UserPlus, UserCog, Ban, CheckCircle2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { EmptyState, LoadingState } from "@/components/StateViews";

interface MemberRow { id: string; name: string; email: string; role: string; status: string; }
interface Invitation { id: string; email: string; role: string; status: string; created_at: string; expires_at: string; }

const STATUS_META: Record<string, { label: string; variant: "success" | "destructive" | "secondary" }> = {
  active: { label: "正常", variant: "success" },
  suspended: { label: "已停用", variant: "destructive" },
  left: { label: "已离开", variant: "secondary" },
};

function roleBadge(role: string): "default" | "secondary" | "outline" {
  if (role === "owner") return "default";
  if (role === "admin") return "secondary";
  return "outline";
}
function roleLabel(role: string): string {
  if (role === "owner") return "拥有者";
  if (role === "admin") return "管理员";
  if (role === "member") return "成员";
  return role;
}
function fmtDate(s: string): string {
  if (!s) return "—";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

export default function TeamManagement() {
  const { user } = useAuth();
  const isOwner = user?.tenant_role === "owner";

  const { data, isLoading } = useConsoleQuery<{ members: MemberRow[] }>("/team");
  const members: MemberRow[] = data?.members ?? [];

  const { data: invData } = useConsoleQuery<{ invitations: Invitation[]; total: number }>("/team/invitations");
  const invitations: Invitation[] = invData?.invitations ?? [];

  // Invite dialog
  const [showInvite, setShowInvite] = useState(false);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("member");
  const inviteMutation = useConsoleMutation<unknown, { email: string; role: string }>("post", "/team/invite", "/team/invitations");

  async function handleInvite() {
    if (!inviteEmail.trim()) return;
    try {
      await inviteMutation.mutateAsync({ email: inviteEmail.trim(), role: inviteRole });
      setShowInvite(false);
      setInviteEmail("");
      setInviteRole("member");
    } catch { /* mutation state shows error */ }
  }

  // Remove member
  const removeMutation = useConsoleMutation<unknown, { id: string }>("delete", v => `/team/${v.id}`, "/team");
  async function handleRemove(m: MemberRow) {
    if (!confirm(`确定移除成员 ${m.email} 吗？`)) return;
    try { await removeMutation.mutateAsync({ id: m.id }); } catch { /* handled */ }
  }

  // Change role dialog (owner only)
  const [editingMember, setEditingMember] = useState<MemberRow | null>(null);
  const [newRole, setNewRole] = useState("");
  const roleMutation = useConsoleMutation<unknown, { id: string; role: string }>("put", v => `/team/${v.id}/role`, "/team");
  function openRoleEdit(m: MemberRow) { roleMutation.reset(); setEditingMember(m); setNewRole(m.role); }
  async function handleRoleSave() {
    if (!editingMember) return;
    try { await roleMutation.mutateAsync({ id: editingMember.id, role: newRole }); setEditingMember(null); } catch { /* handled */ }
  }

  // Toggle member status (suspend / activate). Backend forbids: self changes,
  // suspending the owner, and non-owners suspending admins.
  const statusMutation = useConsoleMutation<unknown, { id: string; status: string }>("put", v => `/team/${v.id}/status`, "/team");
  async function handleToggleStatus(m: MemberRow) {
    const next = m.status === "suspended" ? "active" : "suspended";
    const verb = next === "suspended" ? "停用" : "启用";
    if (!confirm(`确定${verb}成员 ${m.email} 吗？`)) return;
    try { await statusMutation.mutateAsync({ id: m.id, status: next }); } catch { /* handled */ }
  }
  function canToggleStatus(m: MemberRow): boolean {
    if (m.id === user?.id) return false;
    if (m.role === "owner") return false;
    if (m.role === "admin" && !isOwner) return false;
    return true;
  }

  // Cancel pending invitation
  const cancelInvMutation = useConsoleMutation<unknown, { id: string }>("delete", v => `/team/invitations/${v.id}`, "/team/invitations");
  async function handleCancelInv(inv: Invitation) {
    if (!confirm(`取消对 ${inv.email} 的邀请吗？`)) return;
    try { await cancelInvMutation.mutateAsync({ id: inv.id }); } catch { /* handled */ }
  }

  // Latest error from any row action (remove / status toggle / cancel invite),
  // surfaced in a banner so a failure is never silently swallowed.
  const rowActionError = removeMutation.error ?? statusMutation.error ?? cancelInvMutation.error;

  // Transfer ownership (owner only). Reload so the demoted owner's role takes
  // effect app-wide. TODO(#auth): replace reload with an auth-context refresh
  // once AuthContext exposes a refresh() that re-runs fetchMe.
  const [showTransfer, setShowTransfer] = useState(false);
  const transferMutation = useConsoleMutation<unknown, { target_user_id: string }>("post", "/team/transfer-ownership", "/team");
  async function handleTransfer(m: MemberRow) {
    if (!confirm(`确认将企业所有权转让给 ${m.name || m.email}？转让后您将成为普通管理员，此操作不可撤销。`)) return;
    try {
      await transferMutation.mutateAsync({ target_user_id: m.id });
      setShowTransfer(false);
      window.location.reload();
    } catch { /* handled */ }
  }
  const adminCandidates = members.filter(m => m.role === "admin" && m.id !== user?.id && m.status === "active");

  // Search filter
  const [query, setQuery] = useState("");
  const filtered = query.trim()
    ? members.filter(m => m.name.toLowerCase().includes(query.toLowerCase()) || m.email.toLowerCase().includes(query.toLowerCase()))
    : members;

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>团队管理</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header>
        <SectionPageLayout.Content><LoadingState message="加载团队成员..." /></SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>团队管理</SectionPageLayout.Title>
          <SectionPageLayout.Description>共 {members.length} 人{invitations.length > 0 ? ` · ${invitations.length} 封待处理邀请` : ""}</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
        <SectionPageLayout.Actions>
          {isOwner && (
            <Button variant="outline" onClick={() => setShowTransfer(true)}>
              <UserCog size={16} className="mr-1.5" />转让所有权
            </Button>
          )}
          <Button onClick={() => { setInviteEmail(""); setInviteRole("member"); inviteMutation.reset(); setShowInvite(true); }}>
            <UserPlus size={16} className="mr-1.5" />邀请成员
          </Button>
        </SectionPageLayout.Actions>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        {rowActionError && (
          <Card className="mb-4 border-destructive/30">
            <CardContent className="p-3 text-sm text-destructive">
              {rowActionError instanceof Error ? rowActionError.message : String(rowActionError)}
            </CardContent>
          </Card>
        )}
        <div className="mb-4 relative max-w-sm">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索姓名或邮箱..." className="pl-9 h-9 text-sm" />
          {query && <Button variant="ghost" size="icon" className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7" onClick={() => setQuery("")}><X size={14} /></Button>}
        </div>

        <Card className="overflow-hidden">
          <Table>
            <TableHeader><TableRow><TableHead>姓名</TableHead><TableHead>邮箱</TableHead><TableHead>角色</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {filtered.length === 0 && <TableRow><TableCell colSpan={5}><EmptyState icon={Users} title={query ? "未找到匹配成员" : "暂无团队成员"} /></TableCell></TableRow>}
              {filtered.map(m => (
                <TableRow key={m.id}>
                  <TableCell className="font-medium text-sm">{m.name}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{m.email}</TableCell>
                  <TableCell><Badge variant={roleBadge(m.role)}>{roleLabel(m.role)}</Badge></TableCell>
                  <TableCell><Badge variant={STATUS_META[m.status]?.variant ?? "secondary"}>{STATUS_META[m.status]?.label ?? m.status}</Badge></TableCell>
                  <TableCell className="text-right">
                    <div className="flex gap-1 justify-end">
                      {m.role === "owner" ? (
                        <span className="text-xs text-muted-foreground self-center">所有者不可操作</span>
                      ) : (
                        <>
                          {canToggleStatus(m) && (
                            <Button variant="outline" size="sm" onClick={() => handleToggleStatus(m)}>
                              {m.status === "suspended" ? <><CheckCircle2 size={12} className="mr-1" />启用</> : <><Ban size={12} className="mr-1" />停用</>}
                            </Button>
                          )}
                          {isOwner && <Button variant="outline" size="sm" onClick={() => openRoleEdit(m)}><Edit size={12} className="mr-1" />改角色</Button>}
                          {m.id !== user?.id && (
                            <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => handleRemove(m)}><Trash2 size={12} /></Button>
                          )}
                        </>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>

        {/* Pending invitations */}
        <Card className="mt-6 overflow-hidden">
          <div className="px-4 py-3 border-b">
            <h3 className="font-medium text-sm">待处理邀请</h3>
            <p className="text-xs text-muted-foreground mt-0.5">受邀用户注册后自动加入企业</p>
          </div>
          {invitations.length === 0 ? (
            <div className="p-6 text-center text-sm text-muted-foreground">暂无待处理邀请</div>
          ) : (
            <Table>
              <TableHeader><TableRow><TableHead>邮箱</TableHead><TableHead>角色</TableHead><TableHead>邀请时间</TableHead><TableHead>有效期至</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {invitations.map(inv => (
                  <TableRow key={inv.id}>
                    <TableCell className="text-sm">{inv.email}</TableCell>
                    <TableCell><Badge variant={roleBadge(inv.role)}>{roleLabel(inv.role)}</Badge></TableCell>
                    <TableCell className="text-sm text-muted-foreground">{fmtDate(inv.created_at)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{fmtDate(inv.expires_at)}</TableCell>
                    <TableCell className="text-right">
                      <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => handleCancelInv(inv)} disabled={cancelInvMutation.isPending}>
                        <X size={12} className="mr-1" />取消
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>

        {/* Invite Dialog */}
        <Dialog open={showInvite} onOpenChange={setShowInvite}>
          <DialogContent>
            <DialogHeader><DialogTitle>邀请成员</DialogTitle></DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2"><Label>邮箱地址 *</Label><Input value={inviteEmail} onChange={e => setInviteEmail(e.target.value)} type="email" placeholder="colleague@example.com" /></div>
              <div className="space-y-2"><Label>角色</Label><div className="flex gap-2">{[{ v: "admin", l: "管理员" }, { v: "member", l: "成员" }].map(o => <Button key={o.v} variant={inviteRole === o.v ? "default" : "outline"} size="sm" onClick={() => setInviteRole(o.v)}>{o.l}</Button>)}</div></div>
              {inviteMutation.isError && <p className="text-sm text-destructive">{inviteMutation.error instanceof Error ? inviteMutation.error.message : "邀请失败"}</p>}
            </div>
            <DialogFooter><Button variant="outline" onClick={() => setShowInvite(false)}>取消</Button><Button onClick={handleInvite} disabled={inviteMutation.isPending}>{inviteMutation.isPending ? "发送中..." : "发送邀请"}</Button></DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Change Role Dialog */}
        <Dialog open={editingMember !== null} onOpenChange={() => setEditingMember(null)}>
          <DialogContent>
            <DialogHeader><DialogTitle>修改角色</DialogTitle></DialogHeader>
            {editingMember && (
              <div className="space-y-4">
                <div className="p-3 bg-muted rounded-lg"><p className="font-medium">{editingMember.name}</p><p className="text-xs text-muted-foreground">{editingMember.email}</p></div>
                <div className="space-y-2"><Label>角色</Label><div className="flex gap-2">{[{ v: "admin", l: "管理员" }, { v: "member", l: "成员" }].map(o => <Button key={o.v} variant={newRole === o.v ? "default" : "outline"} size="sm" onClick={() => setNewRole(o.v)}>{o.l}</Button>)}</div></div>
                {roleMutation.isError && <p className="text-sm text-destructive">{roleMutation.error instanceof Error ? roleMutation.error.message : "修改失败"}</p>}
              </div>
            )}
            <DialogFooter><Button variant="outline" onClick={() => setEditingMember(null)}>取消</Button><Button onClick={handleRoleSave} disabled={roleMutation.isPending}>{roleMutation.isPending ? "保存中..." : "保存"}</Button></DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Transfer Ownership Dialog (owner only) */}
        <Dialog open={showTransfer} onOpenChange={setShowTransfer}>
          <DialogContent>
            <DialogHeader><DialogTitle>转让企业所有权</DialogTitle></DialogHeader>
            <div className="space-y-3">
              <p className="text-sm text-muted-foreground">选择一位管理员接收所有权。转让后您将变为普通管理员，此操作不可撤销。</p>
              {adminCandidates.length === 0 ? (
                <div className="p-4 text-center text-sm text-muted-foreground border rounded-lg">暂无可用管理员，请先将成员提升为管理员</div>
              ) : (
                <div className="space-y-2">
                  {adminCandidates.map(m => (
                    <div key={m.id} className="flex items-center justify-between p-3 border rounded-lg">
                      <div><p className="font-medium text-sm">{m.name}</p><p className="text-xs text-muted-foreground">{m.email}</p></div>
                      <Button size="sm" onClick={() => handleTransfer(m)} disabled={transferMutation.isPending}>转让</Button>
                    </div>
                  ))}
                </div>
              )}
              {transferMutation.isError && <p className="text-sm text-destructive">{transferMutation.error instanceof Error ? transferMutation.error.message : "转让失败"}</p>}
            </div>
            <DialogFooter><Button variant="outline" onClick={() => setShowTransfer(false)}>关闭</Button></DialogFooter>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
