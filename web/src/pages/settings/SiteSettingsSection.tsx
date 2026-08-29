import React, { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

type SiteSettings = Record<string, string>;

export default function SiteSettingsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SiteSettings>("/settings/site");
  const [form, setForm] = useState<SiteSettings>({});
  const fileRefs = useRef<Record<string, HTMLInputElement | null>>({});

  useEffect(() => {
    if (data) {
      const next: SiteSettings = {};
      for (const [k, v] of Object.entries(data)) {
        next[k] = typeof v === "string" ? v : JSON.stringify(v);
      }
      setForm(next);
    }
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, SiteSettings>("put", "/settings/site", "/settings/site", {
    onSuccess: () => toast.success(t("common.saved")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
  });

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((prev) => ({ ...prev, [key]: e.target.value }));

  const saveKeys = (keys: string[]) => () => {
    const patch: SiteSettings = {};
    for (const k of keys) patch[k] = form[k] ?? "";
    save.mutate(patch);
  };

  const field = (key: string, label: string, type: "text" | "textarea" = "text") => (
    <div className="space-y-1.5">
      <Label htmlFor={key}>{label}</Label>
      {type === "textarea" ? (
        <textarea
          id={key}
          value={form[key] ?? ""}
          onChange={set(key)}
          rows={4}
          className="w-full rounded-xl border border-input bg-white/70 px-3 py-2 text-sm"
        />
      ) : (
        <Input id={key} value={form[key] ?? ""} onChange={set(key)} />
      )}
    </div>
  );

  const uploadAsset = (key: "logo_url" | "favicon_url") => async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const fd = new FormData();
    fd.append("file", file);
    try {
      const res = await fetch("/api/admin/settings/upload", {
        method: "POST",
        body: fd,
        credentials: "include",
      });
      const data = (await res.json().catch(() => ({}))) as { url?: string; error?: string };
      if (!res.ok || !data.url) {
        throw new Error(data.error || "上传失败");
      }
      setForm((prev) => ({ ...prev, [key]: data.url! }));
      toast.success(t("settings.uploadSuccess"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("common.saveFailed"));
    } finally {
      e.target.value = "";
    }
  };

  const uploadField = (key: "logo_url" | "favicon_url", label: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={key}>{label}</Label>
      <div className="flex gap-2">
        <Input id={key} value={form[key] ?? ""} onChange={set(key)} className="flex-1" />
        <Button type="button" variant="outline" onClick={() => fileRefs.current[key]?.click()}>
          {t("settings.upload")}
        </Button>
        <input
          ref={(el) => {
            fileRefs.current[key] = el;
          }}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif,image/x-icon"
          className="hidden"
          onChange={uploadAsset(key)}
        />
      </div>
      <p className="text-xs text-[#5C6472]">{t("settings.uploadHint")}</p>
    </div>
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.siteTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.siteSubtitle")}</p>
      </div>
      <Tabs defaultValue="basic">
        <TabsList>
          <TabsTrigger value="basic">{t("settings.tabBasic")}</TabsTrigger>
          <TabsTrigger value="notice">{t("settings.tabNotice")}</TabsTrigger>
          <TabsTrigger value="nav">{t("settings.tabNav")}</TabsTrigger>
          <TabsTrigger value="legal">{t("settings.tabLegal")}</TabsTrigger>
        </TabsList>

        <TabsContent value="basic">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.basicTitle")}</CardTitle>
              <CardDescription>{t("settings.basicDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {field("site_name", t("settings.siteName"))}
              {uploadField("logo_url", t("settings.logo"))}
              {uploadField("favicon_url", t("settings.favicon"))}
              {field("server_address", t("settings.serverAddress"))}
              {field("contact_email", t("settings.contactEmail"))}
              <Button onClick={saveKeys(["site_name", "logo_url", "favicon_url", "server_address", "contact_email"])} disabled={save.isPending}>
                {t("common.save")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="notice">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.noticeTitle")}</CardTitle>
              <CardDescription>{t("settings.noticeDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {field("notice", t("settings.notice"), "textarea")}
              {field("footer_text", t("settings.footerText"), "textarea")}
              <Button onClick={saveKeys(["notice", "footer_text"])} disabled={save.isPending}>{t("common.save")}</Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="nav">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.navTitle")}</CardTitle>
              <CardDescription>{t("settings.navDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {field("header_nav_modules", t("settings.headerNav"), "textarea")}
              {field("sidebar_modules", t("settings.sidebarModules"), "textarea")}
              <Button onClick={saveKeys(["header_nav_modules", "sidebar_modules"])} disabled={save.isPending}>{t("common.save")}</Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="legal">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.legalTitle")}</CardTitle>
              <CardDescription>{t("settings.legalDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {field("legal.user_agreement", t("settings.userAgreement"), "textarea")}
              {field("legal.privacy_policy", t("settings.privacyPolicy"), "textarea")}
              <Button onClick={saveKeys(["legal.user_agreement", "legal.privacy_policy"])} disabled={save.isPending}>{t("common.save")}</Button>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
