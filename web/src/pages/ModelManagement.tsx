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
      case "active": return <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-700">上架</span>;
      case "inactive": return <span className="px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-500">已下架</span>;
      case "beta": return <span className="px-2 py-0.5 rounded text-xs bg-blue-100 text-blue-700">测试</span>;
      default: return <span className="px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-500">{s || "active"}</span>;
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
        <h2 className="text-2xl font-bold mb-6">模型管理</h2>
        <div className="p-12 text-center bg-white rounded-xl border">
          <div className="animate-spin w-8 h-8 border-2 border-primary-600 border-t-transparent rounded-full mx-auto mb-3" />
          <p className="text-gray-500">加载模型中...</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold">模型管理</h2>
          <p className="text-sm text-gray-500 mt-1">管理模型目录，配置定价维度</p>
        </div>
        <button onClick={openCreate} className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm font-medium">
          <Plus size={16} /> 添加模型
        </button>
      </div>

      {loadError && (
        <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
          <p className="font-medium">加载失败</p>
          <p className="mt-1">{loadError}</p>
          <button onClick={() => refetch()} className="mt-2 px-3 py-1 bg-red-600 text-white rounded text-xs">重试</button>
        </div>
      )}

      {showForm && (
        <div className="mb-6 p-6 bg-white border border-gray-200 rounded-xl">
          <h3 className="font-semibold mb-4">{editing ? "编辑模型" : "添加模型"}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">编码 *</label>
              <input type="text" value={code} onChange={(e) => setCode(e.target.value)} placeholder="例如: gpt-4o"
                disabled={!!editing}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm disabled:bg-gray-100" />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">提供商 *</label>
              <select value={provider} onChange={(e) => setProvider(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm">
                <option value="">选择提供商...</option>
                {PROVIDER_OPTIONS.map((o) => (
                  <option key={o.v} value={o.v}>{o.l}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">类别</label>
              <select value={category} onChange={(e) => setCategory(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm">
                <option value="chat">对话</option>
                <option value="embedding">嵌入</option>
                <option value="image">图片</option>
                <option value="audio">音频</option>
                <option value="video">视频</option>
                <option value="rerank">重排序</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">显示名称</label>
              <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="例如: GPT-4o"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">上下文窗口</label>
              <input type="number" value={contextWindow} onChange={(e) => setContextWindow(Number(e.target.value))}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
            </div>
          </div>

          <div className="mt-6">
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-gray-700">定价维度</label>
              <button onClick={addPricing} className="text-sm text-primary-600 hover:underline">+ 添加维度</button>
            </div>
            {pricings.map((p, idx) => (
              <div key={idx} className="flex gap-3 mb-2 items-start">
                <select value={p.dimension} onChange={(e) => updatePricing(idx, "dimension", e.target.value)}
                  className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-32">
                  <option value="">选择维度</option>
                  {DIMENSIONS.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
                <input type="text" value={p.unit_name} onChange={(e) => updatePricing(idx, "unit_name", e.target.value)}
                  placeholder="单位" className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-20" />
                <input type="text" value={p.unit_price} onChange={(e) => updatePricing(idx, "unit_price", e.target.value)}
                  placeholder="单价" className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-32 font-mono" />
                <button onClick={() => removePricing(idx)} className="p-2 text-gray-400 hover:text-red-500" title="移除">
                  <Trash2 size={16} />
                </button>
              </div>
            ))}
          </div>

          <div className="flex gap-3 mt-6">
            <button onClick={handleSubmit} className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm">
              {editing ? "保存更改" : "确认创建"}
            </button>
            <button onClick={() => { setShowForm(false); resetForm(); }} className="px-4 py-2 border border-gray-300 rounded-lg text-sm text-gray-600 hover:bg-gray-50">
              取消
            </button>
          </div>
        </div>
      )}

      <div className="space-y-6">
        {models.length === 0 && (
          <div className="p-12 text-center text-gray-400 bg-white rounded-xl border border-gray-200">
            <Box size={40} className="mx-auto mb-3 opacity-30" />
            <p>暂无模型</p>
            <p className="text-sm mt-1">点击上方按钮添加第一个模型</p>
          </div>
        )}
        {sortedGroups.map(([pv, pvModels]) => {
          const isCollapsed = collapsed.has(pv);
          return (
          <div key={pv} className="bg-white rounded-xl border border-gray-200 overflow-hidden">
            <button
              onClick={() => toggleGroup(pv)}
              className="w-full px-5 py-3 bg-gray-50 border-b border-gray-100 flex items-center gap-3 hover:bg-gray-100 transition-colors"
            >
              {isCollapsed ? <ChevronRight size={16} className="text-gray-400" /> : <ChevronDown size={16} className="text-gray-400" />}
              <h3 className="font-semibold text-sm text-gray-700">
                {providerLabel(pv) || pv}
              </h3>
              <span className="text-xs text-gray-400">{pvModels.length} 个模型</span>
            </button>
            {!isCollapsed && pvModels.map((m) => (
              <div key={m.id} className="px-5 py-4 border-b border-gray-50 last:border-b-0 flex items-center justify-between hover:bg-gray-50/50">
                <div>
                  <div className="flex items-center gap-2">
                    <p className="font-medium text-sm">{m.display_name || m.code}</p>
                    <span className="px-1.5 py-0.5 rounded text-xs bg-primary-50 text-primary-700 font-medium">
                      {providerLabel(m.provider)}
                    </span>
                    {statusBadge(m.status)}
                  </div>
                  <p className="text-xs text-gray-500 mt-0.5">
                    <code className="text-xs font-mono">{m.code}</code> · {m.category}
                    {m.context_window > 0 ? ` · ${Math.round(m.context_window / 1000)}k context` : ""}
                  </p>
                  {m.pricings && m.pricings.length > 0 && (
                    <p className="text-xs text-gray-400 mt-0.5">
                      {m.pricings.map((p) => `${p.dimension}: ${p.unit_price}`).join(" / ")}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => openEdit(m)} className="p-1.5 text-gray-400 hover:text-primary-600 rounded" title="编辑">
                    <Edit size={14} />
                  </button>
                  <button onClick={() => handleDelete(m)} className="p-1.5 text-gray-400 hover:text-red-600 rounded" title="下架">
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
