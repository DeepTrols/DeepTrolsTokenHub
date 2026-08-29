import { useEffect, useState } from "react";
import { toast } from "sonner";
import { MessageSquare, Plus, Trash2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAdminMutation, useAdminQuery } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

interface PresetEntry {
  name: string;
  url: string;
}

/** Parses the persisted JSON array of {"name": "url"} entries. */
function parsePresets(raw: string | undefined): PresetEntry[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.map((entry) => {
      if (!entry || typeof entry !== "object") return { name: "", url: "" };
      const entries = Object.entries(entry as Record<string, unknown>);
      if (entries.length !== 1) return { name: "", url: "" };
      return { name: entries[0][0], url: String(entries[0][1] ?? "") };
    });
  } catch {
    return [];
  }
}

/**
 * External chat-client presets (new-api chat2link parity): each preset is
 * {"name": "https://chat.example/?api_key={key}&base_url={address}"}.
 */
export default function ChatPresetsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<Record<string, string>>("/settings/site");
  const [presets, setPresets] = useState<PresetEntry[]>([]);

  useEffect(() => {
    if (data) setPresets(parsePresets(data["chat_presets"]));
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, Record<string, string>>(
    "put",
    "/settings/site",
    "/settings/site",
    {
      onSuccess: () => toast.success(t("settings.savedPresets")),
      onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
    },
  );

  const update = (index: number, patch: Partial<PresetEntry>) =>
    setPresets((prev) => prev.map((p, i) => (i === index ? { ...p, ...patch } : p)));

  const handleSave = () => {
    const entries = presets
      .filter((p) => p.name.trim() && p.url.trim())
      .map((p) => ({ [p.name.trim()]: p.url.trim() }));
    save.mutate({ chat_presets: JSON.stringify(entries) });
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.contentTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.contentSubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.chatTitle")}</CardTitle>
          <CardDescription>
            {t("settings.chatDesc")}{" "}
            <code className="text-[#4F6BED]">{"{key}"}</code>（自动注入 API Key）、{" "}
            <code className="text-[#4F6BED]">{"{address}"}</code>（平台服务地址）、{" "}
            <code className="text-[#4F6BED]">{"{cherryConfig}"}</code> 等
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {presets.length === 0 && (
            <p className="text-sm text-[#8C93A1]">{t("settings.noPresets")}</p>
          )}
          {presets.map((p, i) => (
            <div key={i} className="grid grid-cols-[1fr_2fr_auto] gap-3 items-end">
              <div className="space-y-1.5">
                <Label htmlFor={`preset-name-${i}`}>{t("settings.name")}</Label>
                <Input
                  id={`preset-name-${i}`}
                  value={p.name}
                  onChange={(e) => update(i, { name: e.target.value })}
                  placeholder={t("settings.namePlaceholder")}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor={`preset-url-${i}`}>{t("settings.urlTemplate")}</Label>
                <Input
                  id={`preset-url-${i}`}
                  value={p.url}
                  onChange={(e) => update(i, { url: e.target.value })}
                  placeholder="https://chat.example.com/?api_key={key}&base_url={address}"
                  className="font-mono"
                />
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t("settings.deletePreset", { name: p.name || t("settings.presetX", { n: i + 1 }) })}
                onClick={() => setPresets((prev) => prev.filter((_, idx) => idx !== i))}
              >
                <Trash2 size={15} className="text-[#C4372C]" />
              </Button>
            </div>
          ))}
          <div className="flex items-center gap-3 pt-1">
            <Button variant="outline" size="sm" onClick={() => setPresets((prev) => [...prev, { name: "", url: "" }])}>
              <Plus size={14} className="mr-1" /> {t("settings.addPreset")}
            </Button>
            <Button size="sm" onClick={handleSave} disabled={save.isPending}>
              <MessageSquare size={14} className="mr-1" />
              {save.isPending ? t("common.saving") : t("common.save")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
