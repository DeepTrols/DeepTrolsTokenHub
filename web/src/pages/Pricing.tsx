import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, Search } from "lucide-react";
import { publicApi } from "../lib/api";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface PricingConditions {
  min_total_tokens?: number;
  max_total_tokens?: number;
}

export interface PricingRow {
  dimension: string;
  unit_name: string;
  unit_price: string;
  price_type: string;
  period?: string;
  conditions?: PricingConditions;
}

export interface PricingModel {
  id: string;
  code: string;
  provider: string;
  category: string;
  display_name: string;
  description?: string;
  context_window: number;
  pricings: PricingRow[];
  pricing: Record<string, string>;
}

export interface PricingResponse {
  data: PricingModel[];
  total: number;
}

const DIMENSION_LABEL: Record<string, string> = {
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

const tierLabel = (c?: PricingConditions) => {
  if (!c) return "";
  const parts: string[] = [];
  if (typeof c.min_total_tokens === "number") parts.push(`≥ ${c.min_total_tokens}`);
  if (typeof c.max_total_tokens === "number") parts.push(`≤ ${c.max_total_tokens}`);
  return parts.join(" · ");
};

export default function Pricing() {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: ["public", "pricing"],
    queryFn: () => publicApi.get<PricingResponse>("/pricing"),
    staleTime: 5 * 60_000,
  });
  const [category, setCategory] = useState("all");
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  const models = query.data?.data ?? [];
  const categories = useMemo(() => {
    const set = new Set(models.map((m) => m.category || "other"));
    return ["all", ...set];
  }, [models]);

  const filtered = useMemo(() => {
    const kw = search.trim().toLowerCase();
    return models.filter((m) => {
      if (category !== "all" && (m.category || "other") !== category) return false;
      if (!kw) return true;
      return (
        m.display_name.toLowerCase().includes(kw) ||
        m.code.toLowerCase().includes(kw) ||
        m.provider.toLowerCase().includes(kw)
      );
    });
  }, [models, category, search]);

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("pricing.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("pricing.subtitle")}</p>
      </div>

      {query.isLoading ? (
        <LoadingState message={t("pricing.loading")} />
      ) : query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} title={t("pricing.loadFailed")} />
      ) : models.length === 0 ? (
        <EmptyState title={t("pricing.empty")} description={t("pricing.emptyDesc")} />
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-3">
            <div className="flex flex-wrap gap-2">
              {categories.map((c) => (
                <button
                  key={c}
                  onClick={() => setCategory(c)}
                  className={`px-3 py-1.5 rounded-full text-[12.5px] font-semibold border transition-all ${
                    category === c
                      ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-transparent shadow-[0_6px_14px_rgba(79,107,237,0.28)]"
                      : "glass-soft text-[#5C6472] hover:text-[#161A23]"
                  }`}
                >
                  {c === "all" ? t("pricing.all") : c}
                </button>
              ))}
            </div>
            <div className="relative ml-auto w-[220px]">
              <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-[#8C93A1]" />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("pricing.searchPlaceholder")}
                className="w-full glass-soft rounded-xl pl-9 pr-3 py-2 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20"
              />
            </div>
          </div>

          <p className="text-xs text-[#8C93A1] mb-3">{t("pricing.count", { count: filtered.length })}</p>

          {filtered.length === 0 ? (
            <EmptyState title={t("pricing.noMatch")} description={t("pricing.noMatchDesc")} />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              {filtered.map((m) => {
                const isOpen = expanded === m.code;
                return (
                  <div key={m.code} className="glass rounded-[22px] p-5 flex flex-col">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h3 className="font-display font-semibold truncate">{m.display_name || m.code}</h3>
                        <p className="text-[12px] text-[#5C6472] font-mono mt-0.5 truncate">{m.code}</p>
                      </div>
                      <span className="text-[11px] font-semibold text-[#8C93A1] bg-white/60 rounded-full px-2.5 py-1 shrink-0">
                        {m.category || t("pricing.other")}
                      </span>
                    </div>
                    <div className="mt-3 flex items-center justify-between text-[12px] text-[#5C6472]">
                      <span>{m.provider}</span>
                      <span>{m.context_window ? t("pricing.contextK", { n: (m.context_window / 1000).toFixed(0) }) : "—"}</span>
                    </div>
                    {m.description && <p className="mt-2 text-[12.5px] text-[#5C6472]/80 line-clamp-2">{m.description}</p>}

                    <div className="mt-4 grid grid-cols-2 gap-2">
                      <div className="glass-soft rounded-xl p-2.5">
                        <div className="text-[11px] text-[#8C93A1]">{t("pricing.inputPrice")}</div>
                        <div className="text-[14px] font-semibold text-[#161A23]">
                          {m.pricing?.input ? `¥${m.pricing.input}` : "—"}
                        </div>
                      </div>
                      <div className="glass-soft rounded-xl p-2.5">
                        <div className="text-[11px] text-[#8C93A1]">{t("pricing.outputPrice")}</div>
                        <div className="text-[14px] font-semibold text-[#161A23]">
                          {m.pricing?.output ? `¥${m.pricing.output}` : "—"}
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => setExpanded(isOpen ? null : m.code)}
                      className="mt-3 flex items-center justify-center gap-1 text-[12px] font-semibold text-[#4F6BED] hover:underline"
                    >
                      {isOpen ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
                      {isOpen ? t("pricing.collapseDetail") : t("pricing.expandDetail")}
                    </button>

                    {isOpen && (
                      <div className="mt-2 overflow-x-auto">
                        <table className="w-full text-[12px]">
                          <thead>
                            <tr className="text-left text-[#8C93A1] border-b border-black/5">
                              <th className="py-1.5 pr-2">{t("pricing.thDimension")}</th>
                              <th className="py-1.5 pr-2">{t("pricing.thUnit")}</th>
                              <th className="py-1.5 pr-2">{t("pricing.thPrice")}</th>
                              <th className="py-1.5 pr-2">{t("pricing.thType")}</th>
                              <th className="py-1.5">{t("pricing.thTier")}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {(m.pricings ?? []).map((p, i) => (
                              <tr key={i} className="border-b border-black/[0.03]">
                                <td className="py-1.5 pr-2">{t(DIMENSION_LABEL[p.dimension] ?? p.dimension)}</td>
                                <td className="py-1.5 pr-2 text-[#5C6472]">{p.unit_name}</td>
                                <td className="py-1.5 pr-2 font-medium">¥{p.unit_price}</td>
                                <td className="py-1.5 text-[#8C93A1]">{p.price_type === "sell" ? t("pricing.sell") : t("pricing.cost")}</td>
                                <td className="py-1.5 text-[#8B6FE8]">{tierLabel(p.conditions) || "—"}</td>
                              </tr>
                            ))}
                            {(m.pricings ?? []).length === 0 && (
                              <tr>
                                <td colSpan={5} className="py-2 text-[#8C93A1]">{t("pricing.noPricing")}</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </>
      )}
    </div>
  );
}
