import { useMemo, useState } from "react";
import { useAuth } from "../lib/auth";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import {
  Ban,
  CheckCircle2,
  Coins,
  Edit,
  Search,
  Trash2,
  UserPlus,
  Users,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { EmptyState, LoadingState } from "@/components/StateViews";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

interface MemberRow {
  id: string;
  name: string;
  email: string;
  role: string;
  status: string;
}

interface QuotaAllocation {
  user_id: string;
  allocated: number;
  used: number;
  remaining: number;
}

interface QuotaPool {
  id: string;
  dimension: string;
  total_amount: number;
  allocated: number;
  used: number;
  remaining: number;
  unit_name: string;
  allocations: QuotaAllocation[];
}

/** Per-member quota aggregated across all pools for the list column. */
interface MemberQuota {
  allocated: number;
  used: number;
}

interface CreateSubAccountVars {
  email: string;
  display_name: string;
  password: string;
  role: string;
}

interface AllocateQuotaVars {
  user_id: string;
  pool_id: string;
  amount: number;
}

const STATUS_META: Record<string, { label: string; variant: "success" | "destructive" | "secondary" }> = {
  active: { label: "正常", variant: "success" },
  suspended: { label: "已停用", variant: "destructive" },
  left: { label: "已离开", variant: "secondary" },
};

const ROLE_OPTIONS = [
  { v: "admin", l: "管理员" },
  { v: "member", l: "成员" },
];

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

function fmtNum(n: number | undefined): string {
  return (n ?? 0).toLocaleString("en-US");
}

export default function TeamManagement() {
  const { user } = useAuth();
  const isOwner = user?.tenant_role === "owner";

  const {
    data,
    isLoading,
    error: membersError,
    refetch,
  } = useConsoleQuery<{ members: MemberRow[] }>("/team");
  const members: MemberRow[] = data?.members ?? [];

  const { data: quotaData } = useConsoleQuery<{ pools: QuotaPool[] }>("/team/quotas");
  const pools: QuotaPool[] = quotaData?.pools ?? [];

  // Aggregate each member's quota across all pools for the list column.
  const quotaByUser = useMemo(() => {
    const map = new Map<string, MemberQuota>();
    for (const pool of pools) {
      for (const a of pool.allocations) {
        const prev = map.get(a.user_id);
        map.set(a.user_id, {
          allocated: (prev?.allocated ?? 0) + a.allocated,
          used: (prev?.used ?? 0) + a.used,
        });
      }
    }
    return map;
  }, [pools]);

  const allocatedTotal = pools.reduce((sum, p) => sum + p.allocated, 0);
  const headroomTotal = pools.reduce((sum, p) => sum + p.remaining, 0);

  // --- Mutations ---
  const createMutation = useConsoleMutation<unknown, CreateSubAccountVars>(
    "post", "/team/members", "/team",
  );
  const allocateMutation = useConsoleMutation<unknown, AllocateQuotaVars>(
    "post", "/team/quotas/allocate", "/team/quotas",
  );
  const removeMutation = useConsoleMutation<unknown, { id: string }>(
    "delete", v => `/team/${v.id}`, "/team",
  );
  const roleMutation = useConsoleMutation<unknown, { id: string; role: string }>(
    "put", v => `/team/${v.id}/role`, "/team",
  );
  const statusMutation = useConsoleMutation<unknown, { id: string; status: string }>(
    "put", v => `/team/${v.id}/status`, "/team",
  );

  // Latest error from a row action, surfaced in a banner so a failure is never
  // silently swallowed.
  const rowActionError =
    removeMutation.error ?? statusMutation.error ?? roleMutation.error;

  // --- Create sub-account dialog ---
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState({
    email: "",
    display_name: "",
    password: "",
    role: "member",
  });
  const createValid =
    createForm.email.includes("@") &&
    createForm.display_name.trim().length > 0 &&
    createForm.password.length >= 8;

  async function handleCreate() {
    if (!createValid) return;
    try {
      await createMutation.mutateAsync({
        email: createForm.email.trim(),
        display_name: createForm.display_name.trim(),
        password: createForm.password,
        role: createForm.role,
      });
      setShowCreate(false);
      setCreateForm({ email: "", display_name: "", password: "", role: "member" });
    } catch {
      /* surfaced inside the dialog */
    }
  }

  // --- Change role dialog (owner only) ---
  const [editingMember, setEditingMember] = useState<MemberRow | null>(null);
  const [newRole, setNewRole] = useState("");
  function openRoleEdit(m: MemberRow) {
    roleMutation.reset();
    setEditingMember(m);
    setNewRole(m.role);
  }
  async function handleRoleSave() {
    if (!editingMember) return;
    try {
      await roleMutation.mutateAsync({ id: editingMember.id, role: newRole });
      setEditingMember(null);
    } catch {
      /* surfaced inside the dialog */
    }
  }

  // --- Allocate quota dialog ---
  // The backend allocation is additive: the amount is added to the member's
  // existing share, and the atomic capacity check rejects anything that would
  // push the pool past its total.
  const [allocMember, setAllocMember] = useState<MemberRow | null>(null);
  const [allocPoolId, setAllocPoolId] = useState("");
  const [allocAmount, setAllocAmount] = useState("");
  const allocPool = pools.find(p => p.id === allocPoolId) ?? null;
  const allocCurrent = allocPool?.allocations.find(a => a.user_id === allocMember?.id);
  const allocAmountNum = Number(allocAmount);
  const allocValid =
    !!allocMember &&
    !!allocPool &&
    Number.isInteger(allocAmountNum) &&
    allocAmountNum > 0 &&
    allocAmountNum <= allocPool.remaining;

  function openAllocate(m: MemberRow) {
    allocateMutation.reset();
    setAllocMember(m);
    setAllocPoolId(pools.length > 0 ? pools[0].id : "");
    setAllocAmount("");
  }
  async function handleAllocate() {
    if (!allocMember || !allocPool || !allocValid) return;
    try {
      await allocateMutation.mutateAsync({
        user_id: allocMember.id,
        pool_id: allocPool.id,
        amount: allocAmountNum,
      });
      setAllocMember(null);
      setAllocAmount("");
    } catch {
      /* surfaced inside the dialog */
    }
  }

  // --- Status toggle ---
  async function handleToggleStatus(m: MemberRow) {
    const next = m.status === "suspended" ? "active" : "suspended";
    const verb = next === "suspended" ? "停用" : "启用";
    if (!confirm(`确定${verb}成员 ${m.email} 吗？`)) return;
    try {
      await statusMutation.mutateAsync({ id: m.id, status: next });
    } catch {
      /* handled by the banner */
    }
  }
  function canToggleStatus(m: MemberRow): boolean {
    if (m.id === user?.id) return false;
    if (m.role === "owner") return false;
    if (m.role === "admin" && !isOwner) return false;
    return true;
  }

  // --- Remove member ---
  async function handleRemove(m: MemberRow) {
    if (!confirm(`确定移除成员 ${m.email} 吗？`)) return;
    try {
      await removeMutation.mutateAsync({ id: m.id });
    } catch {
      /* handled by the banner */
    }
  }

  // --- Search filter ---
  const [query, setQuery] = useState("");
  const filtered = query.trim()
    ? members.filter(m =>
        m.name.toLowerCase().includes(query.toLowerCase()) ||
        m.email.toLowerCase().includes(query.toLowerCase()),
      )
    : members;

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>团队管理</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <LoadingState message="加载团队成员..." />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  if (membersError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>团队管理</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <Card className="border-destructive/30">
            <CardContent className="p-6 text-center">
              <p className="text-sm text-destructive mb-3">
                {membersError instanceof Error ? membersError.message : "加载失败"}
              </p>
              <Button variant="outline" onClick={() => refetch()}>重试</Button>
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>团队管理</SectionPageLayout.Title>
          <SectionPageLayout.Description>
            共 {members.length} 人
            {pools.length > 0
              ? ` · 企业剩余可分配 ${fmtNum(headroomTotal)} ${pools[0].unit_name}`
              : ""}
          </SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
        <SectionPageLayout.Actions>
          <Button
            onClick={() => {
              createMutation.reset();
              setCreateForm({ email: "", display_name: "", password: "", role: "member" });
              setShowCreate(true);
            }}
          >
            <UserPlus size={16} className="mr-1.5" />添加子账号
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

        {pools.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4">
            <Card>
              <CardContent className="p-4">
                <p className="text-xs text-muted-foreground">配额池数量</p>
                <p className="text-xl font-semibold mt-1">{pools.length}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-xs text-muted-foreground">企业剩余可分配</p>
                <p className="text-xl font-semibold mt-1">
                  {fmtNum(headroomTotal)}
                  <span className="ml-1 text-sm font-normal text-muted-foreground">{pools[0].unit_name}</span>
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-xs text-muted-foreground">已分配总量</p>
                <p className="text-xl font-semibold mt-1">
                  {fmtNum(allocatedTotal)}
                  <span className="ml-1 text-sm font-normal text-muted-foreground">{pools[0].unit_name}</span>
                </p>
              </CardContent>
            </Card>
          </div>
        )}

        <div className="mb-4 relative max-w-sm">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="搜索姓名或邮箱..."
            className="pl-9 h-9 text-sm"
          />
          {query && (
            <Button
              variant="ghost"
              size="icon"
              className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
              onClick={() => setQuery("")}
            >
              <X size={14} />
            </Button>
          )}
        </div>

        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>姓名</TableHead>
                <TableHead>邮箱</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>配额</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <EmptyState icon={Users} title={query ? "未找到匹配成员" : "暂无团队成员"} />
                  </TableCell>
                </TableRow>
              )}
              {filtered.map(m => {
                const q = quotaByUser.get(m.id);
                return (
                  <TableRow key={m.id}>
                    <TableCell className="font-medium text-sm">{m.name}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{m.email}</TableCell>
                    <TableCell>
                      <Badge variant={roleBadge(m.role)}>{roleLabel(m.role)}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={STATUS_META[m.status]?.variant ?? "secondary"}>
                        {STATUS_META[m.status]?.label ?? m.status}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {!q || q.allocated <= 0 ? (
                        <span className="text-sm text-muted-foreground">—</span>
                      ) : (
                        <div className="text-sm">
                          <div>已分配 {fmtNum(q.allocated)}</div>
                          <div className="text-xs text-muted-foreground">已用 {fmtNum(q.used)}</div>
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex gap-1 justify-end">
                        {m.role === "owner" ? (
                          <span className="text-xs text-muted-foreground self-center">所有者不可操作</span>
                        ) : (
                          <>
                            <Button variant="outline" size="sm" onClick={() => openAllocate(m)}>
                              <Coins size={12} className="mr-1" />分配额度
                            </Button>
                            {canToggleStatus(m) && (
                              <Button variant="outline" size="sm" onClick={() => handleToggleStatus(m)}>
                                {m.status === "suspended" ? (
                                  <><CheckCircle2 size={12} className="mr-1" />启用</>
                                ) : (
                                  <><Ban size={12} className="mr-1" />停用</>
                                )}
                              </Button>
                            )}
                            {isOwner && (
                              <Button variant="outline" size="sm" onClick={() => openRoleEdit(m)}>
                                <Edit size={12} className="mr-1" />改角色
                              </Button>
                            )}
                            {m.id !== user?.id && (
                              <Button
                                variant="outline"
                                size="sm"
                                aria-label="移除成员"
                                className="hover:text-destructive"
                                onClick={() => handleRemove(m)}
                              >
                                <Trash2 size={12} />
                              </Button>
                            )}
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>

        {/* Create sub-account dialog */}
        <Dialog open={showCreate} onOpenChange={setShowCreate}>
          <DialogContent>
            <DialogHeader><DialogTitle>添加子账号</DialogTitle></DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>邮箱地址 *</Label>
                <Input
                  value={createForm.email}
                  onChange={e => setCreateForm(f => ({ ...f, email: e.target.value }))}
                  type="email"
                  placeholder="colleague@company.com"
                />
              </div>
              <div className="space-y-2">
                <Label>姓名 *</Label>
                <Input
                  value={createForm.display_name}
                  onChange={e => setCreateForm(f => ({ ...f, display_name: e.target.value }))}
                  placeholder="张三"
                />
              </div>
              <div className="space-y-2">
                <Label>初始密码 *（至少 8 位）</Label>
                <Input
                  value={createForm.password}
                  onChange={e => setCreateForm(f => ({ ...f, password: e.target.value }))}
                  type="password"
                  placeholder="至少 8 位"
                />
                {createForm.password.length > 0 && createForm.password.length < 8 && (
                  <p className="text-xs text-destructive">密码至少 8 位</p>
                )}
              </div>
              <div className="space-y-2">
                <Label>角色</Label>
                <div className="flex gap-2">
                  {ROLE_OPTIONS.map(o => (
                    <Button
                      key={o.v}
                      type="button"
                      variant={createForm.role === o.v ? "default" : "outline"}
                      size="sm"
                      onClick={() => setCreateForm(f => ({ ...f, role: o.v }))}
                    >
                      {o.l}
                    </Button>
                  ))}
                </div>
              </div>
              {createMutation.isError && (
                <p className="text-sm text-destructive">
                  {createMutation.error instanceof Error ? createMutation.error.message : "创建失败"}
                </p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowCreate(false)}>取消</Button>
              <Button onClick={handleCreate} disabled={!createValid || createMutation.isPending}>
                {createMutation.isPending ? "创建中..." : "创建子账号"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Allocate quota dialog */}
        <Dialog open={allocMember !== null} onOpenChange={() => setAllocMember(null)}>
          <DialogContent>
            <DialogHeader><DialogTitle>分配额度</DialogTitle></DialogHeader>
            {allocMember && (
              <div className="space-y-4">
                <div className="p-3 bg-muted rounded-lg">
                  <p className="font-medium">{allocMember.name}</p>
                  <p className="text-xs text-muted-foreground">{allocMember.email}</p>
                </div>

                {pools.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    企业暂无配额池，请联系平台管理员创建后再分配。
                  </p>
                ) : (
                  <>
                    <div className="space-y-2">
                      <Label>配额池</Label>
                      <div className="space-y-2">
                        {pools.map(p => (
                          <button
                            key={p.id}
                            type="button"
                            onClick={() => {
                              setAllocPoolId(p.id);
                              setAllocAmount("");
                            }}
                            className={`w-full flex items-center justify-between px-3 py-2 rounded-lg border text-sm text-left ${
                              allocPoolId === p.id
                                ? "border-primary bg-primary/5"
                                : "border-input hover:bg-muted"
                            }`}
                          >
                            <span>{p.unit_name || "token"} 池</span>
                            <span className="text-xs text-muted-foreground">
                              剩余可分配 {fmtNum(p.remaining)}
                            </span>
                          </button>
                        ))}
                      </div>
                    </div>

                    {allocPool && (
                      <>
                        <div className="grid grid-cols-3 gap-2 text-sm">
                          <div className="p-2 bg-muted rounded-lg">
                            <p className="text-xs text-muted-foreground">该成员已分配</p>
                            <p className="font-medium">{fmtNum(allocCurrent?.allocated ?? 0)}</p>
                          </div>
                          <div className="p-2 bg-muted rounded-lg">
                            <p className="text-xs text-muted-foreground">已用</p>
                            <p className="font-medium">{fmtNum(allocCurrent?.used ?? 0)}</p>
                          </div>
                          <div className="p-2 bg-muted rounded-lg">
                            <p className="text-xs text-muted-foreground">本池可分配</p>
                            <p className="font-medium">{fmtNum(allocPool.remaining)}</p>
                          </div>
                        </div>
                        <div className="space-y-2">
                          <Label>追加额度 *（最多 {fmtNum(allocPool.remaining)}）</Label>
                          <Input
                            value={allocAmount}
                            onChange={e => setAllocAmount(e.target.value.replace(/[^0-9]/g, ""))}
                            type="text"
                            inputMode="numeric"
                            placeholder="例如 1000"
                          />
                          {allocAmount && !allocValid && (
                            <p className="text-xs text-destructive">
                              {allocAmountNum > allocPool.remaining
                                ? "超出企业剩余可分配额度"
                                : "请输入正整数"}
                            </p>
                          )}
                        </div>
                      </>
                    )}
                    {allocateMutation.isError && (
                      <p className="text-sm text-destructive">
                        {allocateMutation.error instanceof Error
                          ? allocateMutation.error.message
                          : "分配失败"}
                      </p>
                    )}
                  </>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setAllocMember(null)}>取消</Button>
              {pools.length > 0 && (
                <Button onClick={handleAllocate} disabled={!allocValid || allocateMutation.isPending}>
                  {allocateMutation.isPending ? "分配中..." : "确认分配"}
                </Button>
              )}
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Change role dialog (owner only) */}
        <Dialog open={editingMember !== null} onOpenChange={() => setEditingMember(null)}>
          <DialogContent>
            <DialogHeader><DialogTitle>修改角色</DialogTitle></DialogHeader>
            {editingMember && (
              <div className="space-y-4">
                <div className="p-3 bg-muted rounded-lg">
                  <p className="font-medium">{editingMember.name}</p>
                  <p className="text-xs text-muted-foreground">{editingMember.email}</p>
                </div>
                <div className="space-y-2">
                  <Label>角色</Label>
                  <div className="flex gap-2">
                    {ROLE_OPTIONS.map(o => (
                      <Button
                        key={o.v}
                        type="button"
                        variant={newRole === o.v ? "default" : "outline"}
                        size="sm"
                        onClick={() => setNewRole(o.v)}
                      >
                        {o.l}
                      </Button>
                    ))}
                  </div>
                </div>
                {roleMutation.isError && (
                  <p className="text-sm text-destructive">
                    {roleMutation.error instanceof Error ? roleMutation.error.message : "修改失败"}
                  </p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditingMember(null)}>取消</Button>
              <Button onClick={handleRoleSave} disabled={roleMutation.isPending}>
                {roleMutation.isPending ? "保存中..." : "保存"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
