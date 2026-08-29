import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import { ProviderModelPreview } from "@/lib/api";
import "../i18n";
import { useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  providerId: string;
  onClose: () => void;
  onSynced: () => void;
}

export function ProviderSyncDialog({ open, providerId, onClose, onSynced }: Props) {
  const { t } = useTranslation();
  const preview = useAdminQuery<{ models: ProviderModelPreview[] }>(`/channels/${providerId}/models/preview`, {
    enabled: open && !!providerId,
  });
  const sync = useAdminMutation<{ created: number }, void>(
    "post",
    () => `/channels/${providerId}/models/sync`,
    "/providers",
    { onSuccess: () => onSynced() },
  );
  const [selected, setSelected] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (open && preview.data) {
      const next: Record<string, boolean> = {};
      for (const m of preview.data.models) next[m.id] = true;
      setSelected(next);
    }
  }, [open, preview.data]);

  const models = preview.data?.models ?? [];
  const newCount = models.filter((m) => !m.exists).length;

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
            models.map((m) => (
              <label key={m.id} className="flex items-center justify-between gap-3 px-4 py-2.5 border-b border-black/5 last:border-0 hover:bg-white/40">
                <span className="flex items-center gap-3">
                  <input
                    type="checkbox"
                    checked={!!selected[m.id]}
                    onChange={(e) => setSelected((p) => ({ ...p, [m.id]: e.target.checked }))}
                    className="accent-[#4F6BED]"
                  />
                  <code className="font-mono text-sm">{m.id}</code>
                </span>
                <Badge variant={m.exists ? "secondary" : "success"}>{m.exists ? t("components.syncExists") : t("components.syncNew")}</Badge>
              </label>
            ))
          )}
        </div>

        <div className="flex items-center justify-between">
          <p className="text-xs text-[#5C6472]">
            {t("components.syncNewCount", { count: newCount })}
          </p>
          <Button onClick={() => sync.mutate()} disabled={sync.isPending || preview.isLoading}>
            {sync.isPending ? t("components.syncSyncing") : t("components.syncApply")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
