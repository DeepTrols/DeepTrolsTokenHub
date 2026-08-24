import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../lib/auth";
import {
  consoleKey,
  useConsoleMutation,
  useConsoleQuery,
} from "../lib/hooks/use-api";
import { fmtMoney, isValidAmount, toCents, toMoneyInput } from "../lib/money";
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
  /** Personal-wallet spendable balance as a decimal string. */
  balance: string;
}

interface CreateSubAccountVars {
  email: string;
  display_name: string;
  password: string;
  role: string;
}

interface AllocateBalanceVars {
  user_id: string;
  amount: string;
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

export function TeamManagementContent() {
  const { user } = useAuth();
  const isOwner = user?.tenant_role === "owner";

  const {
    data,
    isLoading,
    error: membersError,
    refetch,
  } = useConsoleQuery<{ members: MemberRow[] }>("/team");
  const members: MemberRow[] = data?.members ?? [];

  // The admin's own spendable balance, shown in the allocate dialog so the
  // amount entered can never exceed what they can actually transfer.
  const {
    data: walletData,
    isError: walletError,
    refetch: walletRefetch,
  } = useConsoleQuery<{ available: string }>("/wallet");
  const walletLoaded = !!walletData;
  const adminAvailable = walletData?.available ?? "0";

  // --- Mutations ---
  const queryClient = useQueryClient();
  const createMutation = useConsoleMutation<unknown, CreateSubAccountVars>(
    "post", "/team/members", "/team",
  );
  const allocateMutation = useConsoleMutation<unknown, AllocateBalanceVars>(
    "post", "/team/balance/allocate", "/team",
    {
      // The transfer also moves money out of the admin's own wallet, so the
      // wallet view needs a refresh alongside the member list.
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: consoleKey("/wallet") });
      },
    },
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

  // --- Allocate balance dialog ---
  // The backend transfers the amount from the admin's wallet to the member's
  // wallet; the entered value must be a positive decimal with at most two
  // fractional digits and cannot exceed the admin's own spendable balance.
  const [allocMember, setAllocMember] = useState<MemberRow | null>(null);
  const [allocAmount, setAllocAmount] = useState("");
  const allocAmountValid = isValidAmount(allocAmount);
  const allocExceeds =
    walletLoaded &&
    allocAmountValid &&
    toCents(allocAmount) > toCents(adminAvailable);
  // The ceiling is only meaningful once the wallet read has succeeded. Without
  // it (still loading, or errored) the confirm button stays disabled so the
  // client-side guard can never be silently bypassed.
  const allocValid =
    !!allocMember && walletLoaded && allocAmountValid && !allocExceeds;

  function openAllocate(m: MemberRow) {
    allocateMutation.reset();
    setAllocMember(m);
    setAllocAmount("");
  }
  async function handleAllocate() {
    if (!allocMember || !allocValid) return;
    try {
      await allocateMutation.mutateAsync({
        user_id: allocMember.id,
        amount: allocAmount,
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
      <LoadingState message="加载团队成员..." />
    );
  }

  if (membersError) {
    return (
      <Card className="border-destructive/30">
        <CardContent className="p-6 text-center">
          <p className="text-sm text-destructive mb-3">
            {membersError instanceof Error ? membersError.message : "加载失败"}
          </p>
          <Button variant="outline" onClick={() => refetch()}>重试</Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="font-display font-semibold">团队管理</h3>
          <p className="text-[13px] text-[#5C6472] mt-0.5">共 {members.length} 人 · 管理员可给成员分配余额</p>
        </div>
        <Button
          onClick={() => {
            createMutation.reset();
            setCreateForm({ email: "", display_name: "", password: "", role: "member" });
            setShowCreate(true);
          }}
        >
          <UserPlus size={16} className="mr-1.5" />添加子账号
        </Button>
      </div>

      {rowActionError && (
        <Card className="border-destructive/30">
          <CardContent className="p-3 text-sm text-destructive">
            {rowActionError instanceof Error ? rowActionError.message : String(rowActionError)}
          </CardContent>
        </Card>
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
                <TableHead>余额</TableHead>
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
              {filtered.map(m => (
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
                    {m.balance ? (
                      <span className="text-sm tabular-nums">¥{fmtMoney(m.balance)}</span>
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex gap-1 justify-end">
                      {m.role === "owner" ? (
                        <span className="text-xs text-muted-foreground self-center">所有者不可操作</span>
                      ) : (
                        <>
                          <Button variant="outline" size="sm" onClick={() => openAllocate(m)}>
                            <Coins size={12} className="mr-1" />分配余额
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
              ))}
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

        {/* Allocate balance dialog */}
        <Dialog open={allocMember !== null} onOpenChange={() => setAllocMember(null)}>
          <DialogContent>
            <DialogHeader><DialogTitle>分配余额</DialogTitle></DialogHeader>
            {allocMember && (
              <div className="space-y-4">
                <div className="p-3 bg-muted rounded-lg">
                  <p className="font-medium">{allocMember.name}</p>
                  <p className="text-xs text-muted-foreground">{allocMember.email}</p>
                </div>

                <div className="p-3 bg-muted rounded-lg text-sm">
                  <p className="text-xs text-muted-foreground">您的可用余额</p>
                  {walletLoaded ? (
                    <p className="font-medium tabular-nums">¥{fmtMoney(adminAvailable)}</p>
                  ) : walletError ? (
                    <div className="space-y-1">
                      <p className="text-xs text-destructive">可用余额加载失败</p>
                      <Button variant="outline" size="sm" onClick={() => walletRefetch()}>
                        重试
                      </Button>
                    </div>
                  ) : (
                    <p className="font-medium tabular-nums">加载中...</p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label>转账金额（CNY）*</Label>
                  <Input
                    value={allocAmount}
                    onChange={e => setAllocAmount(toMoneyInput(e.target.value))}
                    type="text"
                    inputMode="decimal"
                    placeholder="例如 10.00"
                  />
                  {allocAmount && !allocValid && (
                    <p className="text-xs text-destructive">
                      {allocExceeds ? "超出您的可用余额" : "请输入有效金额（最多两位小数）"}
                    </p>
                  )}
                </div>

                <p className="text-xs text-muted-foreground">
                  金额将直接从您的余额转入 {allocMember.name} 的账户。
                </p>

                {allocateMutation.isError && (
                  <p className="text-sm text-destructive">
                    {allocateMutation.error instanceof Error
                      ? allocateMutation.error.message
                      : "分配失败"}
                  </p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setAllocMember(null)}>取消</Button>
              <Button onClick={handleAllocate} disabled={!allocValid || allocateMutation.isPending}>
                {allocateMutation.isPending ? "分配中..." : "确认分配"}
              </Button>
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
    </div>
  );
}
