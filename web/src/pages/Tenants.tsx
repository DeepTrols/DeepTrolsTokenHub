import { useMemo, useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Plus,
  Search,
  X,
  CheckCircle2,
  Pause,
  Play,
  Trash2,
  Building2,
} from "lucide-react";

interface TenantData {
  id: string;
  code: string;
  name: string;
  status: string;
  status_reason?: string;
  owner_id?: string;
  created_at: string;
}

const STATUS_META: Record<string, { label: string; variant: "success" | "destructive" | "secondary" | "outline" }> = {
  pending_review: { label: "待审核", variant: "secondary" },
  active: { label: "已激活", variant: "success" },
  suspended: { label: "已停用", variant: "destructive" },
  terminated: { label: "已终止", variant: "outline" },
  rejected: { label: "已拒绝", variant: "outline" },
};

type LifecycleAction = "approve" | "reject" | "suspend" | "reactivate" | "terminate";

const ACTION_LABEL: Record<LifecycleAction, string> = {
  approve: "审核通过",
  reject: "拒绝",
  suspend: "停用",
  reactivate: "启用",
  terminate: "终止",
};

export default function Tenants() {
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
  } = useAdminQuery<{ data: TenantData[]; total: number }>("/tenants");
  const tenants = data?.data ?? [];
  const total = data?.total ?? 0;

  const [q, setQ] = useState("");
  const filtered = useMemo(() => {
    if (!q.trim()) return tenants;
    const lq = q.toLowerCase();
    return tenants.filter(
      (t) => t.name.toLowerCase().includes(lq) || t.code.toLowerCase().includes(lq),
    );
  }, [tenants, q]);

  // 生命周期变更：状态更新走 PUT，终止走 DELETE（服务端置为 terminated）。
  const uM = useAdminMutation<unknown, { id: string; status: string; status_reason?: string }>(
    "put",
    (v) => "/tenants/" + v.id,
    "/tenants",
  );
  const dM = useAdminMutation<unknown, { id: string }>(
    "delete",
    (v) => "/tenants/" + v.id,
    "/tenants",
  );

  const [pendingAction, setPendingAction] = useState<{ tenant: TenantData; action: LifecycleAction } | null>(null);
  const [reason, setReason] = useState("");

  const runTransition = async (t: TenantData, action: LifecycleAction, r?: string) => {
    if (action === "terminate") {
      await dM.mutateAsync({ id: t.id });
      return;
    }
    const next: Record<Exclude<LifecycleAction, "terminate">, string> = {
      approve: "active",
      reject: "rejected",
      suspend: "suspended",
      reactivate: "active",
    };
    await uM.mutateAsync({ id: t.id, status: next[action], status_reason: r ?? "" });
  };

  const requestAction = (t: TenantData, action: LifecycleAction) => {
    // 审核通过 / 启用 无需原因，直接执行。
    if (action === "approve" || action === "reactivate") {
      runTransition(t, action).catch(() => {});
      return;
    }
    setPendingAction({ tenant: t, action });
    setReason("");
  };

  const confirmAction = async () => {
    if (!pendingAction) return;
    const { tenant, action } = pendingAction;
    setPendingAction(null);
    try {
      await runTransition(tenant, action, reason.trim() || undefined);
    } catch {
      /* 错误由 refetch 后的列表状态反映 */
    }
  };

  // 创建企业（可同步预配负责人账号，避免新企业无人可用）
  const [createOpen, setCreateOpen] = useState(false);
  const [nm, setNm] = useState("");
  const [cd, setCd] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [ownerPassword, setOwnerPassword] = useState("");
  const cM = useAdminMutation<unknown, { name: string; code: string; owner_email?: string; owner_password?: string }>(
    "post",
    "/tenants",
    "/tenants",
  );
  const create = async () => {
    if (!nm.trim() || !cd.trim()) return;
    try {
      const payload: { name: string; code: string; owner_email?: string; owner_password?: string } = {
        name: nm.trim(),
        code: cd.trim(),
      };
      // 负责人邮箱/密码成对提供时才预配 owner；仅填其一则交由服务端 400。
      if (ownerEmail.trim()) {
        payload.owner_email = ownerEmail.trim();
        payload.owner_password = ownerPassword;
      }
      await cM.mutateAsync(payload);
      setCreateOpen(false);
      setNm("");
      setCd("");
      setOwnerEmail("");
      setOwnerPassword("");
    } catch {
      /* 冲突/校验错误由列表状态反映 */
    }
  };

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>企业管理</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <LoadingState message="加载企业..." />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>企业管理</SectionPageLayout.Title>
          <SectionPageLayout.Description>共 {total} 家企业</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
        <SectionPageLayout.Actions>
          <Button
            onClick={() => {
              setNm("");
              setCd("");
              setOwnerEmail("");
              setOwnerPassword("");
              setCreateOpen(true);
            }}
          >
            <Plus size={16} className="mr-1.5" />
            创建企业
          </Button>
        </SectionPageLayout.Actions>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        <div className="mb-4 flex items-center gap-2">
          <div className="relative max-w-sm flex-1">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索企业名称 / 编码"
              className="pl-9 h-9 text-sm"
            />
            {q && (
              <Button
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                onClick={() => setQ("")}
              >
                <X size={14} />
              </Button>
            )}
          </div>
        </div>

        {isError && <ErrorState error={error} onRetry={() => refetch()} />}

        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>企业</TableHead>
                <TableHead>编码</TableHead>
                <TableHead>企业 ID</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <EmptyState icon={Building2} title={q ? "未找到" : "暂无企业"} />
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((t) => {
                const meta = STATUS_META[t.status] ?? { label: t.status, variant: "secondary" as const };
                return (
                  <TableRow key={t.id}>
                    <TableCell>
                      <p className="font-medium text-sm">{t.name}</p>
                    </TableCell>
                    <TableCell>
                      <code className="text-xs">{t.code}</code>
                    </TableCell>
                    <TableCell>
                      <code
                        className="text-xs text-muted-foreground"
                        title={t.id}
                      >
                        {t.id.slice(0, 8)}
                      </code>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1 items-start">
                        <Badge variant={meta.variant}>{meta.label}</Badge>
                        {t.status_reason && (
                          <span className="text-xs text-muted-foreground">{t.status_reason}</span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {new Date(t.created_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex gap-1 justify-end flex-wrap">
                        {t.status === "pending_review" && (
                          <>
                            <Button variant="default" size="sm" onClick={() => requestAction(t, "approve")}>
                              <CheckCircle2 size={14} className="mr-1" />
                              审核通过
                            </Button>
                            <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "reject")}>
                              拒绝
                            </Button>
                          </>
                        )}
                        {t.status === "active" && (
                          <>
                            <Button variant="outline" size="sm" onClick={() => requestAction(t, "suspend")}>
                              <Pause size={14} className="mr-1" />
                              停用
                            </Button>
                            <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "terminate")}>
                              <Trash2 size={14} className="mr-1" />
                              终止
                            </Button>
                          </>
                        )}
                        {t.status === "suspended" && (
                          <>
                            <Button variant="outline" size="sm" onClick={() => requestAction(t, "reactivate")}>
                              <Play size={14} className="mr-1" />
                              启用
                            </Button>
                            <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "terminate")}>
                              <Trash2 size={14} className="mr-1" />
                              终止
                            </Button>
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
      </SectionPageLayout.Content>

      {/* 状态变更确认（拒绝 / 停用 / 终止） */}
      <Dialog open={pendingAction !== null} onOpenChange={() => setPendingAction(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {pendingAction ? `${ACTION_LABEL[pendingAction.action]}：${pendingAction.tenant.name}` : ""}
            </DialogTitle>
          </DialogHeader>
          {pendingAction && (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>原因（可选）</Label>
                <Input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder={pendingAction.action === "terminate" ? "填写终止原因" : "填写操作原因"}
                />
              </div>
              <p className="text-sm text-muted-foreground">
                {pendingAction.action === "terminate"
                  ? "终止后该企业将无法继续使用平台服务，此操作不可撤销。"
                  : "确认后企业状态将更新。"}
              </p>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingAction(null)}>
              取消
            </Button>
            <Button
              variant={
                pendingAction?.action === "reject" || pendingAction?.action === "terminate"
                  ? "destructive"
                  : "default"
              }
              onClick={confirmAction}
            >
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 创建企业 */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建企业</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>企业名称 *</Label>
              <Input value={nm} onChange={(e) => setNm(e.target.value)} placeholder="例：某某科技有限公司" />
            </div>
            <div className="space-y-2">
              <Label>企业编码 *</Label>
              <Input value={cd} onChange={(e) => setCd(e.target.value)} placeholder="唯一编码，如 acme-corp" />
            </div>
            <div className="space-y-2">
              <Label>负责人邮箱（可选）</Label>
              <Input
                value={ownerEmail}
                onChange={(e) => setOwnerEmail(e.target.value)}
                placeholder="ceo@example.com"
              />
            </div>
            <div className="space-y-2">
              <Label>初始密码（可选）</Label>
              <Input
                type="password"
                value={ownerPassword}
                onChange={(e) => setOwnerPassword(e.target.value)}
                placeholder="至少 8 位，与邮箱成对填写"
              />
            </div>
            <p className="text-xs text-muted-foreground">
              新企业创建后为待审核状态，需审核通过后方可使用。填写负责人邮箱+初始密码后，将同时创建该负责人账号并设为企业 owner。
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button onClick={create} disabled={!nm.trim() || !cd.trim() || cM.isPending}>
              {cM.isPending ? "创建中..." : "创建"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionPageLayout>
  );
}
