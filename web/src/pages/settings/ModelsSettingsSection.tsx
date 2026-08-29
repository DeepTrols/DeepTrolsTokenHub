import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Box } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

type SiteSettings = Record<string, string>;

function strBool(v: unknown): boolean {
  if (typeof v === "boolean") return v;
  if (typeof v === "string") return v === "true";
  return false;
}

export default function ModelsSettingsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SiteSettings>("/settings/site");
  const [publicVisible, setPublicVisible] = useState(true);
  const [defaultStatus, setDefaultStatus] = useState("active");

  useEffect(() => {
    if (!data) return;
    setPublicVisible(strBool(data.models_public_visible));
    setDefaultStatus(data.new_model_default_status || "active");
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, SiteSettings>("put", "/settings/site", "/settings/site", {
    onSuccess: () => toast.success(t("settings.savedModels")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
  });

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.modelsTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.modelsSubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.catalogSync")}</CardTitle>
          <CardDescription>{t("settings.catalogSyncDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label>{t("settings.publicVisible")}</Label>
              <p className="text-xs text-[#5C6472]">{t("settings.publicVisibleHint")}</p>
            </div>
            <Switch checked={publicVisible} onCheckedChange={setPublicVisible} />
          </div>
          <div className="space-y-1.5 max-w-xs">
            <Label htmlFor="new-model-status">{t("settings.defaultStatus")}</Label>
            <Select value={defaultStatus} onValueChange={setDefaultStatus}>
              <SelectTrigger id="new-model-status"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="active">{t("settings.statusActive")}</SelectItem>
                <SelectItem value="draft">{t("settings.statusDraft")}</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-[#5C6472]">{t("settings.statusHint")}</p>
          </div>
          <Button
            onClick={() =>
              save.mutate({
                models_public_visible: String(publicVisible),
                new_model_default_status: defaultStatus,
              })
            }
            disabled={save.isPending}
          >
            <Box size={14} className="mr-1.5" />
            {save.isPending ? t("common.saving") : t("common.save")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
