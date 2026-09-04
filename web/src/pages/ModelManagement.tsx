import { LoadingState } from "@/components/StateViews";
import { Button } from "@/components/ui/button";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Plus, Edit, Trash2, Box, ChevronDown, ChevronRight } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

interface PricingConditions {
  min_total_tokens?: number;
  max_total_tokens?: number;
}

interface PricingRow {
  dimension: string;
  unit_name: string;
  unit_price: string;
  price_type?: string;
  period?: string;
  conditions?: PricingConditions;
}

interface ModelDetail {
  id: string;
  code: string;
  provider: string;
  category: string;
  display_name: string;
  description?: string;
  context_window: number;
  max_output_tokens?: number;
  status?: string;
  pricings: PricingRow[];
}

const DIMENSIONS = ["input", "output", "cache_read", "cache_write", "reasoning", "image", "audio", "tts", "video"];
const emptyPricing = (): PricingRow => ({ dimension: "", unit_name: "1M tokens", unit_price: "", price_type: "sell", period: "off_peak" });

const priceTypeLabel = (t?: string) => (t === "cost" ? "modelmgmt.priceTypeCost" : "modelmgmt.priceTypeSell");
const periodLabel = (p?: string) => (p === "peak" ? "modelmgmt.periodPeak" : "modelmgmt.periodOffPeak");
const conditionsLabel = (c?: PricingConditions) => {
  if (!c) return "";
  const parts: string[] = [];
  if (typeof c.min_total_tokens === "number") parts.push(`≥ ${c.min_total_tokens}`);
  if (typeof c.max_total_tokens === "number") parts.push(`≤ ${c.max_total_tokens}`);
  return parts.join(" · ");
};

/** Serializes a pricing row's tier conditions, omitting empty conditions. */
const buildConditions = (p: PricingRow): PricingConditions | undefined => {
  const c = p.conditions || {};
  const out: PricingConditions = {};
  if (typeof c.min_total_tokens === "number" && Number.isFinite(c.min_total_tokens)) {
    out.min_total_tokens = c.min_total_tokens;
  }
  if (typeof c.max_total_tokens === "number" && Number.isFinite(c.max_total_tokens)) {
    out.max_total_tokens = c.max_total_tokens;
  }
  return Object.keys(out).length > 0 ? out : undefined;
};

const PROVIDER_OPTIONS = [
  { v: "deepseek", l: "DeepSeek 深度求索" },
  { v: "qwen", l: "Qwen 通义千问" },
  { v: "zhipu", l: "智谱AI ChatGLM" },
  { v: "moonshot", l: "Moonshot 月之暗面" },
  { v: "baidu", l: "百度文心" },
  { v: "xfyun", l: "讯飞星火" },
  { v: "bytedance", l: "字节豆包" },
  { v: "tencent", l: "腾讯混元" },
  { v: "lingyi", l: "零一万物 Yi" },
  { v: "openai", l: "OpenAI" },
  { v: "anthropic", l: "Anthropic" },
  { v: "google", l: "Google Gemini" },
  { v: "openrouter", l: "OpenRouter" },
  { v: "siliconflow", l: "SiliconFlow 硅基流动" },
];

const providerLabel = (p: string) => {
  const found = PROVIDER_OPTIONS.find((o) => o.v === p);
  return found ? found.l : p;
};

export default function ModelManagement() {
  const { t } = useTranslation();
  const {
    data: modelData,
    isLoading: loading,
    isError,
    error: queryError,
    refetch,
  } = useAdminQuery<{ data: ModelDetail[]; total: number }>("/models");
  const models = Array.isArray(modelData?.data) ? modelData.data : [];
  const loadError = isError ? (queryError instanceof Error ? queryError.message : String(queryError)) : "";
  const [actionError, setActionError] = useState("");
  const [editing, setEditing] = useState<ModelDetail | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [code, setCode] = useState("");
  const [provider, setProvider] = useState("");
  const [category, setCategory] = useState("chat");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [contextWindow, setContextWindow] = useState(0);
  const [maxOutputTokens, setMaxOutputTokens] = useState(0);
  const [pricings, setPricings] = useState<PricingRow[]>([emptyPricing()]);
  const [search, setSearch] = useState("");
  const [filterProvider, setFilterProvider] = useState("");
  const [filterCategory, setFilterCategory] = useState("");

  const createMutation = useAdminMutation<unknown, Record<string, unknown>>("post", "/models");
  const updateMutation = useAdminMutation<unknown, { id: string } & Record<string, unknown>>(
    "put",
    (v) => `/models/${v.id}`,
    "/models",
  );
  const deleteMutation = useAdminMutation<unknown, { id: string }>(
    "delete",
    (v) => `/models/${v.id}`,
    "/models",
  );
  const syncCatalog = useAdminMutation<{ created: number; updated: number }, { overwrite: boolean }>(
    "post",
    "/models/sync_catalog",
    "/models",
    { onSuccess: () => refetch() },
  );

  const resetForm = () => {
    setCode(""); setProvider(""); setCategory("chat"); setDisplayName("");
    setDescription(""); setContextWindow(0); setMaxOutputTokens(0);
    setPricings([emptyPricing()]); setEditing(null);
    setActionError("");
  };

  const openCreate = () => { resetForm(); setShowForm(true); };
  const openEdit = (m: ModelDetail) => {
    setCode(m.code); setProvider(m.provider); setCategory(m.category);
    setDisplayName(m.display_name); setDescription(m.description ?? "");
    setContextWindow(m.context_window); setMaxOutputTokens(m.max_output_tokens ?? 0);
    // Only sell rows are editable in the UI; cost rows are owned by provider
    // cost sync and must never be sent back (the backend ignores them anyway).
    const sellPricings = (m.pricings || []).filter((p) => p.price_type !== "cost");
    setPricings(
      sellPricings.length > 0
        ? sellPricings.map((p) => ({ ...p, price_type: p.price_type || "sell", period: p.period || "off_peak" }))
        : [emptyPricing()],
    );
    setEditing(m); setShowForm(true);
  };

  const handleSubmit = async () => {
    if (!code.trim() || !provider.trim()) return;
    const body = {
      code: code.trim(), provider: provider.trim(), category,
      display_name: displayName, description, context_window: contextWindow, max_output_tokens: maxOutputTokens,
      pricings: pricings
        .filter((p) => p.dimension && p.unit_price && p.price_type !== "cost")
        .map((p) => {
          const conditions = buildConditions(p);
          return {
            dimension: p.dimension, unit_name: p.unit_name, unit_price: p.unit_price, period: p.period,
            price_type: p.price_type || "sell",
            ...(conditions ? { conditions } : {}),
          };
        }),
    };
    try {
      if (editing) {
        await updateMutation.mutateAsync({ id: editing.id, ...body });
      } else {
        await createMutation.mutateAsync(body);
      }
      resetForm(); setShowForm(false);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleDelete = async (m: ModelDetail) => {
    if (!confirm(t("modelmgmt.deleteConfirm", { name: m.display_name || m.code }))) return;
    try {
      await deleteMutation.mutateAsync({ id: m.id });
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    }
  };

  const addPricing = () => {
    if (pricings.length >= DIMENSIONS.length) return;
    setPricings([...pricings, emptyPricing()]);
  };

  const removePricing = (idx: number) => {
    if (pricings.length <= 1) return;
    setPricings(pricings.filter((_, i) => i !== idx));
  };

  const updatePricing = (idx: number, field: keyof PricingRow, value: string) => {
    setPricings(pricings.map((p, i) => (i === idx ? { ...p, [field]: value } : p)));
  };

  const updateCondition = (idx: number, key: keyof PricingConditions, value: string) => {
    setPricings((prev) =>
      prev.map((p, i) => {
        if (i !== idx) return p;
        const conditions: PricingConditions = { ...(p.conditions || {}) };
        const trimmed = value.trim();
        if (trimmed === "") {
          delete conditions[key];
        } else {
          const num = Number(trimmed);
          if (!Number.isNaN(num)) conditions[key] = num;
        }
        return { ...p, conditions };
      }),
    );
  };

  const statusBadge = (s?: string) => {
    switch (s) {
      case "active": return <span className="status-pill ok"><i />{t("modelmgmt.statusActive")}</span>;
      case "inactive": return <span className="status-pill run"><i />{t("modelmgmt.statusInactive")}</span>;
      case "beta": return <span className="status-pill text-primary-700"><i className="bg-[#F78B28] shadow-[0_0_8px_#F78B28]" />{t("modelmgmt.statusBeta")}</span>;
      default: return <span className="status-pill run"><i />{s || "active"}</span>;
    }
  };

  // Search + filter, then group by provider
  const q = search.trim().toLowerCase();
  const filtered = models.filter((m) => {
    const hay = `${m.code} ${m.display_name} ${m.provider} ${m.category}`.toLowerCase();
    return (
      (!q || hay.includes(q)) &&
      (!filterProvider || m.provider === filterProvider) &&
      (!filterCategory || m.category === filterCategory)
    );
  });
  const grouped: Record<string, ModelDetail[]> = {};
  for (const m of filtered) {
    const key = m.provider || "unknown";
    if (!grouped[key]) grouped[key] = [];
    grouped[key].push(m);
  }
  const sortedGroups = Object.entries(grouped).sort((a, b) => a[0].localeCompare(b[0]));

  // Collapsible state — all expanded by default
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const toggleGroup = (provider: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(provider)) {
        next.delete(provider);
      } else {
        next.add(provider);
      }
      return next;
    });

  if (loading) {
    return (
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight mb-6">{t("modelmgmt.title")}</h2>
        <LoadingState message={t("modelmgmt.loading")} />
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">{t("modelmgmt.title")}</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">{t("modelmgmt.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => syncCatalog.mutate({ overwrite: true })} disabled={syncCatalog.isPending}>
            {syncCatalog.isPending ? t("modelmgmt.syncing") : t("modelmgmt.syncCatalog")}
          </Button>
          <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />{t("modelmgmt.addModel")}</Button>
        </div>
      </div>

      {loadError && (
        <div className="mb-4 p-4 glass-soft border-[#E5484D]/25 rounded-xl text-[#C4372C] text-sm">
          <p className="font-medium">{t("modelmgmt.loadFailed")}</p>
          <p className="mt-1">{loadError}</p>
          <Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>{t("modelmgmt.retry")}</Button>
        </div>
      )}

      {showForm && (
        <div className="mb-6 p-6 glass rounded-2xl">
          <h3 className="font-display font-semibold mb-4">{editing ? t("modelmgmt.editTitle") : t("modelmgmt.addTitle")}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.code")}</label>
              <input type="text" value={code} onChange={(e) => setCode(e.target.value)} placeholder={t("modelmgmt.codePlaceholder")}
                disabled={!!editing}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm disabled:opacity-50 focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.provider")}</label>
              <select value={provider} onChange={(e) => setProvider(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
                <option value="">{t("modelmgmt.selectProvider")}</option>
                {PROVIDER_OPTIONS.map((o) => (
                  <option key={o.v} value={o.v}>{o.l}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.category")}</label>
              <select value={category} onChange={(e) => setCategory(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
                <option value="chat">{t("modelmgmt.catChat")}</option>
                <option value="embedding">{t("modelmgmt.catEmbedding")}</option>
                <option value="image">{t("modelmgmt.catImage")}</option>
                <option value="audio">{t("modelmgmt.catAudio")}</option>
                <option value="video">{t("modelmgmt.catVideo")}</option>
                <option value="rerank">{t("modelmgmt.catRerank")}</option>
              </select>
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.displayName")}</label>
              <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={t("modelmgmt.displayNamePlaceholder")}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.contextWindow")}</label>
              <input type="number" value={contextWindow} onChange={(e) => setContextWindow(Number(e.target.value))}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.maxOutput")}</label>
              <input type="number" value={maxOutputTokens} onChange={(e) => setMaxOutputTokens(Number(e.target.value))}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>
            <div className="col-span-2">
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">{t("modelmgmt.description")}</label>
              <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2}
                placeholder={t("modelmgmt.descPlaceholder")}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>
          </div>

          <div className="mt-6">
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-[#161A23]">{t("modelmgmt.pricingDims")}</label>
              <button onClick={addPricing} className="text-sm text-primary-700 font-semibold hover:underline">{t("modelmgmt.addDimension")}</button>
            </div>
            <p className="text-xs text-[#5C6472] mb-2">{t("modelmgmt.pricingHint")}</p>
            {pricings.map((p, idx) => (
              <div key={idx} className="flex gap-3 mb-2 items-start">
                <select value={p.dimension} onChange={(e) => updatePricing(idx, "dimension", e.target.value)}
                  className="glass-soft rounded-xl px-3 py-2 text-sm w-32 focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
                  <option value="">{t("modelmgmt.selectDimension")}</option>
                  {DIMENSIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <input type="text" value={p.unit_name} onChange={(e) => updatePricing(idx, "unit_name", e.target.value)}
                  placeholder={t("modelmgmt.unitPlaceholder")} className="glass-soft rounded-xl px-3 py-2 text-sm w-20 focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
                <input type="text" value={p.unit_price} onChange={(e) => updatePricing(idx, "unit_price", e.target.value)}
                  placeholder={t("modelmgmt.pricePlaceholder")} className="glass-soft rounded-xl px-3 py-2 text-sm w-32 font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
                <select value={p.period || "off_peak"} onChange={(e) => updatePricing(idx, "period", e.target.value)}
                  className="glass-soft rounded-xl px-3 py-2 text-sm w-24 focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
                  <option value="off_peak">{t("modelmgmt.offPeak")}</option>
                  <option value="peak">{t("modelmgmt.peak")}</option>
                </select>
                <input
                  type="number"
                  value={p.conditions?.min_total_tokens ?? ""}
                  onChange={(e) => updateCondition(idx, "min_total_tokens", e.target.value)}
                  placeholder={t("modelmgmt.minTokens")}
                  title={t("modelmgmt.minTokens")}
                  className="glass-soft rounded-xl px-3 py-2 text-sm w-28 font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
                />
                <input
                  type="number"
                  value={p.conditions?.max_total_tokens ?? ""}
                  onChange={(e) => updateCondition(idx, "max_total_tokens", e.target.value)}
                  placeholder={t("modelmgmt.maxTokens")}
                  title={t("modelmgmt.maxTokens")}
                  className="glass-soft rounded-xl px-3 py-2 text-sm w-28 font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
                />
                <button onClick={() => removePricing(idx)} className="p-2 text-[#5C6472]/70 hover:text-[#C4372C]" title={t("modelmgmt.remove")}>
                  <Trash2 size={16} />
                </button>
              </div>
            ))}
          </div>

          {actionError && <p className="text-sm text-[#C4372C] mt-3">{actionError}</p>}

          <div className="flex gap-3 mt-6">
            <Button onClick={handleSubmit}>
              {editing ? t("modelmgmt.saveChanges") : t("modelmgmt.confirmCreate")}
            </Button>
            <Button variant="outline" onClick={() => { setShowForm(false); resetForm(); }}>
              {t("modelmgmt.cancel")}
            </Button>
          </div>
        </div>
      )}

      <div className="mb-4 flex flex-wrap gap-3">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("modelmgmt.searchPlaceholder")}
          className="min-w-[240px] flex-1 glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
        />
        <select
          value={filterProvider}
          onChange={(e) => setFilterProvider(e.target.value)}
          className="glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
        >
          <option value="">{t("modelmgmt.allProviders")}</option>
          {PROVIDER_OPTIONS.map((o) => (
            <option key={o.v} value={o.v}>{o.l}</option>
          ))}
        </select>
        <select
          value={filterCategory}
          onChange={(e) => setFilterCategory(e.target.value)}
          className="glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
        >
          <option value="">{t("modelmgmt.allCategories")}</option>
          <option value="chat">{t("modelmgmt.catChat")}</option>
          <option value="embedding">{t("modelmgmt.catEmbedding")}</option>
          <option value="image">{t("modelmgmt.catImage")}</option>
          <option value="audio">{t("modelmgmt.catAudio")}</option>
          <option value="video">{t("modelmgmt.catVideo")}</option>
          <option value="rerank">{t("modelmgmt.catRerank")}</option>
        </select>
      </div>

      <div className="space-y-6">
        {filtered.length === 0 && (
          <div className="p-12 text-center text-[#5C6472] glass rounded-2xl">
            <Box size={40} className="mx-auto mb-3 opacity-30" />
            <p>{models.length === 0 ? t("modelmgmt.empty") : t("modelmgmt.noMatch")}</p>
            <p className="text-sm text-[#5C6472]/70 mt-1">
              {models.length === 0 ? t("modelmgmt.emptyDesc") : t("modelmgmt.noMatchDesc")}
            </p>
          </div>
        )}
        {sortedGroups.map(([pv, pvModels]) => {
          const isCollapsed = collapsed.has(pv);
          return (
          <div key={pv} className="glass rounded-2xl overflow-hidden">
            <button
              onClick={() => toggleGroup(pv)}
              className="w-full px-5 py-3.5 bg-white/55 border-b border-black/10 flex items-center gap-3 hover:bg-white/80 transition-colors"
            >
              {isCollapsed ? <ChevronRight size={16} className="text-[#5C6472]/70" /> : <ChevronDown size={16} className="text-[#5C6472]/70" />}
              <h3 className="font-semibold text-sm text-[#161A23]">
                {providerLabel(pv) || pv}
              </h3>
              <span className="text-xs text-[#5C6472]/70">{t("modelmgmt.modelsCount", { count: pvModels.length })}</span>
            </button>
            {!isCollapsed && pvModels.map((m) => (
              <div key={m.id} className="px-5 py-4 border-b border-black/[0.05] last:border-b-0 flex items-center justify-between hover:bg-white/60 transition-colors">
                <div>
                  <div className="flex items-center gap-2">
                    <p className="font-medium text-sm">{m.display_name || m.code}</p>
                    <span className="px-1.5 py-0.5 rounded-lg text-xs bg-[#F78B28]/10 text-primary-700 font-medium">
                      {providerLabel(m.provider)}
                    </span>
                    {statusBadge(m.status)}
                  </div>
                  <p className="text-xs text-[#5C6472] mt-0.5">
                    <code className="text-xs font-mono">{m.code}</code> · {m.category}
                    {m.context_window > 0 ? ` · ${Math.round(m.context_window / 1000)}k context` : ""}
                  </p>
                  {m.pricings && m.pricings.length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {m.pricings.map((p, i) => (
                        <span key={i} className="text-[11px] px-1.5 py-0.5 rounded-md bg-black/[0.04] text-[#5C6472]">
                          {p.dimension}: {p.unit_price}
                          <span className="ml-1 text-[#5C6472]/70">/ {p.unit_name || "unit"}</span>
                          <span className="ml-1 text-primary-700">{t(priceTypeLabel(p.price_type))}</span>
                          <span className="ml-1 text-[#A06B12]">{t(periodLabel(p.period))}</span>
                          {conditionsLabel(p.conditions) && (
                            <span className="ml-1 text-[#B94723]">{conditionsLabel(p.conditions)}</span>
                          )}
                        </span>
                      ))}
                    </div>
                  )}
                  {m.pricings && m.pricings.length > 0 && !m.pricings.some((p) => p.price_type !== "cost") && (
                    <p className="text-xs text-[#A06B12]/80 mt-0.5">{t("modelmgmt.costOnly")}</p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => updateMutation.mutateAsync({ id: m.id, status: m.status === "active" ? "inactive" : "active" })}
                    className="p-1.5 text-[#5C6472]/70 hover:text-[#9A4D06] rounded-lg hover:bg-white/70 transition-colors"
                    title={m.status === "active" ? t("modelmgmt.offline") : t("modelmgmt.online")}
                  >
                    {m.status === "active" ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                  </button>
                  <button onClick={() => openEdit(m)} className="p-1.5 text-[#5C6472]/70 hover:text-primary-700 rounded-lg hover:bg-white/70 transition-colors" title={t("modelmgmt.edit")}>
                    <Edit size={14} />
                  </button>
                  <button onClick={() => handleDelete(m)} className="p-1.5 text-[#5C6472]/70 hover:text-[#C4372C] rounded-lg hover:bg-white/70 transition-colors" title={t("modelmgmt.delete")}>
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )})}
      </div>
    </div>
  );
}
