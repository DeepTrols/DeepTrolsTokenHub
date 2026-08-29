import React, { useEffect, useState } from "react";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

type SiteSettings = Record<string, string>;

export default function RequestLimitsSettingsSection() {
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

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.limitsTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.limitsSubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.rateLimit")}</CardTitle>
          <CardDescription>{t("settings.rateLimitDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="gateway_rate_limit_rpm">{t("settings.gatewayRpm")}</Label>
            <Input id="gateway_rate_limit_rpm" value={form.gateway_rate_limit_rpm ?? ""} onChange={set("gateway_rate_limit_rpm")} />
          </div>
          <Button
            onClick={() => save.mutate({ gateway_rate_limit_rpm: form.gateway_rate_limit_rpm ?? "" })}
            disabled={save.isPending}
          >
            {t("common.save")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
