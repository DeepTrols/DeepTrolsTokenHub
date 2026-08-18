import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Plus, Edit, Trash2, Box, ChevronDown, ChevronRight } from "lucide-react";

interface PricingRow {
  dimension: string;
  unit_name: string;
  unit_price: string;
}

interface ModelDetail {
  id: string;
  code: string;
  provider: string;
  category: string;
  display_name: string;
  context_window: number;
  status?: string;
  pricings: PricingRow[];
}

const DIMENSIONS = ["input", "output", "cache_read", "cache_write", "reasoning", "image", "audio", "tts", "video"];
const emptyPricing = (): PricingRow => ({ dimension: "", unit_name: "token", unit_price: "" });

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
  const [contextWindow, setContextWindow] = useState(0);
  const [pricings, setPricings] = useState<PricingRow[]>([emptyPricing()]);

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

  const resetForm = () => {
    setCode(""); setProvider(""); setCategory("chat"); setDisplayName("");
    setContextWindow(0); setPricings([emptyPricing()]); setEditing(null);
  };

  const openCreate = () => { resetForm(); setShowForm(true); };
  const openEdit = (m: ModelDetail) => {
    setCode(m.code); setProvider(m.provider); setCategory(m.category);
    setDisplayName(m.display_name); setContextWindow(m.context_window);
    setPricings(m.pricings && m.pricings.length > 0 ? m.pricings : [emptyPricing()]);
    setEditing(m); setShowForm(true);
  };

  const handleSubmit = async () => {
    if (!code.trim() || !provider.trim()) return;
    const body = {
      code: code.trim(), provider: provider.trim(), category,
      display_name: displayName, context_window: contextWindow,
      pricings: pricings.filter((p) => p.dimension && p.unit_price),
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
    if (!confirm(`确定要下架模型 "${m.display_name || m.code}" 吗？`)) return;
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

  const statusBadge = (s?: string) => {
    switch (s) {
      case "active": return <span className="status-pill ok"><i />上架</span>;
      case "inactive": return <span className="status-pill run"><i />已下架</span>;
      case "beta": return <span className="status-pill text-[#4F6BED]"><i className="bg-[#4F6BED] shadow-[0_0_8px_#4F6BED]" />测试</span>;
      default: return <span className="status-pill run"><i />{s || "active"}</span>;
    }
  };

  // Group models by provider
  const grouped: Record<string, ModelDetail[]> = {};
  for (const m of models) {
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
      next.has(provider) ? next.delete(provider) : next.add(provider);
      return next;
    });

  if (loading) {
    return (
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight mb-6">模型管理</h2>
        <LoadingState message="加载模型中..." />
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">模型管理</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">管理模型目录，配置定价维度</p>
        </div>
        <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />添加模型</Button>
      </div>

      {loadError && (
        <div className="mb-4 p-4 glass-soft border-[#E5484D]/25 rounded-xl text-[#C4372C] text-sm">
          <p className="font-medium">加载失败</p>
          <p className="mt-1">{loadError}</p>
          <Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>重试</Button>
        </div>
      )}

      {showForm && (
        <div className="mb-6 p-6 glass rounded-2xl">
          <h3 className="font-display font-semibold mb-4">{editing ? "编辑模型" : "添加模型"}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">编码 *</label>
              <input type="text" value={code} onChange={(e) => setCode(e.target.value)} placeholder="例如: gpt-4o"
                disabled={!!editing}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm disabled:opacity-50 focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">提供商 *</label>
              <select value={provider} onChange={(e) => setProvider(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20">
                <option value="">选择提供商...</option>
                {PROVIDER_OPTIONS.map((o) => (
                  <option key={o.v} value={o.v}>{o.l}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">类别</label>
              <select value={category} onChange={(e) => setCategory(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20">
                <option value="chat">对话</option>
                <option value="embedding">嵌入</option>
                <option value="image">图片</option>
                <option value="audio">音频</option>
                <option value="video">视频</option>
                <option value="rerank">重排序</option>
              </select>
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">显示名称</label>
              <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="例如: GPT-4o"
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
            </div>
            <div>
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">上下文窗口</label>
              <input type="number" value={contextWindow} onChange={(e) => setContextWindow(Number(e.target.value))}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
            </div>
          </div>

          <div className="mt-6">
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-[#161A23]">定价维度</label>
              <button onClick={addPricing} className="text-sm text-[#4F6BED] font-semibold hover:underline">+ 添加维度</button>
            </div>
            {pricings.map((p, idx) => (
              <div key={idx} className="flex gap-3 mb-2 items-start">
                <select value={p.dimension} onChange={(e) => updatePricing(idx, "dimension", e.target.value)}
                  className="glass-soft rounded-xl px-3 py-2 text-sm w-32 focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20">
                  <option value="">选择维度</option>
                  {DIMENSIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <input type="text" value={p.unit_name} onChange={(e) => updatePricing(idx, "unit_name", e.target.value)}
                  placeholder="单位" className="glass-soft rounded-xl px-3 py-2 text-sm w-20 focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
                <input type="text" value={p.unit_price} onChange={(e) => updatePricing(idx, "unit_price", e.target.value)}
                  placeholder="单价" className="glass-soft rounded-xl px-3 py-2 text-sm w-32 font-mono focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
                <button onClick={() => removePricing(idx)} className="p-2 text-[#5C6472]/70 hover:text-[#C4372C]" title="移除">
                  <Trash2 size={16} />
                </button>
              </div>
            ))}
          </div>

          <div className="flex gap-3 mt-6">
            <Button onClick={handleSubmit}>
              {editing ? "保存更改" : "确认创建"}
            </Button>
            <Button variant="outline" onClick={() => { setShowForm(false); resetForm(); }}>
              取消
            </Button>
          </div>
        </div>
      )}

      <div className="space-y-6">
        {models.length === 0 && (
          <div className="p-12 text-center text-[#5C6472] glass rounded-2xl">
            <Box size={40} className="mx-auto mb-3 opacity-30" />
            <p>暂无模型</p>
            <p className="text-sm text-[#5C6472]/70 mt-1">点击上方按钮添加第一个模型</p>
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
              <span className="text-xs text-[#5C6472]/70">{pvModels.length} 个模型</span>
            </button>
            {!isCollapsed && pvModels.map((m) => (
              <div key={m.id} className="px-5 py-4 border-b border-black/[0.05] last:border-b-0 flex items-center justify-between hover:bg-white/60 transition-colors">
                <div>
                  <div className="flex items-center gap-2">
                    <p className="font-medium text-sm">{m.display_name || m.code}</p>
                    <span className="px-1.5 py-0.5 rounded-lg text-xs bg-[#4F6BED]/10 text-[#4F6BED] font-medium">
                      {providerLabel(m.provider)}
                    </span>
                    {statusBadge(m.status)}
                  </div>
                  <p className="text-xs text-[#5C6472] mt-0.5">
                    <code className="text-xs font-mono">{m.code}</code> · {m.category}
                    {m.context_window > 0 ? ` · ${Math.round(m.context_window / 1000)}k context` : ""}
                  </p>
                  {m.pricings && m.pricings.length > 0 && (
                    <p className="text-xs text-[#5C6472]/70 mt-0.5">
                      {m.pricings.map((p) => `${p.dimension}: ${p.unit_price}`).join(" / ")}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => openEdit(m)} className="p-1.5 text-[#5C6472]/70 hover:text-[#4F6BED] rounded-lg hover:bg-white/70 transition-colors" title="编辑">
                    <Edit size={14} />
                  </button>
                  <button onClick={() => handleDelete(m)} className="p-1.5 text-[#5C6472]/70 hover:text-[#C4372C] rounded-lg hover:bg-white/70 transition-colors" title="下架">
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
