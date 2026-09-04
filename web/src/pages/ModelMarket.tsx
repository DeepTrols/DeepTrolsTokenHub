import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { useState } from "react";
import { ModelData } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { Cpu, Search } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

const cl: Record<string, string> = {
  chat: "modelmarket.catChat",
  embedding: "modelmarket.catEmbedding",
  image: "modelmarket.catImage",
  audio: "modelmarket.catAudio",
  video: "modelmarket.catVideo",
};
const pl: Record<string, string> = {
  deepseek: "DeepSeek",
  openai: "OpenAI",
  anthropic: "Anthropic",
  google: "Google Gemini",
  qwen: "Qwen",
  zhipu: "智谱AI",
  moonshot: "Moonshot",
  baidu: "百度文心",
  xfyun: "讯飞星火",
  bytedance: "字节豆包",
  tencent: "腾讯混元",
  lingyi: "零一万物",
  openrouter: "OpenRouter",
  siliconflow: "SiliconFlow",
};
function pLabel(p: string): string {
  return pl[p] || p;
}

export default function ModelMarket() {
  const { t } = useTranslation();
  const { data: md, isLoading, isError, error, refetch } = useConsoleQuery<{ data: ModelData[] }>("/models");
  const models = md?.data ?? [];
  const [s, setS] = useState("");
  const [prov, setProv] = useState("");
  const [sort, setSort] = useState<"default" | "priceAsc" | "priceDesc">("default");
  const providers = Array.from(new Set(models.map((m) => m.provider))).sort();
  let filtered = models.filter((m) => {
    if (prov && m.provider !== prov) return false;
    if (!s) return true;
    const q = s.toLowerCase();
    return m.code.toLowerCase().includes(q) || m.display_name.toLowerCase().includes(q) || m.provider.toLowerCase().includes(q);
  });
  const priceOf = (m: ModelData) => {
    const v = m.pricing?.["input"] ?? m.pricing?.["output"];
    const n = Number(v);
    return Number.isFinite(n) ? n : Number.MAX_SAFE_INTEGER;
  };
  if (sort !== "default") {
    filtered = [...filtered].sort((a, b) => (sort === "priceAsc" ? priceOf(a) - priceOf(b) : priceOf(b) - priceOf(a)));
  }

  if (isLoading)
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("modelmarket.title")}</h2>
        </div>
        <Card>
          <CardContent className="p-12 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-[#F78B28] border-t-transparent rounded-full mx-auto mb-3" />
            <p className="text-muted-foreground">{t("modelmarket.loading")}</p>
          </CardContent>
        </Card>
      </div>
    );
  if (isError)
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("modelmarket.title")}</h2>
        </div>
        <Card className="border-destructive/20">
          <CardContent className="p-6 text-center">
            <p className="text-destructive mb-3">{error instanceof Error ? error.message : t("modelmarket.loadFailed")}</p>
            <Button variant="destructive" size="sm" onClick={() => refetch()}>
              {t("modelmarket.retry")}
            </Button>
          </CardContent>
        </Card>
      </div>
    );

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("modelmarket.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("modelmarket.subtitle")}</p>
      </div>
      <div className="mb-4 relative">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder={t("modelmarket.searchPlaceholder")} value={s} onChange={(e) => setS(e.target.value)} className="pl-10 h-10 text-sm" />
      </div>
      <div className="mb-4 flex flex-wrap gap-3">
        <select value={prov} onChange={(e) => setProv(e.target.value)}
          className="glass-soft rounded-xl px-3 py-2 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
          <option value="">{t("modelmarket.allProviders")}</option>
          {providers.map((p) => <option key={p} value={p}>{pLabel(p)}</option>)}
        </select>
        <select value={sort} onChange={(e) => setSort(e.target.value as typeof sort)}
          className="glass-soft rounded-xl px-3 py-2 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
          <option value="default">{t("modelmarket.sortDefault")}</option>
          <option value="priceAsc">{t("modelmarket.sortAsc")}</option>
          <option value="priceDesc">{t("modelmarket.sortDesc")}</option>
        </select>
      </div>
      {filtered.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center text-muted-foreground">{t("modelmarket.empty")}</CardContent>
        </Card>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {filtered.map((m) => (
          <div key={m.code} className="glass-soft rounded-xl p-4 hover:bg-white/85 transition-all">
            <div className="flex items-start gap-3 mb-3">
              <div className="p-2 bg-[#F78B28]/10 rounded-xl shrink-0">
                <Cpu size={20} className="text-primary-700" />
              </div>
              <div className="flex-1">
                <h4 className="font-semibold text-sm truncate">{m.display_name}</h4>
                <p className="text-xs text-muted-foreground mt-0.5 flex flex-wrap gap-1">
                  <Badge variant="secondary" className="text-xs">
                    {pLabel(m.provider)}
                  </Badge>
                  <Badge variant="secondary" className="text-xs">
                    {t(cl[m.category] || m.category)}
                  </Badge>
                  {m.context_window > 0 && (
                    <Badge variant="secondary" className="text-xs">
                      {m.context_window.toLocaleString()} ctx
                    </Badge>
                  )}
                </p>
              </div>
            </div>
            {m.pricing && Object.keys(m.pricing).length > 0 && (
              <div className="border-t border-black/10 pt-2.5">
                <p className="text-xs text-muted-foreground mb-1.5">{t("modelmarket.pricingLabel")}</p>
                <div className="grid grid-cols-2 gap-1 text-xs">
                  {Object.entries(m.pricing).map(([dim, price]) => {
                    const dimL: Record<string, string> = {
                      input: "modelmarket.dimInput",
                      output: "modelmarket.dimOutput",
                      cache_read: "modelmarket.dimCacheRead",
                      cache_write: "modelmarket.dimCacheWrite",
                      reasoning: "modelmarket.dimReasoning",
                      image: "modelmarket.dimImage",
                      audio: "modelmarket.dimAudio",
                      tts: "modelmarket.dimTts",
                      video: "modelmarket.dimVideo",
                    };
                    return (
                      <div key={dim} className="flex justify-between py-1 px-2 glass-soft rounded-lg">
                        <span className="text-muted-foreground">{t(dimL[dim] || dim)}</span>
                        <span className="font-mono font-medium">{price === "0" || price === "0.00" ? t("modelmarket.free") : price + " CNY"}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
