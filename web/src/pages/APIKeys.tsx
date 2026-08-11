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
import { useState, useMemo } from "react";
import { APIKeyData, UsageLog } from "../lib/api";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { setKeySecret } from "../lib/keyMemory";
import { Plus, Copy, Trash2, EyeOff, Key, Shield, AlertTriangle, Clock, ChevronDown, ChevronRight, Ban, Play, Zap, BarChart3, ExternalLink } from "lucide-react";

function formatRelativeTime(isoString: string): string {
  if (!isoString) return "从未使用";
  const now = Date.now();
  const then = new Date(isoString).getTime();
  const diffMs = now - then;
  const diffMinutes = Math.floor(diffMs / 60000);
  if (diffMinutes < 1) return "刚刚";
  if (diffMinutes < 60) return `${diffMinutes} 分钟前`;
  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours < 24) return `${diffHours} 小时前`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays} 天前`;
  if (diffDays < 30) return `${diffDays} 天前`;
  return new Date(isoString).toLocaleDateString("zh-CN");
}

function computeMonthlyProgress(data: APIKeyData): string {
  if (!data.monthly_limit || data.monthly_limit === "0") return "";
  // In a real app this would come from the backend as current_spend per period
  // For now, display the limit
  return `0 / ${data.monthly_limit} CNY`;
}

function computeWeeklyProgress(data: APIKeyData): string {
  if (!data.weekly_limit || data.weekly_limit === "0") return "";
  return `0 / ${data.weekly_limit} CNY`;
}

export default function APIKeys() {
  const {
    data: keyData,
    isLoading,
    isError,
    error,
    refetch,
  } = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const keys = keyData?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  // Fetch usage logs for per-key stats
  const { data: usageData } = useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=500");
  const usageLogs = usageData?.data ?? [];

  const keyUsage = useMemo(() => {
    const m: Record<string, { calls: number; cost: number; tokens: number }> = {};
    for (const l of usageLogs) {
      const kid = l.api_key_id || "";
      if (!kid) continue;
      if (!m[kid]) m[kid] = { calls: 0, cost: 0, tokens: 0 };
      m[kid].calls += 1;
      m[kid].cost += parseFloat(l.cost || "0");
      m[kid].tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
    }
    return m;
  }, [usageLogs]);
  const [showCreate, setShowCreate] = useState(false);
  const [newKey, setNewKey] = useState<{ plaintext?: string; warning?: string } | null>(null);
  const [name, setName] = useState("");
  const [allowedModels, setAllowedModels] = useState("");
  const [sourceWhitelist, setSourceWhitelist] = useState("");
  const [monthlyLimit, setMonthlyLimit] = useState("");
  const [weeklyLimit, setWeeklyLimit] = useState("");
  const [cumulativeLimit, setCumulativeLimit] = useState("");
  const [overLimitAction, setOverLimitAction] = useState("block");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const createMutation = useConsoleMutation<Record<string, unknown>, Record<string, unknown>>(
    "post",
    "/api-keys",
  );
  const toggleMutation = useConsoleMutation<unknown, { id: string; status: string }>(
    "put",
    (v) => `/api-keys/${v.id}`,
    "/api-keys",
  );
  const deleteMutation = useConsoleMutation<unknown, { id: string }>(
    "delete",
    (v) => `/api-keys/${v.id}`,
    "/api-keys",
  );

  const handleCreate = async () => {
    if (!name.trim()) return;
    const body: Record<string, unknown> = { name: name.trim() };

    if (allowedModels.trim()) {
      body.allowed_models = allowedModels.split(",").map((m) => m.trim()).filter(Boolean);
    }
    if (sourceWhitelist.trim()) {
      body.source_whitelist = sourceWhitelist.split(",").map((s) => s.trim()).filter(Boolean);
    }
    if (monthlyLimit.trim()) body.monthly_limit = monthlyLimit.trim();
    if (weeklyLimit.trim()) body.weekly_limit = weeklyLimit.trim();
    if (cumulativeLimit.trim()) body.cumulative_limit = cumulativeLimit.trim();
    body.over_limit_action = overLimitAction;

    const res = await createMutation.mutateAsync(body);
    // Keep the fresh plaintext in MEMORY only (never localStorage) so the
    // Playground can reuse it within this session; a refresh clears it.
    if (res.plaintext && res.id) { setKeySecret(String(res.id), String(res.plaintext)); }
    setNewKey(res);
    setName("");
    setAllowedModels("");
    setSourceWhitelist("");
    setMonthlyLimit("");
    setWeeklyLimit("");
    setCumulativeLimit("");
    setOverLimitAction("block");
    setShowCreate(false);
  };

  const handleToggleStatus = async (key: APIKeyData) => {
    const newStatus = key.status === "active" ? "disabled" : "active";
    try {
      await toggleMutation.mutateAsync({ id: key.id, status: newStatus });
    } catch (err) {
      console.error("Failed to toggle status:", err);
    }
  };

  const handleDelete = async (key: APIKeyData) => {
    if (!confirm("确定要删除该 API 密钥吗？删除后立即失效，使用该密钥的服务将无法访问。")) return;
    try {
      await deleteMutation.mutateAsync({ id: key.id });
    } catch (err) {
      console.error("Failed to delete key:", err);
    }
  };

  const handleCopy = (key: APIKeyData) => {
    navigator.clipboard.writeText(key.id);
  };

  const resetCreateForm = () => {
    setName("");
    setAllowedModels("");
    setSourceWhitelist("");
    setMonthlyLimit("");
    setWeeklyLimit("");
    setCumulativeLimit("");
    setOverLimitAction("block");
    setShowCreate(false);
  };

  const toggleExpand = (id: string) => {
    setExpandedId(expandedId === id ? null : id);
  };

  const statusBadgeClass = (status: string) => {
    switch (status) {
      case "active": return "bg-green-100 text-green-700";
      case "disabled": return "bg-gray-100 text-gray-500";
      case "revoked": return "bg-red-100 text-red-600";
      case "over_limit": return "bg-yellow-100 text-yellow-700";
      default: return "bg-gray-100 text-gray-500";
    }
  };

  const statusLabel = (status: string) => {
    switch (status) {
      case "active": return "启用";
      case "disabled": return "已停用";
      case "revoked": return "已撤销";
      case "over_limit": return "超限";
      default: return status;
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold">API 密钥</h2>
          <p className="text-sm text-gray-500 mt-1">管理 API 密钥，控制模型访问权限与消费额度</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm font-medium">
          <Plus size={16} /> 创建密钥
        </button>
      </div>

      {newKey && (
        <div className="mb-6 p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
          <div className="flex items-center gap-2 mb-2">
            <Key size={16} className="text-yellow-700" />
            <p className="font-medium text-yellow-800">新密钥已创建</p>
          </div>
          <code className="block bg-white px-4 py-2.5 rounded border border-yellow-300 text-sm font-mono mb-2 break-all">{newKey.plaintext}</code>
          <p className="text-yellow-700 text-sm flex items-center gap-1"><EyeOff size={14} /> {newKey.warning || "请立即复制并安全保存，此密钥仅显示一次"}</p>
          <button onClick={() => setNewKey(null)} className="mt-3 text-sm text-primary-600 hover:underline">我已保存，关闭提示</button>
        </div>
      )}

      {showCreate && (
        <div className="mb-6 p-5 bg-white border border-gray-200 rounded-xl">
          <h3 className="font-semibold mb-4">创建新密钥</h3>
          <div className="space-y-4">
            {/* Boundary 1: Identity - Name */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">密钥名称</label>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：生产环境、测试环境" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500" />
            </div>

            {/* Boundary 2: Model boundary - Model whitelist */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                <Shield size={14} className="inline mr-1" />
                模型白名单
              </label>
              <input
                type="text"
                value={allowedModels}
                onChange={(e) => setAllowedModels(e.target.value)}
                placeholder="例如: gpt-4o, claude-sonnet"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
              />
              <p className="text-xs text-gray-400 mt-1">用逗号分隔，留空表示不限制</p>
            </div>

            {/* Boundary 3: Source boundary - IP whitelist */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                <Shield size={14} className="inline mr-1" />
                IP 白名单
              </label>
              <textarea
                value={sourceWhitelist}
                onChange={(e) => setSourceWhitelist(e.target.value)}
                placeholder="例如: 192.168.1.0/24, 10.0.0.0/8"
                rows={2}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500 resize-none"
              />
              <p className="text-xs text-gray-400 mt-1">支持 CIDR 格式，用逗号分隔，留空表示不限制</p>
            </div>

            {/* Boundary 4: Budget boundary - Spend limits */}
            <div className="border-t border-gray-100 pt-4">
              <h4 className="text-sm font-medium text-gray-700 mb-3 flex items-center gap-1">
                <AlertTriangle size={14} />
                消费限额
              </h4>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">月度限额 (CNY/月)</label>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={monthlyLimit}
                    onChange={(e) => setMonthlyLimit(e.target.value)}
                    placeholder="月度消费上限"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">周限额 (CNY/周)</label>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={weeklyLimit}
                    onChange={(e) => setWeeklyLimit(e.target.value)}
                    placeholder="周消费上限"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">累计限额 (CNY 总计)</label>
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={cumulativeLimit}
                    onChange={(e) => setCumulativeLimit(e.target.value)}
                    placeholder="累计消费上限"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
                  />
                </div>
              </div>
            </div>

            {/* Over-limit action */}
            <div className="border-t border-gray-100 pt-4">
              <label className="block text-sm font-medium text-gray-700 mb-2">超限动作</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="over_limit_action"
                    value="block"
                    checked={overLimitAction === "block"}
                    onChange={(e) => setOverLimitAction(e.target.value)}
                    className="text-primary-600 focus:ring-primary-500"
                    aria-label="阻止 (block)"
                  />
                  <span className="text-sm text-gray-700">阻止 (block)</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="over_limit_action"
                    value="warn"
                    checked={overLimitAction === "warn"}
                    onChange={(e) => setOverLimitAction(e.target.value)}
                    className="text-primary-600 focus:ring-primary-500"
                    aria-label="警告 (warn)"
                  />
                  <span className="text-sm text-gray-700">警告 (warn)</span>
                </label>
              </div>
            </div>
          </div>
          <div className="flex gap-3 mt-4">
            <button onClick={handleCreate} className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 text-sm">确认创建</button>
            <button onClick={resetCreateForm} className="px-4 py-2 border border-gray-300 rounded-lg text-sm text-gray-600 hover:bg-gray-50">取消</button>
          </div>
        </div>
      )}

      {loadError && (
        <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
          <p className="font-medium">加载失败</p>
          <p className="mt-1">{loadError}</p>
          <button onClick={() => refetch()} className="mt-2 px-3 py-1 bg-red-600 text-white rounded text-xs">
            重试
          </button>
        </div>
      )}
      {isLoading && (
        <div className="p-12 text-center bg-white rounded-xl border">
          <div className="animate-spin w-8 h-8 border-2 border-primary-600 border-t-transparent rounded-full mx-auto mb-3" />
          <p className="text-gray-500">加载 API 密钥...</p>
        </div>
      )}
      <div className="bg-white rounded-xl border border-gray-200">
        {!isLoading && keys.length === 0 && (
          <div className="p-12 text-center text-gray-400">
            <Key size={40} className="mx-auto mb-3 opacity-30" />
            <p>暂无 API 密钥</p>
            <p className="text-sm mt-1">点击上方按钮创建第一个密钥</p>
          </div>
        )}
        {keys.map((key) => {
          const isExpanded = expandedId === key.id;
          const modelCount = key.allowed_models?.length || 0;
          const hasIpWhitelist = key.source_whitelist && key.source_whitelist.length > 0;
          const canToggle = key.status === "active" || key.status === "disabled";

          return (
            <div key={key.id} className="border-b border-gray-100 last:border-b-0">
              <div
                className="p-4 flex items-center justify-between hover:bg-gray-50 cursor-pointer"
                onClick={() => toggleExpand(key.id)}
              >
                <div className="flex items-start gap-3 min-w-0 flex-1">
                  {/* Expand toggle + 7d active indicator */}
                  <div className="flex items-center gap-1.5 pt-0.5">
                    {isExpanded ? <ChevronDown size={14} className="text-gray-400" /> : <ChevronRight size={14} className="text-gray-400" />}
                    {key.last_7d_active && (
                      <span className="inline-block w-2 h-2 rounded-full bg-green-500" title="最近 7 天活跃" />
                    )}
                  </div>

                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <p className="font-medium text-sm truncate">{key.name}</p>
                      <span className={`px-2 py-0.5 rounded text-xs ${statusBadgeClass(key.status)}`}>
                        {statusLabel(key.status)}
                      </span>
                    </div>
                    <code className="text-xs text-gray-500 font-mono">{key.masked_key}</code>

                    {/* Per-key usage stats */}
                    {(() => { const u = keyUsage[key.id]; if (!u) return null; return (
                      <div className="flex items-center gap-3 mt-1 text-xs text-gray-500">
                        <span className="flex items-center gap-1"><Zap size={10} />{u.calls} 次调用</span>
                        <span className="flex items-center gap-1"><BarChart3 size={10} />{u.tokens.toLocaleString()} tokens</span>
                        <span className="font-mono text-indigo-600">{formatAmount(u.cost)} CNY</span>
                      </div>); })()}

                    {/* Boundary indicators row */}
                    <div className="flex items-center gap-3 mt-1.5 text-xs text-gray-500 flex-wrap">
                      {/* Model boundary */}
                      <span title={key.allowed_models?.join(", ") || ""}>
                        {modelCount > 0 ? `${modelCount} models` : "未限制"}
                      </span>

                      {/* Source boundary */}
                      <span className="flex items-center gap-1">
                        <Shield size={10} />
                        {hasIpWhitelist ? "已配置" : "未限制"}
                      </span>

                      {/* Budget boundary */}
                      {key.monthly_limit && key.monthly_limit !== "0" && (
                        <span>{key.monthly_limit} CNY/月</span>
                      )}

                      {/* Evidence boundary */}
                      <span className="flex items-center gap-1">
                        <Clock size={10} />
                        最后使用: {formatRelativeTime(key.last_used_at || "")}
                      </span>

                      <a
                        href={`/logs?key_id=${key.id}`}
                        className="flex items-center gap-1 text-primary-600 hover:underline"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <ExternalLink size={10} />
                        查看日志
                      </a>
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 ml-3" onClick={(e) => e.stopPropagation()}>
                  {canToggle && (
                    <button
                      className="p-1.5 text-gray-400 hover:text-gray-600 rounded"
                      title={key.status === "active" ? "停用" : "启用"}
                      onClick={() => handleToggleStatus(key)}
                      aria-label={key.status === "active" ? "停用" : "启用"}
                    >
                      {key.status === "active" ? <Ban size={14} /> : <Play size={14} />}
                    </button>
                  )}
                  <button className="p-1.5 text-gray-400 hover:text-gray-600 rounded" title="复制" onClick={() => handleCopy(key)}>
                    <Copy size={14} />
                  </button>
                  <button className="p-1.5 text-gray-400 hover:text-red-600 rounded" title="删除" onClick={() => handleDelete(key)} aria-label="删除">
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>

              {/* Expanded detail view */}
              {isExpanded && (
                <div className="px-4 pb-4 pt-0 border-t border-gray-50 bg-gray-50/50">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-4">
                    {/* Identity */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2 flex items-center gap-1">
                        <Key size={12} /> 身份边界
                      </h4>
                      <p className="text-sm font-medium">{key.name}</p>
                      <p className="text-xs text-gray-400 mt-1">ID: {key.id}</p>
                      <p className="text-xs text-gray-400">创建于 {new Date(key.created_at).toLocaleDateString("zh-CN")}</p>
                    </div>

                    {/* Model boundary */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2">模型边界</h4>
                      {modelCount > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {key.allowed_models!.map((m) => (
                            <span key={m} className="px-2 py-0.5 bg-indigo-50 text-indigo-700 rounded text-xs font-mono">{m}</span>
                          ))}
                        </div>
                      ) : (
                        <p className="text-xs text-gray-400">未限制 (所有模型可用)</p>
                      )}
                    </div>

                    {/* Source boundary */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2 flex items-center gap-1">
                        <Shield size={12} /> 来源边界
                      </h4>
                      {hasIpWhitelist ? (
                        <div className="flex flex-wrap gap-1">
                          {key.source_whitelist!.map((ip) => (
                            <span key={ip} className="px-2 py-0.5 bg-teal-50 text-teal-700 rounded text-xs font-mono">{ip}</span>
                          ))}
                        </div>
                      ) : (
                        <p className="text-xs text-gray-400">未限制 (允许所有 IP)</p>
                      )}
                    </div>

                    {/* Budget boundary */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2 flex items-center gap-1">
                        <AlertTriangle size={12} /> 预算边界
                      </h4>
                      <div className="space-y-1.5 text-xs">
                        <div className="flex justify-between">
                          <span className="text-gray-500">月度限额:</span>
                          <span className="font-mono">
                            {key.monthly_limit && key.monthly_limit !== "0"
                              ? computeMonthlyProgress(key)
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-gray-500">周限额:</span>
                          <span className="font-mono">
                            {key.weekly_limit && key.weekly_limit !== "0"
                              ? computeWeeklyProgress(key)
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-gray-500">累计限额:</span>
                          <span className="font-mono">
                            {key.cumulative_limit && key.cumulative_limit !== "0"
                              ? `${key.cumulative_limit} CNY`
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-gray-500">超限动作:</span>
                          <span className={`font-medium ${key.over_limit_action === "block" ? "text-red-600" : "text-yellow-600"}`}>
                            {key.over_limit_action === "block" ? "阻止" : "警告"}
                          </span>
                        </div>
                      </div>
                    </div>

                    {/* Status boundary */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2">状态边界</h4>
                      <div className="flex items-center gap-2">
                        <span className={`px-2 py-0.5 rounded text-xs ${statusBadgeClass(key.status)}`}>
                          {statusLabel(key.status)}
                        </span>
                        {canToggle && (
                          <button
                            onClick={() => handleToggleStatus(key)}
                            className="text-xs text-primary-600 hover:underline"
                          >
                            {key.status === "active" ? "停用" : "启用"}
                          </button>
                        )}
                      </div>
                    </div>

                    {/* Evidence boundary */}
                    <div className="bg-white p-3 rounded-lg border border-gray-200">
                      <h4 className="text-xs font-semibold text-gray-500 uppercase mb-2 flex items-center gap-1">
                        <Clock size={12} /> 证据边界
                      </h4>
                      <div className="space-y-1.5 text-xs">
                        <div className="flex items-center gap-2">
                          <span className="text-gray-500">最后使用:</span>
                          <span>{formatRelativeTime(key.last_used_at || "")}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-gray-500">7天活跃:</span>
                          {key.last_7d_active ? (
                            <span className="flex items-center gap-1 text-green-600">
                              <span className="inline-block w-1.5 h-1.5 rounded-full bg-green-500" />
                              活跃
                            </span>
                          ) : (
                            <span className="text-gray-400">不活跃</span>
                          )}
                        </div>
                        <a
                          href={`/logs?key_id=${key.id}`}
                          className="flex items-center gap-1 text-primary-600 hover:underline mt-1"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <ExternalLink size={10} />
                          查看日志
                        </a>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
