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

function strBool(v: unknown): boolean {
  if (typeof v === "boolean") return v;
  if (typeof v === "string") return v === "true";
  return false;
}

export default function SecuritySettingsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SiteSettings>("/settings/site");
  const [registerEnabled, setRegisterEnabled] = useState(true);
  const [oauthEnabled, setOauthEnabled] = useState(false);
  const [oauthClientID, setOauthClientID] = useState("");
  const [oauthClientSecret, setOauthClientSecret] = useState("");
  const [googleEnabled, setGoogleEnabled] = useState(false);
  const [googleClientID, setGoogleClientID] = useState("");
  const [googleClientSecret, setGoogleClientSecret] = useState("");
  const [oauthRedirectBase, setOauthRedirectBase] = useState("");

  useEffect(() => {
    if (data) {
      setRegisterEnabled(strBool(data.register_enabled));
      setOauthEnabled(strBool(data.oauth_github_enabled));
      setOauthClientID(data.oauth_github_client_id ?? "");
      setOauthClientSecret(data.oauth_github_client_secret ?? "");
      setGoogleEnabled(strBool(data.oauth_google_enabled));
      setGoogleClientID(data.oauth_google_client_id ?? "");
      setGoogleClientSecret(data.oauth_google_client_secret ?? "");
      setOauthRedirectBase(data.oauth_redirect_base_url ?? "");
    }
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, SiteSettings>("put", "/settings/site", "/settings/site", {
    onSuccess: () => toast.success(t("common.saved")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
  });

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.securityTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.securitySubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.registerTitle")}</CardTitle>
          <CardDescription>{t("settings.registerDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-3">
            <input
              id="register_enabled"
              type="checkbox"
              checked={registerEnabled}
              onChange={(e) => setRegisterEnabled(e.target.checked)}
              className="accent-[#4F6BED]"
            />
            <Label htmlFor="register_enabled">{t("settings.openRegister")}</Label>
          </div>
          <Button
            onClick={() => save.mutate({ register_enabled: String(registerEnabled) })}
            disabled={save.isPending}
          >
            {t("common.save")}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.oauthGithub")}</CardTitle>
          <CardDescription>
            {t("settings.oauthGithubDesc")}
            {" "}
            <code className="text-[#4F6BED]">
              {(oauthRedirectBase || t("settings.callbackHint")) + "/api/oauth/github/callback"}
            </code>
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label>{t("settings.enableGithub")}</Label>
              <p className="text-xs text-[#5C6472]">{t("settings.oauthEntryHint")}</p>
            </div>
            <Switch checked={oauthEnabled} onCheckedChange={setOauthEnabled} />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="oauth_client_id">{t("settings.clientId")}</Label>
              <Input
                id="oauth_client_id"
                value={oauthClientID}
                onChange={(e) => setOauthClientID(e.target.value)}
                placeholder="Iv1.xxxxxxxx"
                className="font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="oauth_client_secret">{t("settings.clientSecret")}</Label>
              <Input
                id="oauth_client_secret"
                type="password"
                value={oauthClientSecret}
                onChange={(e) => setOauthClientSecret(e.target.value)}
                placeholder="GitHub OAuth App 密钥"
                className="font-mono"
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="oauth_redirect_base">{t("settings.redirectBase")}</Label>
            <Input
              id="oauth_redirect_base"
              value={oauthRedirectBase}
              onChange={(e) => setOauthRedirectBase(e.target.value)}
              placeholder="https://console.example.com（留空使用当前站点）"
            />
            <p className="text-xs text-[#5C6472]">{t("settings.redirectHint")}</p>
          </div>
          <Button
            onClick={() =>
              save.mutate({
                oauth_github_enabled: String(oauthEnabled),
                oauth_github_client_id: oauthClientID,
                oauth_github_client_secret: oauthClientSecret,
                oauth_redirect_base_url: oauthRedirectBase,
              })
            }
            disabled={save.isPending}
          >
            {t("settings.saveOAuth")}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.oauthGoogle")}</CardTitle>
          <CardDescription>
            {t("settings.oauthGoogleDesc")}
            {" "}
            <code className="text-[#4F6BED]">
              {(oauthRedirectBase || t("settings.callbackHint")) + "/api/oauth/google/callback"}
            </code>
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label>{t("settings.enableGoogle")}</Label>
              <p className="text-xs text-[#5C6472]">{t("settings.oauthEntryHint")}</p>
            </div>
            <Switch checked={googleEnabled} onCheckedChange={setGoogleEnabled} />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="google_client_id">{t("settings.clientId")}</Label>
              <Input
                id="google_client_id"
                value={googleClientID}
                onChange={(e) => setGoogleClientID(e.target.value)}
                placeholder="xxxxx.apps.googleusercontent.com"
                className="font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="google_client_secret">{t("settings.clientSecret")}</Label>
              <Input
                id="google_client_secret"
                type="password"
                value={googleClientSecret}
                onChange={(e) => setGoogleClientSecret(e.target.value)}
                placeholder="Google OAuth 客户端密钥"
                className="font-mono"
              />
            </div>
          </div>
          <Button
            onClick={() =>
              save.mutate({
                oauth_google_enabled: String(googleEnabled),
                oauth_google_client_id: googleClientID,
                oauth_google_client_secret: googleClientSecret,
                oauth_redirect_base_url: oauthRedirectBase,
              })
            }
            disabled={save.isPending}
          >
            {t("settings.saveOAuth")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
