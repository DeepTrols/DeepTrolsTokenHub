import { toast } from "sonner";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState } from "react";
import { useAdminQuery, useAdminMutation } from "../lib/hooks/use-api";
import { Pencil, Plus, Trash2, Users, Wallet } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

interface QuotaPool {
  id: string; tenant_id: string; tenant_name: string; model_id: string; model_code: string;
  model_name: string; dimension: string; total_amount: number; allocated_amount: number;
  used_amount: number; unit_name: string;
}

interface TenantOption {
  id: string;
  name: string;
  code: string;
}

interface ModelOption {
  id: string;
  code: string;
  display_name: string;
}

function fmtNum(n: number): string {
  if (n >= 1e9) return (n / 1e9).toFixed(1) + "B";
  if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(1) + "K";
  return String(n);
}

function usagePct(used: number, total: number): number { return total <= 0 ? 0 : Math.min(100, Math.round((used / total) * 100)); }
function pctColor(pct: number): string { if (pct >= 90) return "bg-red-500"; if (pct >= 70) return "bg-yellow-500"; return "bg-green-500"; }

export default function QuotaManagement() {
  const { data: quotaData, isLoading, isError, error, refetch } = useAdminQuery<{ data: QuotaPool[]; total: number }>("/quotas");
  const pools = Array.isArray(quotaData?.data) ? quotaData.data : [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  // 创建配额池需要从租户/模型目录选择作用域，而不是让管理员手填 UUID。
  const { data: tenantsData } = useAdminQuery<{ data: TenantOption[]; total: number }>("/tenants");
  const { data: modelsData } = useAdminQuery<{ data: ModelOption[]; total: number }>("/models");
  const tenants = tenantsData?.data ?? [];
  const models = modelsData?.data ?? [];

  const createMut = useAdminMutation<unknown, Record<string, unknown>>("post", "/quotas");
  const allocateMut = useAdminMutation<unknown, { poolId: string } & Record<string, unknown>>("post", (v) => `/quotas/${v.poolId}/allocate`);
  // 编辑/删除：poolId 只用于拼路径；PUT 的 body 会带上 poolId，handler 按需解码 total_amount/unit_name/dimension，多余字段忽略。
  const updateMut = useAdminMutation<unknown, { poolId: string } & Record<string, unknown>>("put", (v) => `/quotas/${v.poolId}`, "/quotas");
  const deleteMut = useAdminMutation<unknown, { poolId: string }>("delete", (v) => `/quotas/${v.poolId}`, "/quotas");

  const [showCreate, setShowCreate] = useState(false);
  const [showAllocate, setShowAllocate] = useState(false);
  const [allocPool, setAllocPool] = useState<QuotaPool | null>(null);
  const [newPool, setNewPool] = useState({ tenant_id: "", model_id: "", total_amount: 1000000, unit_name: "token", dimension: "token" });
  const [alloc, setAlloc] = useState({ user_id: "", amount: 10000 });
  const [showEdit, setShowEdit] = useState(false);
  const [editPool, setEditPool] = useState<QuotaPool | null>(null);
  const [editForm, setEditForm] = useState({ total_amount: 0, unit_name: "token", dimension: "token" });
  const [delPool, setDelPool] = useState<QuotaPool | null>(null);

  const handleCreate = async () => {
    await createMut.mutateAsync({
      total_amount: newPool.total_amount, unit_name: newPool.unit_name, dimension: newPool.dimension,
      // tenant_id is required (schema NOT NULL); the create button is disabled
      // until it is chosen, so always send it. model_id is optional.
      tenant_id: newPool.tenant_id,
      ...(newPool.model_id ? { model_id: newPool.model_id } : {}),
    });
    setShowCreate(false); refetch();
  };

  const handleAllocate = async () => {
    if (!allocPool) return;
    await allocateMut.mutateAsync({ poolId: allocPool.id, user_id: alloc.user_id, amount: alloc.amount });
    setShowAllocate(false); setAllocPool(null); refetch();
  };

  const handleUpdate = async () => {
    if (!editPool) return;
    try {
      await updateMut.mutateAsync({ poolId: editPool.id, ...editForm });
      setShowEdit(false); setEditPool(null); refetch();
    } catch (err) {
      // 后端 400（如新总量低于已分配）会带明确错误信息，必须透出给管理员而不是静默失败。
      toast.error(err instanceof Error ? err.message : "更新配额池失败");
    }
  };

  const handleDelete = async () => {
    if (!delPool) return;
    try {
      await deleteMut.mutateAsync({ poolId: delPool.id });
      setDelPool(null); refetch();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除配额池失败");
    }
  };

  const totalQuota = pools.reduce((s, p) => s + p.total_amount, 0);
  const totalUsed = pools.reduce((s, p) => s + p.used_amount, 0);

  if (isLoading) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>配额管理</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><LoadingState message="加载配额数据..." /></SectionPageLayout.Content></SectionPageLayout>;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div><h2 className="text-2xl font-bold">配额管理</h2><p className="text-sm text-muted-foreground mt-1">管理租户和模型的 Token 配额池及用户分配</p></div>
        <Button onClick={() => setShowCreate(true)}><Plus size={16} className="mr-1.5" />创建配额池</Button>
      </div>

      {loadError && <ErrorState error={loadError} onRetry={() => refetch()} />}

      <div className="grid grid-cols-3 gap-4 mb-6">
        {[{ label: "配额池总数", v: pools.length }, { label: "总配额量", v: fmtNum(totalQuota) }, { label: "已使用", v: fmtNum(totalUsed) }].map(c => <Card key={c.label}><CardContent className="p-5"><p className="text-sm text-muted-foreground">{c.label}</p><p className="text-2xl font-bold mt-1">{String(c.v)}</p></CardContent></Card>)}
      </div>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader><TableRow><TableHead>租户</TableHead><TableHead>模型</TableHead><TableHead>总量</TableHead><TableHead>已分配</TableHead><TableHead>已使用</TableHead><TableHead>使用率</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
          <TableBody>
            {pools.length === 0 && <TableRow><TableCell colSpan={7}><EmptyState icon={Wallet} title="暂无配额数据" /></TableCell></TableRow>}
            {pools.map((p) => {
              const pct = usagePct(p.used_amount, p.total_amount);
              return <TableRow key={p.id} className="hover:bg-muted/30">
                <TableCell className="font-medium">{p.tenant_name || p.tenant_id || "—"}</TableCell>
                <TableCell>{p.model_name || p.model_code || "全部模型"}</TableCell>
                <TableCell className="font-mono text-xs">{fmtNum(p.total_amount)} {p.unit_name}</TableCell>
                <TableCell className="font-mono text-xs">{fmtNum(p.allocated_amount)}</TableCell>
                <TableCell className="font-mono text-xs">{fmtNum(p.used_amount)}</TableCell>
                <TableCell><div className="flex items-center gap-2"><div className="w-20 h-2 bg-muted rounded-full overflow-hidden"><div className={`h-full rounded-full ${pctColor(pct)}`} style={{ width: `${pct}%` }} /></div><span className="text-xs text-muted-foreground">{pct}%</span></div></TableCell>
                <TableCell className="text-right">
                  <div className="flex gap-1 justify-end">
                    <Button variant="outline" size="sm" onClick={() => { setEditPool(p); setEditForm({ total_amount: p.total_amount, unit_name: p.unit_name, dimension: p.dimension }); setShowEdit(true); }}><Pencil size={12} className="mr-1" />编辑</Button>
                    <Button variant="outline" size="sm" onClick={() => { setAllocPool(p); setAlloc({ user_id: "", amount: 10000 }); setShowAllocate(true); }}><Users size={12} className="mr-1" />分配</Button>
                    <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => setDelPool(p)}><Trash2 size={12} className="mr-1" />删除</Button>
                  </div>
                </TableCell>
              </TableRow>;
            })}
          </TableBody>
        </Table>
      </Card>

      {/* Create Pool Dialog */}
      <Dialog open={showCreate} onOpenChange={setShowCreate}>
        <DialogContent>
          <DialogHeader><DialogTitle>创建配额池</DialogTitle><DialogDescription>创建租户或模型级别的 Token 配额池</DialogDescription></DialogHeader>
          {(!tenantsData || !modelsData) && (
            <p className="text-xs text-muted-foreground">加载租户/模型目录中...</p>
          )}
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>租户 *</Label>
              {/* tenant_id 在 schema 中为 NOT NULL，每个池必须归属一个租户；留空会 500。 */}
              <Select value={newPool.tenant_id} onValueChange={(v) => setNewPool({ ...newPool, tenant_id: v })}>
                <SelectTrigger><SelectValue placeholder="选择租户" /></SelectTrigger>
                <SelectContent>
                  {tenants.map((t) => <SelectItem key={t.id} value={t.id}>{t.name}（{t.code}）</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>模型</Label>
              <Select value={newPool.model_id || "none"} onValueChange={(v) => setNewPool({ ...newPool, model_id: v === "none" ? "" : v })}>
                <SelectTrigger><SelectValue placeholder="留空 = 所有模型" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">留空 = 所有模型</SelectItem>
                  {models.map((m) => <SelectItem key={m.id} value={m.id}>{m.display_name || m.code}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2"><Label>总量</Label><Input type="number" min={1} value={newPool.total_amount} onChange={e => setNewPool({ ...newPool, total_amount: Number(e.target.value) })} /></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setShowCreate(false)}>取消</Button><Button onClick={handleCreate} disabled={!newPool.tenant_id || createMut.isPending}>{createMut.isPending ? "创建中..." : "创建"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Allocate Dialog */}
      <Dialog open={showAllocate} onOpenChange={setShowAllocate}>
        <DialogContent>
          <DialogHeader><DialogTitle>分配配额</DialogTitle><DialogDescription>{allocPool ? `从 "${allocPool.tenant_name || allocPool.model_code || allocPool.id}" 分配配额给用户` : ""}</DialogDescription></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>用户 ID</Label><Input value={alloc.user_id} onChange={e => setAlloc({ ...alloc, user_id: e.target.value })} placeholder="UUID" /></div>
            <div className="space-y-2"><Label>分配数量</Label><Input type="number" min={1} value={alloc.amount} onChange={e => setAlloc({ ...alloc, amount: Number(e.target.value) })} /></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setShowAllocate(false)}>取消</Button><Button onClick={handleAllocate} disabled={allocateMut.isPending}>{allocateMut.isPending ? "分配中..." : "确认分配"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Pool Dialog */}
      <Dialog open={showEdit} onOpenChange={setShowEdit}>
        <DialogContent>
          <DialogHeader><DialogTitle>编辑配额池</DialogTitle><DialogDescription>{editPool ? `调整 "${editPool.tenant_name || editPool.model_code || editPool.id}" 的总量与单位（作用域不可修改）` : ""}</DialogDescription></DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2"><Label>总量 *</Label><Input type="number" min={1} value={editForm.total_amount} onChange={e => setEditForm({ ...editForm, total_amount: Number(e.target.value) })} /></div>
            <div className="space-y-2"><Label>单位</Label><Input value={editForm.unit_name} onChange={e => setEditForm({ ...editForm, unit_name: e.target.value })} placeholder="token" /></div>
            <div className="space-y-2"><Label>维度</Label><Input value={editForm.dimension} onChange={e => setEditForm({ ...editForm, dimension: e.target.value })} placeholder="token" /></div>
            <p className="text-xs text-muted-foreground">新总量不能低于已分配数量（{editPool ? editPool.allocated_amount : 0}）。</p>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setShowEdit(false)}>取消</Button><Button onClick={handleUpdate} disabled={editForm.total_amount <= 0 || updateMut.isPending}>{updateMut.isPending ? "保存中..." : "保存"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Pool Confirm Dialog */}
      <Dialog open={delPool !== null} onOpenChange={() => setDelPool(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>删除配额池</DialogTitle><DialogDescription>{delPool ? `"${delPool.tenant_name || delPool.model_code || delPool.id}"` : ""}</DialogDescription></DialogHeader>
          <p className="text-sm text-muted-foreground">删除后该配额池及其所有用户分配、配额台账记录将被永久移除，此操作不可撤销。</p>
          <DialogFooter><Button variant="outline" onClick={() => setDelPool(null)}>取消</Button><Button variant="destructive" onClick={handleDelete} disabled={deleteMut.isPending}>{deleteMut.isPending ? "删除中..." : "确认删除"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
