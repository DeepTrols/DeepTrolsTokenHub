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
import "../i18n";
import { useTranslation } from "react-i18next";

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
  pending_review: { label: "tenants.statusPending", variant: "secondary" },
  active: { label: "tenants.statusActive", variant: "success" },
  suspended: { label: "tenants.statusSuspended", variant: "destructive" },
  terminated: { label: "tenants.statusTerminated", variant: "outline" },
  rejected: { label: "tenants.statusRejected", variant: "outline" },
};

type LifecycleAction = "approve" | "reject" | "suspend" | "reactivate" | "delete";

const ACTION_LABEL: Record<LifecycleAction, string> = {
  approve: "tenants.actionApprove",
  reject: "tenants.actionReject",
  suspend: "tenants.actionSuspend",
  reactivate: "tenants.actionReactivate",
  delete: "tenants.actionDelete",
};

export default function Tenants() {
  const { t: tr } = useTranslation();
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
  } = useAdminQuery<{ data: TenantData[]; total: number }>("/tenants");
  const tenants = useMemo(() => data?.data ?? [], [data]);
  const total = data?.total ?? 0;

  const [q, setQ] = useState("");
  const filtered = useMemo(() => {
    if (!q.trim()) return tenants;
    const lq = q.toLowerCase();
    return tenants.filter(
      (t) => t.name.toLowerCase().includes(lq) || t.code.toLowerCase().includes(lq),
    );
  }, [tenants, q]);

  // 生命周期变更：状态更新走 PUT，删除走 DELETE（服务端硬删除行与关联数据）。
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
    if (action === "delete") {
      await dM.mutateAsync({ id: t.id });
      return;
    }
    const next: Record<Exclude<LifecycleAction, "delete">, string> = {
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
      // 硬删除不接收原因；仅状态变更类操作带原因。
      const r = action === "delete" ? undefined : reason.trim() || undefined;
      await runTransition(tenant, action, r);
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
            <SectionPageLayout.Title>{tr("tenants.title")}</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <LoadingState message={tr("tenants.loading")} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>{tr("tenants.title")}</SectionPageLayout.Title>
          <SectionPageLayout.Description>{tr("tenants.totalCount", { count: total })}</SectionPageLayout.Description>
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
            {tr("tenants.createEnterprise")}
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
              placeholder={tr("tenants.searchPlaceholder")}
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
                <TableHead>{tr("tenants.thEnterprise")}</TableHead>
                <TableHead>{tr("tenants.thCode")}</TableHead>
                <TableHead>{tr("tenants.thId")}</TableHead>
                <TableHead>{tr("tenants.thStatus")}</TableHead>
                <TableHead>{tr("tenants.thCreated")}</TableHead>
                <TableHead className="text-right">{tr("tenants.thActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <EmptyState icon={Building2} title={q ? tr("tenants.notFound") : tr("tenants.empty")} />
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
                        <Badge variant={meta.variant}>{tr(meta.label)}</Badge>
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
                              {tr("tenants.actionApprove")}
                            </Button>
                            <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "reject")}>
                              {tr("tenants.actionReject")}
                            </Button>
                            <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "delete")}>
                              <Trash2 size={14} className="mr-1" />
                              {tr("tenants.actionDelete")}
                            </Button>
                          </>
                        )}
                        {t.status === "active" && (
                          <>
                            <Button variant="outline" size="sm" onClick={() => requestAction(t, "suspend")}>
                              <Pause size={14} className="mr-1" />
                              {tr("tenants.actionSuspend")}
                            </Button>
                            <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "delete")}>
                              <Trash2 size={14} className="mr-1" />
                              {tr("tenants.actionDelete")}
                            </Button>
                          </>
                        )}
                        {t.status === "suspended" && (
                          <>
                            <Button variant="outline" size="sm" onClick={() => requestAction(t, "reactivate")}>
                              <Play size={14} className="mr-1" />
                              {tr("tenants.actionReactivate")}
                            </Button>
                            <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "delete")}>
                              <Trash2 size={14} className="mr-1" />
                              {tr("tenants.actionDelete")}
                            </Button>
                          </>
                        )}
                        {(t.status === "terminated" || t.status === "rejected") && (
                          <Button variant="ghost" size="sm" className="hover:text-destructive" onClick={() => requestAction(t, "delete")}>
                            <Trash2 size={14} className="mr-1" />
                            {tr("tenants.actionDelete")}
                          </Button>
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

      {/* 操作确认（拒绝 / 停用 / 删除） */}
      <Dialog open={pendingAction !== null} onOpenChange={() => setPendingAction(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {pendingAction ? tr("tenants.confirmTitle", { action: tr(ACTION_LABEL[pendingAction.action]), name: pendingAction.tenant.name }) : ""}
            </DialogTitle>
          </DialogHeader>
          {pendingAction && (
            <div className="space-y-4">
              {pendingAction.action !== "delete" && (
                <div className="space-y-2">
                  <Label>{tr("tenants.reasonLabel")}</Label>
                  <Input
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder={tr("tenants.reasonPlaceholder")}
                  />
                </div>
              )}
              <p className="text-sm text-muted-foreground">
                {pendingAction.action === "delete"
                  ? tr("tenants.deleteWarning")
                  : tr("tenants.confirmUpdate")}
              </p>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingAction(null)}>
              {tr("tenants.cancel")}
            </Button>
            <Button
              variant={
                pendingAction?.action === "reject" || pendingAction?.action === "delete"
                  ? "destructive"
                  : "default"
              }
              onClick={confirmAction}
            >
              {pendingAction?.action === "delete" ? tr("tenants.confirmDelete") : tr("tenants.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 创建企业 */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{tr("tenants.createTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{tr("tenants.nameLabel")}</Label>
              <Input value={nm} onChange={(e) => setNm(e.target.value)} placeholder={tr("tenants.namePlaceholder")} />
            </div>
            <div className="space-y-2">
              <Label>{tr("tenants.codeLabel")}</Label>
              <Input value={cd} onChange={(e) => setCd(e.target.value)} placeholder={tr("tenants.codePlaceholder")} />
            </div>
            <div className="space-y-2">
              <Label>{tr("tenants.ownerEmail")}</Label>
              <Input
                value={ownerEmail}
                onChange={(e) => setOwnerEmail(e.target.value)}
                placeholder="ceo@example.com"
              />
            </div>
            <div className="space-y-2">
              <Label>{tr("tenants.ownerPassword")}</Label>
              <Input
                type="password"
                value={ownerPassword}
                onChange={(e) => setOwnerPassword(e.target.value)}
                placeholder={tr("tenants.ownerPasswordPlaceholder")}
              />
            </div>
            <p className="text-xs text-muted-foreground">{tr("tenants.createHint")}</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              {tr("tenants.cancel")}
            </Button>
            <Button onClick={create} disabled={!nm.trim() || !cd.trim() || cM.isPending}>
              {cM.isPending ? tr("tenants.creating") : tr("tenants.create")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionPageLayout>
  );
}
