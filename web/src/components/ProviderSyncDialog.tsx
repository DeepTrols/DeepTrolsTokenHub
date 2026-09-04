import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import "../i18n";
import { useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  providerId: string;
  onClose: () => void;
  onSynced: () => void;
}

interface PreviewModel {
  upstream: string;
  code: string;
  model_id: string;
  status: "new" | "bound" | "disabled";
  enabled: boolean;
}

interface SyncResult {
  applied: number;
  created: number;
  skipped: number;
}

export function ProviderSyncDialog({ open, providerId, onClose, onSynced }: Props) {
  const { t } = useTranslation();
  const preview = useAdminQuery<{ models: PreviewModel[] }>(`/channels/${providerId}/models/preview`, {
    enabled: open && !!providerId,
  });
  const sync = useAdminMutation<SyncResult, { model_ids: string[]; auto_create: boolean }>(
    "post",
    () => `/channels/${providerId}/models/sync`,
    `/channels/${providerId}/models`,
    {
      onSuccess: (r) => {
        toast.success(t("components.syncDone", { applied: r.applied, created: r.created, skipped: r.skipped }));
        onSynced();
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : t("components.syncFailed")),
    },
  );
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [autoCreate, setAutoCreate] = useState(false);

  useEffect(() => {
    if (open && preview.data) {
      const next: Record<string, boolean> = {};
      for (const m of preview.data.models) {
        next[m.upstream] = m.status === "new" || m.enabled;
      }
      setSelected(next);
    }
  }, [open, preview.data]);

  const models = preview.data?.models ?? [];
  const newModels = models.filter((m) => m.status === "new");
  const boundModels = models.filter((m) => m.status === "bound");
  const disabledModels = models.filter((m) => m.status === "disabled");
  const selectedIds = models.filter((m) => selected[m.upstream]).map((m) => m.upstream);

  const statusLabel = (status: PreviewModel["status"]) => {
    if (status === "new") return t("components.syncNew");
    if (status === "bound") return t("components.syncBound");
    return t("components.syncDisabled");
  };

  const renderGroup = (group: PreviewModel[], title: string) => {
    if (group.length === 0) return null;
    return (
      <div>
        <p className="px-4 pt-3 pb-1 text-xs font-medium text-[#5C6472]">{title}</p>
        {group.map((m) => (
          <label key={m.upstream} className="flex items-center justify-between gap-3 px-4 py-2.5 border-b border-black/5 last:border-0 hover:bg-white/40">
            <span className="flex items-center gap-3">
              <input
                type="checkbox"
                checked={!!selected[m.upstream]}
                onChange={(e) => setSelected((p) => ({ ...p, [m.upstream]: e.target.checked }))}
                className="accent-[#F78B28]"
              />
              <code className="font-mono text-sm">{m.code || m.upstream}</code>
            </span>
            <Badge variant={m.status === "new" ? "success" : "secondary"}>{statusLabel(m.status)}</Badge>
          </label>
        ))}
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>{t("components.syncTitle")}</DialogTitle>
          <DialogDescription>{t("components.syncDesc")}</DialogDescription>
        </DialogHeader>

        <div className="max-h-[50vh] overflow-y-auto border rounded-xl">
          {preview.isLoading ? (
            <p className="p-4 text-sm text-[#5C6472]">{t("components.syncDiscovering")}</p>
          ) : preview.isError ? (
            <p className="p-4 text-sm text-destructive">{t("components.syncDiscoverFailed")}</p>
          ) : models.length === 0 ? (
            <p className="p-4 text-sm text-[#5C6472]">{t("components.syncNoneFound")}</p>
          ) : (
            <>
              {renderGroup(newModels, t("components.syncGroupNew", { count: newModels.length }))}
              {renderGroup(boundModels, t("components.syncGroupBound", { count: boundModels.length }))}
              {renderGroup(disabledModels, t("components.syncGroupDisabled", { count: disabledModels.length }))}
            </>
          )}
        </div>

        <label className="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={autoCreate}
            onChange={(e) => setAutoCreate(e.target.checked)}
            className="mt-0.5 rounded accent-[#F78B28]"
          />
          <span>
            <span className="block text-sm font-medium">{t("components.syncAutoCreate")}</span>
            <span className="block text-xs text-[#5C6472]">{t("components.syncAutoCreateHint")}</span>
          </span>
        </label>

        <div className="flex items-center justify-between">
          <p className="text-xs text-[#5C6472]">
            {t("components.syncNewCount", { count: newModels.length })}
          </p>
          <Button
            onClick={() => sync.mutate({ model_ids: selectedIds, auto_create: autoCreate })}
            disabled={sync.isPending || preview.isLoading || selectedIds.length === 0}
          >
            {sync.isPending ? t("components.syncSyncing") : t("components.syncApply")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
