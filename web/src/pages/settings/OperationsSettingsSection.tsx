import React, { useEffect, useState } from "react";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

type SiteSettings = Record<string, string>;

export default function OperationsSettingsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SiteSettings>("/settings/site");
  const [form, setForm] = useState<SiteSettings>({});

  useEffect(() => {
    if (data) {
      const next: SiteSettings = {};
      for (const [k, v] of Object.entries(data)) next[k] = typeof v === "string" ? v : JSON.stringify(v);
      setForm(next);
    }
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, SiteSettings>("put", "/settings/site", "/settings/site", {
    onSuccess: () => toast.success(t("common.saved")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
  });

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((p) => ({ ...p, [key]: e.target.value }));

  const field = (key: string, label: string, hint?: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={key}>{label}</Label>
      <Input id={key} value={form[key] ?? ""} onChange={set(key)} />
      {hint && <p className="text-xs text-[#5C6472]">{hint}</p>}
    </div>
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.opsTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.opsSubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.channelReconcile")}</CardTitle>
          <CardDescription>{t("settings.channelReconcileDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {field("channel_auto_disable_threshold", t("settings.autoDisable"), t("settings.autoDisableHint"))}
          {field("reconciliation_interval_hours", t("settings.reconcileInterval"))}
          <Button
            onClick={() => save.mutate({
              channel_auto_disable_threshold: form.channel_auto_disable_threshold ?? "",
              reconciliation_interval_hours: form.reconciliation_interval_hours ?? "",
            })}
            disabled={save.isPending}
          >
            {t("common.save")}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.checkin")}</CardTitle>
          <CardDescription>{t("settings.checkinDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label>{t("settings.enableCheckin")}</Label>
              <p className="text-xs text-[#5C6472]">{t("settings.checkinHint")}</p>
            </div>
            <Switch
              checked={form["checkin.enabled"] === "true"}
              onCheckedChange={(v) => setForm((p) => ({ ...p, "checkin.enabled": String(v) }))}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            {field("checkin.min_quota", t("settings.minReward"))}
            {field("checkin.max_quota", t("settings.maxReward"))}
          </div>
          <Button
            onClick={() =>
              save.mutate({
                "checkin.enabled": form["checkin.enabled"] === "true" ? "true" : "false",
                "checkin.min_quota": form["checkin.min_quota"] ?? "",
                "checkin.max_quota": form["checkin.max_quota"] ?? "",
              })
            }
            disabled={save.isPending}
          >
            {t("settings.saveCheckin")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
