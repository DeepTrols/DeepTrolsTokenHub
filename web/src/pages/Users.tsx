import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState, useMemo } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Search, Edit, Trash2, Users as UsersIcon, X, Plus } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

interface UserRow { id: string; email: string; display_name: string; user_type: string; tenant_id: string; tenant_name: string; role: string; status: string; balance: string; total_spend: string; }
function sb(s: string): "success" | "destructive" | "secondary" { if (s === "active") return "success"; if (s === "banned") return "destructive"; return "secondary"; }
function sl(s: string): string { if (s === "active") return "正常"; if (s === "banned") return "已封禁"; return s; }

export default function Users() {
  const { data: ud, isLoading, isError, error, refetch } = useAdminQuery<{ data: UserRow[]; total: number }>("/ledger");
  const all = ud?.data ?? []; const t = ud?.total ?? 0;
  const le = isError ? (error instanceof Error ? error.message : String(error)) : "";
  const [q, setQ] = useState("");
  const [ut, setUt] = useState("all");
  const users = useMemo(() => {
    let list = all;
    if (ut !== "all") list = list.filter(u => u.user_type === ut);
    if (q.trim()) { const lq = q.toLowerCase(); list = list.filter(u => u.email.toLowerCase().includes(lq) || (u.display_name || "").toLowerCase().includes(lq) || (u.tenant_name || "").toLowerCase().includes(lq)); }
    return list;
  }, [all, q, ut]);

  // Edit dialog
  const [ed, setEd] = useState<UserRow | null>(null); const [er, setEr] = useState(""); const [es, setEs] = useState("");
  const rm = useAdminMutation("put", v => "/users/" + (v as any).id + "/role", "/ledger");
  const sm = useAdminMutation("put", v => "/users/" + (v as any).id + "/status", "/ledger");
  const dm = useAdminMutation("delete", v => "/users/" + (v as any).id, "/ledger");
  const oe = (u: UserRow) => { setEd(u); setEr(u.role); setEs(u.status); };
  const hs = async () => { if (!ed) return; try { if (er !== ed.role) await rm.mutateAsync({ id: ed.id, role: er }); if (es !== ed.status) await sm.mutateAsync({ id: ed.id, status: es }); setEd(null); } catch (e) { } };
  const hd = async (u: UserRow) => { if (!confirm("确定删除用户 " + u.email + " 吗？")) return; try { await dm.mutateAsync({ id: u.id }); } catch (e) { } };

  // Create dialog
  const [sc, setSc] = useState(false);
  const [nm, setNm] = useState(""); const [em, setEm] = useState(""); const [pw, setPw] = useState("");
  const [cr, setCr] = useState("user");
  const cm = useAdminMutation("post", "/users", "/ledger");
  const hc = async () => { if (!em.trim() || !pw.trim()) return; try { await cm.mutateAsync({ email: em.trim(), password: pw.trim(), display_name: nm.trim() || em.trim(), role: cr }); setSc(false); setNm(""); setEm(""); setPw(""); setCr("user"); } catch (e) { } };

  if (isLoading) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>用户管理</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><LoadingState message="加载用户列表..." /></SectionPageLayout.Content></SectionPageLayout>;

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>用户管理</SectionPageLayout.Title>
          <SectionPageLayout.Description>共 {t} 人</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
        <SectionPageLayout.Actions>
          <Button onClick={() => { setNm(""); setEm(""); setPw(""); setCr("user"); setSc(true); }}><Plus size={16} className="mr-1.5" />创建用户</Button>
        </SectionPageLayout.Actions>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        <div className="mb-4 flex items-center gap-2">
          <div className="relative max-w-sm flex-1"><Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" /><Input value={q} onChange={e => setQ(e.target.value)} placeholder="搜索..." className="pl-9 h-9 text-sm" />{q && <Button variant="ghost" size="icon" className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7" onClick={() => setQ("")}><X size={14} /></Button>}</div>
          <Select value={ut} onValueChange={setUt}><SelectTrigger className="w-32 h-9 text-sm"><SelectValue placeholder="类型" /></SelectTrigger><SelectContent><SelectItem value="all">全部类型</SelectItem><SelectItem value="personal">个人</SelectItem><SelectItem value="enterprise">企业</SelectItem></SelectContent></Select>
        </div>
        {le && <ErrorState onRetry={() => refetch()} />}
        <Card className="overflow-hidden"><Table><TableHeader><TableRow><TableHead>用户</TableHead><TableHead>类型</TableHead><TableHead>所属企业</TableHead><TableHead>角色</TableHead><TableHead className="text-right">余额</TableHead><TableHead className="text-right">消费</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
          <TableBody>{users.length === 0 && <TableRow><TableCell colSpan={8}><EmptyState icon={UsersIcon} title={q || ut !== "all" ? "未找到" : "暂无用户"} /></TableCell></TableRow>}
            {users.map(u => <TableRow key={u.id}><TableCell><p className="font-medium text-sm">{u.email}</p></TableCell><TableCell><Badge variant={u.user_type === "enterprise" ? "default" : "secondary"}>{u.user_type === "enterprise" ? "企业" : "个人"}</Badge></TableCell><TableCell className="text-sm text-muted-foreground">{u.tenant_name || "—"}</TableCell><TableCell><Badge variant={u.role === "admin" ? "default" : "secondary"}>{u.role === "admin" ? "管理员" : "用户"}</Badge></TableCell><TableCell className="text-right font-mono text-sm">{parseFloat(u.balance || "0").toFixed(2)}</TableCell><TableCell className="text-right font-mono text-sm text-orange-500">{parseFloat(u.total_spend || "0").toFixed(2)}</TableCell><TableCell><Badge variant={sb(u.status)}>{sl(u.status)}</Badge></TableCell><TableCell className="text-right"><div className="flex gap-1 justify-end"><Button variant="outline" size="sm" onClick={() => oe(u)}><Edit size={12} className="mr-1" />编辑</Button><Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => hd(u)}><Trash2 size={12} /></Button></div></TableCell></TableRow>)}
          </TableBody></Table></Card>

        {/* Edit Dialog */}
        <Dialog open={ed !== null} onOpenChange={() => setEd(null)}><DialogContent><DialogHeader><DialogTitle>编辑用户</DialogTitle></DialogHeader>{ed && <div className="space-y-4"><div className="p-3 bg-muted rounded-lg"><p className="font-medium">{ed.email}</p></div><div className="space-y-2"><label className="text-sm font-medium">角色</label><div className="flex gap-2">{[{ v: "user", l: "用户" }, { v: "admin", l: "管理员" }].map(o => <Button key={o.v} variant={er === o.v ? "default" : "outline"} size="sm" onClick={() => setEr(o.v)}>{o.l}</Button>)}</div></div><div className="space-y-2"><label className="text-sm font-medium">状态</label><div className="flex gap-2">{[{ v: "active", l: "启用" }, { v: "banned", l: "禁用" }].map(o => <Button key={o.v} variant={es === o.v ? (o.v === "active" ? "default" : "destructive") : "outline"} size="sm" onClick={() => setEs(o.v)}>{o.l}</Button>)}</div></div></div>}<DialogFooter><Button variant="outline" onClick={() => setEd(null)}>取消</Button><Button onClick={hs}>保存</Button></DialogFooter></DialogContent></Dialog>

        {/* Create Dialog */}
        <Dialog open={sc} onOpenChange={setSc}><DialogContent><DialogHeader><DialogTitle>创建用户</DialogTitle></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>邮箱 *</Label><Input value={em} onChange={e => setEm(e.target.value)} type="email" placeholder="user@example.com" /></div>
            <div className="space-y-2"><Label>密码 *</Label><Input value={pw} onChange={e => setPw(e.target.value)} type="password" placeholder="至少 8 位" /></div>
            <div className="space-y-2"><Label>显示名称</Label><Input value={nm} onChange={e => setNm(e.target.value)} placeholder="留空使用邮箱" /></div>
            <div className="space-y-2"><Label>角色</Label><div className="flex gap-2">{[{ v: "user", l: "用户" }, { v: "admin", l: "管理员" }].map(o => <Button key={o.v} variant={cr === o.v ? "default" : "outline"} size="sm" onClick={() => setCr(o.v)}>{o.l}</Button>)}</div></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setSc(false)}>取消</Button><Button onClick={hc} disabled={cm.isPending}>{cm.isPending ? "创建中..." : "创建"}</Button></DialogFooter></DialogContent></Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
