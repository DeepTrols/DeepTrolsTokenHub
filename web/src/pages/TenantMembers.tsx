import { EmptyState, ErrorState } from "@/components/StateViews";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { ArrowLeft, Plus, Trash2, Users } from "lucide-react";

interface Member { id: string; name: string; email: string; role: string; status: string; joined_at: string }
interface TenantDetail { id: string; code: string; name: string; status: string }

const ROLE_LABEL: Record<string, string> = { owner: "所有者", admin: "管理员", member: "成员" };
const STATUS_META: Record<string, { label: string; variant: "success" | "destructive" | "secondary" }> = {
  active: { label: "正常", variant: "success" },
  suspended: { label: "已停用", variant: "destructive" },
  left: { label: "已离开", variant: "secondary" },
};

export default function TenantMembers() {
  const { id } = useParams<{ id: string }>();
  const membersPath = id ? "/tenants/" + id + "/members" : "";

  const { data: md, isLoading, isError, error, refetch } = useAdminQuery<{ data: Member[]; total: number }>(membersPath, { enabled: !!id });
  const { data: td } = useAdminQuery<{ data: TenantDetail }>(id ? "/tenants/" + id : "", { enabled: !!id });
  const members = md?.data ?? [];

  const [em, setEm] = useState("");
  const [rl, setRl] = useState("member");
  const addM = useAdminMutation<unknown, { tid: string; email: string; role: string }>("post", v => "/tenants/" + v.tid + "/members", v => "/tenants/" + v.tid + "/members");
  const rmM = useAdminMutation<unknown, { tid: string; userId: string }>("delete", v => "/tenants/" + v.tid + "/members/" + v.userId, v => "/tenants/" + v.tid + "/members");
  const chR = useAdminMutation<unknown, { tid: string; userId: string; role: string }>("put", v => "/tenants/" + v.tid + "/members/" + v.userId + "/role", v => "/tenants/" + v.tid + "/members");

  const ha = async () => {
    if (!id || !em.trim()) return;
    try {
      await addM.mutateAsync({ tid: id, email: em.trim(), role: rl });
      setEm("");
      setRl("member");
    } catch (e) { }
  };
  const hr = async (u: Member) => {
    if (!id) return;
    if (!confirm("移除成员 " + u.email + " 吗？")) return;
    try { await rmM.mutateAsync({ tid: id, userId: u.id }); } catch (e) { }
  };
  const hc = async (u: Member) => {
    if (!id || u.role === "owner") return;
    try { await chR.mutateAsync({ tid: id, userId: u.id, role: u.role === "admin" ? "member" : "admin" }); } catch (e) { }
  };
  const le = isError ? (error instanceof Error ? error.message : String(error)) : "";

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <Link to="/admin/tenants" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground mb-2">
            <ArrowLeft size={14} />返回租户管理
          </Link>
          <h2 className="text-2xl font-bold">{td?.data?.name ?? "成员管理"}</h2>
          <p className="text-sm text-muted-foreground mt-1">管理该企业下的所有成员（共 {md?.total ?? 0} 人）</p>
        </div>
      </div>

      {le && <ErrorState onRetry={() => refetch()} />}

      <Card className="mb-4"><CardContent className="p-4">
        <div className="flex items-end gap-3">
          <div className="space-y-2 flex-1"><Label>邮箱</Label><Input value={em} onChange={e => setEm(e.target.value)} type="email" placeholder="user@example.com" /></div>
          <div className="space-y-2 w-32"><Label>角色</Label>
            <Select value={rl} onValueChange={setRl}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="member">成员</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select>
          </div>
          <Button onClick={ha} disabled={!em.trim() || addM.isPending}><Plus size={16} className="mr-1.5" />添加成员</Button>
        </div>
      </CardContent></Card>

      <Card className="overflow-hidden">
        {isLoading ? (
          <CardContent className="p-12 text-center text-muted-foreground">加载中...</CardContent>
        ) : members.length === 0 ? (
          <CardContent className="p-12 text-center text-muted-foreground"><Users size={40} className="mx-auto mb-3 opacity-30" /><p>暂无成员</p></CardContent>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50 text-left text-muted-foreground">
                <th className="px-4 py-2 font-medium">成员</th>
                <th className="px-4 py-2 font-medium">角色</th>
                <th className="px-4 py-2 font-medium">状态</th>
                <th className="px-4 py-2 font-medium">加入时间</th>
                <th className="px-4 py-2 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {members.map(u => (
                <tr key={u.id} className="border-b last:border-0">
                  <td className="px-4 py-3"><p className="font-medium">{u.email}</p>{u.name && <p className="text-xs text-muted-foreground">{u.name}</p>}</td>
                  <td className="px-4 py-3"><Badge variant={u.role === "owner" ? "default" : u.role === "admin" ? "secondary" : "outline"}>{ROLE_LABEL[u.role] ?? u.role}</Badge></td>
                  <td className="px-4 py-3"><Badge variant={STATUS_META[u.status]?.variant ?? "secondary"}>{STATUS_META[u.status]?.label ?? u.status}</Badge></td>
                  <td className="px-4 py-3 text-muted-foreground">{u.joined_at ? new Date(u.joined_at).toLocaleDateString() : "—"}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex gap-1 justify-end">
                      {u.role !== "owner" && <Button variant="outline" size="sm" onClick={() => hc(u)}>{u.role === "admin" ? "降为成员" : "提升管理员"}</Button>}
                      {u.role !== "owner" && <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => hr(u)}><Trash2 size={12} /></Button>}
                      {u.role === "owner" && <span className="text-xs text-muted-foreground self-center">所有者不可操作</span>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}
