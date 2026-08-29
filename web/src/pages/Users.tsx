import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState, useMemo } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Search, Edit, Trash2, Users as UsersIcon, X, Plus } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import "../i18n";
import { useTranslation } from "react-i18next";

interface UserRow {
  id: string;
  email: string;
  display_name: string;
  role: string;
  user_type: string;
  status: string;
  created_at: string;
}

const QUERY = "/users?user_type=personal";

function statusVariant(s: string): "success" | "destructive" | "secondary" {
  if (s === "active") return "success";
  if (s === "banned") return "destructive";
  return "secondary";
}
function statusLabel(s: string): string {
  if (s === "active") return "users.statusActive";
  if (s === "banned") return "users.statusBanned";
  return s;
}

export default function Users() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: UserRow[]; total: number }>(QUERY);
  const all = useMemo(() => data?.data ?? [], [data]);
  const total = data?.total ?? 0;

  const [q, setQ] = useState("");
  const users = useMemo(() => {
    if (!q.trim()) return all;
    const lq = q.toLowerCase();
    return all.filter(
      (u) =>
        u.email.toLowerCase().includes(lq) ||
        (u.display_name || "").toLowerCase().includes(lq),
    );
  }, [all, q]);

  // Edit dialog（角色 / 状态）
  const [ed, setEd] = useState<UserRow | null>(null);
  const [er, setEr] = useState("");
  const [es, setEs] = useState("");
  const rm = useAdminMutation<unknown, { id: string; role: string }>("put", (v) => "/users/" + v.id + "/role", QUERY);
  const sm = useAdminMutation<unknown, { id: string; status: string }>("put", (v) => "/users/" + v.id + "/status", QUERY);
  const dm = useAdminMutation<unknown, { id: string }>("delete", (v) => "/users/" + v.id, QUERY);

  const openEdit = (u: UserRow) => {
    setEd(u);
    setEr(u.role);
    setEs(u.status);
  };
  const saveEdit = async () => {
    if (!ed) return;
    try {
      if (er !== ed.role) await rm.mutateAsync({ id: ed.id, role: er });
      if (es !== ed.status) await sm.mutateAsync({ id: ed.id, status: es });
      setEd(null);
    } catch {
      /* 错误由 refetch 后的列表状态反映 */
    }
  };
  const handleDelete = async (u: UserRow) => {
    if (!window.confirm(t("users.deleteConfirm", { email: u.email }))) return;
    try {
      await dm.mutateAsync({ id: u.id });
    } catch {
      /* 错误由列表状态反映 */
    }
  };

  // Create dialog
  const [sc, setSc] = useState(false);
  const [nm, setNm] = useState("");
  const [em, setEm] = useState("");
  const [pw, setPw] = useState("");
  const [cr, setCr] = useState("user");
  const cm = useAdminMutation<unknown, { email: string; password: string; display_name: string; role: string }>("post", "/users", QUERY);
  const handleCreate = async () => {
    if (!em.trim() || !pw.trim()) return;
    try {
      await cm.mutateAsync({ email: em.trim(), password: pw.trim(), display_name: nm.trim() || em.trim(), role: cr });
      setSc(false);
      setNm("");
      setEm("");
      setPw("");
      setCr("user");
    } catch {
      /* 错误由列表状态反映 */
    }
  };

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>{t("users.title")}</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <LoadingState message={t("users.loading")} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>{t("users.title")}</SectionPageLayout.Title>
          <SectionPageLayout.Description>{t("users.totalCount", { count: total })}</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
        <SectionPageLayout.Actions>
          <Button
            onClick={() => {
              setNm("");
              setEm("");
              setPw("");
              setCr("user");
              setSc(true);
            }}
          >
            <Plus size={16} className="mr-1.5" />
            {t("users.createUser")}
          </Button>
        </SectionPageLayout.Actions>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        <div className="mb-4 flex items-center gap-2">
          <div className="relative max-w-sm flex-1">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("users.searchPlaceholder")} className="pl-9 h-9 text-sm" />
            {q && (
              <Button variant="ghost" size="icon" className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7" onClick={() => setQ("")}>
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
                <TableHead>{t("users.thUser")}</TableHead>
                <TableHead>{t("users.thRole")}</TableHead>
                <TableHead>{t("users.thStatus")}</TableHead>
                <TableHead>{t("users.thCreated")}</TableHead>
                <TableHead className="text-right">{t("users.thActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5}>
                    <EmptyState icon={UsersIcon} title={q ? t("users.notFound") : t("users.empty")} />
                  </TableCell>
                </TableRow>
              )}
              {users.map((u) => (
                <TableRow key={u.id}>
                  <TableCell>
                    <p className="font-medium text-sm">{u.email}</p>
                    {u.display_name && <p className="text-xs text-muted-foreground">{u.display_name}</p>}
                  </TableCell>
                  <TableCell>
                    <Badge variant={u.role === "admin" ? "default" : "secondary"}>
                      {u.role === "admin" ? t("users.roleAdmin") : t("users.roleUser")}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant={statusVariant(u.status)}>{t(statusLabel(u.status))}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {u.created_at ? new Date(u.created_at).toLocaleDateString() : "—"}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex gap-1 justify-end">
                      <Button variant="outline" size="sm" onClick={() => openEdit(u)}>
                        <Edit size={12} className="mr-1" />
                        {t("users.edit")}
                      </Button>
                      <Button variant="outline" size="sm" className="hover:text-destructive" onClick={() => handleDelete(u)}>
                        <Trash2 size={12} />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>

        {/* Edit Dialog */}
        <Dialog open={ed !== null} onOpenChange={() => setEd(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("users.editTitle")}</DialogTitle>
            </DialogHeader>
            {ed && (
              <div className="space-y-4">
                <div className="p-3 bg-muted rounded-lg">
                  <p className="font-medium">{ed.email}</p>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t("users.role")}</label>
                  <div className="flex gap-2">
                    {[{ v: "user", l: t("users.roleUser") }, { v: "admin", l: t("users.roleAdmin") }].map((o) => (
                      <Button key={o.v} variant={er === o.v ? "default" : "outline"} size="sm" onClick={() => setEr(o.v)}>
                        {o.l}
                      </Button>
                    ))}
                  </div>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t("users.status")}</label>
                  <div className="flex gap-2">
                    {[{ v: "active", l: t("users.enable") }, { v: "banned", l: t("users.ban") }].map((o) => (
                      <Button
                        key={o.v}
                        variant={es === o.v ? (o.v === "active" ? "default" : "destructive") : "outline"}
                        size="sm"
                        onClick={() => setEs(o.v)}
                      >
                        {o.l}
                      </Button>
                    ))}
                  </div>
                </div>
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setEd(null)}>
              {t("users.cancel")}
              </Button>
              <Button onClick={saveEdit}>{t("users.save")}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Create Dialog */}
        <Dialog open={sc} onOpenChange={setSc}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("users.createTitle")}</DialogTitle>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("users.email")}</Label>
                <Input value={em} onChange={(e) => setEm(e.target.value)} type="email" placeholder="user@example.com" />
              </div>
              <div className="space-y-2">
                <Label>{t("users.password")}</Label>
                <Input value={pw} onChange={(e) => setPw(e.target.value)} type="password" placeholder={t("users.passwordPlaceholder")} />
              </div>
              <div className="space-y-2">
                <Label>{t("users.displayName")}</Label>
                <Input value={nm} onChange={(e) => setNm(e.target.value)} placeholder={t("users.displayNamePlaceholder")} />
              </div>
              <div className="space-y-2">
                <Label>{t("users.role")}</Label>
                <div className="flex gap-2">
                  {[{ v: "user", l: t("users.roleUser") }, { v: "admin", l: t("users.roleAdmin") }].map((o) => (
                    <Button key={o.v} variant={cr === o.v ? "default" : "outline"} size="sm" onClick={() => setCr(o.v)}>
                      {o.l}
                    </Button>
                  ))}
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setSc(false)}>
              {t("users.cancel")}
              </Button>
              <Button onClick={handleCreate} disabled={cm.isPending}>
                {cm.isPending ? t("users.creating") : t("users.create")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
