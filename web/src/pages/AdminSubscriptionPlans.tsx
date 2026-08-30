import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Crown, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { useAdminMutation, useAdminQuery } from "@/lib/hooks/use-api";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface AdminPlan {
  id: string;
  name: string;
  description: string;
  price: string;
  duration_days: number;
  group_name: string;
  token_quota: number;
  sort_order: number;
  enabled: boolean;
}

interface PlanForm {
  name: string;
  description: string;
  price: string;
  duration_days: string;
  group_name: string;
  token_quota: string;
  sort_order: string;
}

const emptyForm: PlanForm = { name: "", description: "", price: "", duration_days: "30", group_name: "", token_quota: "0", sort_order: "0" };

type PlanPayload = {
  name: string;
  description: string;
  price: string;
  duration_days: number;
  group_name: string;
  token_quota: number;
  sort_order: number;
};

function toNumericPlan(form: PlanForm): PlanPayload {
  return {
    name: form.name,
    description: form.description,
    price: form.price,
    duration_days: Number(form.duration_days),
    group_name: form.group_name,
    token_quota: Number(form.token_quota),
    sort_order: Number(form.sort_order),
  };
}

export default function AdminSubscriptionPlans() {
  const { t } = useTranslation();
  const listQuery = useAdminQuery<{ data: AdminPlan[] }>("/subscription-plans");
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<AdminPlan | null>(null);
  const [form, setForm] = useState<PlanForm>(emptyForm);

  const invalidate = "/subscription-plans";
  type PlanUpdate = PlanPayload & { id: string; enabled?: boolean };
  const save = useAdminMutation<{ ok: boolean }, PlanPayload>(
    "post",
    "/subscription-plans",
    invalidate,
    {
      onSuccess: () => {
        toast.success(t("adminplans.savedPlan"));
        setOpen(false);
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : t("adminplans.saveFailed")),
    },
  );

  const update = useAdminMutation<{ ok: boolean }, PlanUpdate>(
    "put",
    (v) => `/subscription-plans/${v.id}`,
    invalidate,
    { onSuccess: () => toast.success(t("adminplans.updatedPlan")), onError: (e) => toast.error(e instanceof Error ? e.message : t("adminplans.updateFailed")) },
  );
  const remove = useAdminMutation<{ ok: boolean }, { id: string }>(
    "delete",
    (v) => `/subscription-plans/${v.id}`,
    invalidate,
    { onSuccess: () => toast.success(t("adminplans.deletedPlan")), onError: (e) => toast.error(e instanceof Error ? e.message : t("adminplans.deleteFailed")) },
  );

  const plans = listQuery.data?.data ?? [];

  const openCreate = () => {
    setEditing(null);
    setForm(emptyForm);
    setOpen(true);
  };

  const openEdit = (p: AdminPlan) => {
    setEditing(p);
    setForm({
      name: p.name,
      description: p.description,
      price: p.price,
      duration_days: String(p.duration_days),
      group_name: p.group_name ?? "",
      token_quota: String(p.token_quota ?? 0),
      sort_order: String(p.sort_order),
    });
    setOpen(true);
  };

  const handleSave = () => {
    if (!form.name.trim() || !form.price || Number(form.price) <= 0 || Number(form.duration_days) <= 0) {
      toast.error(t("adminplans.invalidPlan"));
      return;
    }
    if (editing) {
      update.mutate({ ...toNumericPlan(form), id: editing.id });
    } else {
      save.mutate(toNumericPlan(form));
    }
  };

  const set = (key: keyof PlanForm) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((p) => ({ ...p, [key]: e.target.value }));

  return (
    <div>
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("adminplans.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("adminplans.subtitle")}</p>
        </div>
        <Button onClick={openCreate}>
          <Plus size={15} className="mr-1.5" /> {t("adminplans.newPlan")}
        </Button>
      </div>

      {listQuery.isLoading ? (
        <LoadingState message={t("adminplans.loading")} />
      ) : listQuery.isError ? (
        <ErrorState error={listQuery.error} onRetry={() => listQuery.refetch()} title={t("adminplans.loadFailed")} />
      ) : plans.length === 0 ? (
        <EmptyState icon={Crown} title={t("adminplans.empty")} description={t("adminplans.emptyDesc")} />
      ) : (
        <div className="glass rounded-[22px] p-4 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("adminplans.thName")}</TableHead>
                <TableHead>{t("adminplans.thPrice")}</TableHead>
                <TableHead>{t("adminplans.thDuration")}</TableHead>
                <TableHead>{t("adminplans.thGroup")}</TableHead>
                <TableHead>{t("adminplans.thQuota")}</TableHead>
                <TableHead>{t("adminplans.thSort")}</TableHead>
                <TableHead>{t("adminplans.thEnabled")}</TableHead>
                <TableHead className="text-right">{t("adminplans.thActions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {plans.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">{p.name}</TableCell>
                  <TableCell>¥{p.price}</TableCell>
                  <TableCell className="text-[#5C6472]">{t("adminplans.days", { n: p.duration_days })}</TableCell>
                  <TableCell className="text-[#5C6472]">{p.group_name || "—"}</TableCell>
                  <TableCell className="text-[#5C6472]">{p.token_quota ? t("adminplans.quotaValue", { n: (p.token_quota / 10000).toFixed(0) }) : "—"}</TableCell>
                  <TableCell className="text-[#5C6472]">{p.sort_order}</TableCell>
                  <TableCell>
                    <Switch
                      checked={p.enabled}
                      onCheckedChange={(v) => update.mutate({ ...toNumericPlan(formFromPlan(p)), id: p.id, enabled: v })}
                    />
                  </TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => openEdit(p)}>
                      <Pencil size={13} className="mr-1" /> {t("adminplans.edit")}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => remove.mutate({ id: p.id })}>
                      <Trash2 size={13} className="mr-1 text-[#C4372C]" /> {t("adminplans.delete")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t("adminplans.editTitle") : t("adminplans.createTitle")}</DialogTitle>
            <DialogDescription>{t("adminplans.dialogDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="plan-name">{t("adminplans.name")}</Label>
              <Input id="plan-name" value={form.name} onChange={set("name")} placeholder={t("adminplans.namePlaceholder")} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="plan-desc">{t("adminplans.desc")}</Label>
              <Input id="plan-desc" value={form.description} onChange={set("description")} placeholder={t("adminplans.descPlaceholder")} />
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="plan-price">{t("adminplans.price")}</Label>
                <Input id="plan-price" type="number" min="0.01" step="0.01" value={form.price} onChange={set("price")} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="plan-days">{t("adminplans.duration")}</Label>
                <Input id="plan-days" type="number" min="1" value={form.duration_days} onChange={set("duration_days")} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="plan-order">{t("adminplans.sort")}</Label>
                <Input id="plan-order" type="number" value={form.sort_order} onChange={set("sort_order")} />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="plan-group">{t("adminplans.group")}</Label>
              <Input
                id="plan-group"
                value={form.group_name}
                onChange={set("group_name")}
                placeholder={t("adminplans.groupPlaceholder")}
              />
              <p className="text-xs text-[#5C6472]">{t("adminplans.groupHint")}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="plan-quota">{t("adminplans.quota")}</Label>
              <Input
                id="plan-quota"
                type="number"
                min="0"
                value={form.token_quota}
                onChange={set("token_quota")}
                placeholder={t("adminplans.quotaPlaceholder")}
              />
              <p className="text-xs text-[#5C6472]">{t("adminplans.quotaHint")}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={save.isPending || update.isPending}>
              {t("adminplans.cancel")}
            </Button>
            <Button onClick={handleSave} disabled={save.isPending || update.isPending}>
              {save.isPending || update.isPending ? t("adminplans.saving") : t("adminplans.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function formFromPlan(p: AdminPlan) {
  return {
    name: p.name,
    description: p.description,
    price: p.price,
    duration_days: String(p.duration_days),
    group_name: p.group_name ?? "",
    token_quota: String(p.token_quota ?? 0),
    sort_order: String(p.sort_order),
  };
}
