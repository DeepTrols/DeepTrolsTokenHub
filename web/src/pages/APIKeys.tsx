import { LoadingState } from "@/components/StateViews";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
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

const statusVariant = (status: string) => {
  switch (status) {
    case "active": return "success";
    case "disabled": return "secondary";
    case "revoked": return "destructive";
    case "over_limit": return "warning";
    default: return "secondary";
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

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">API 密钥</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">管理 API 密钥，控制模型访问权限与消费额度</p>
        </div>
        <Button onClick={() => setShowCreate(true)}><Plus size={16} className="mr-1.5" />创建密钥</Button>
      </div>

      {newKey && (
        <div className="mb-6 p-4 glass-soft rounded-xl border-[#D3A94E]/40">
          <div className="flex items-center gap-2 mb-2">
            <Key size={16} className="text-[#A06B12]" />
            <p className="font-medium text-[#A06B12]">新密钥已创建</p>
          </div>
          <code className="block bg-white/70 px-4 py-2.5 rounded-lg border border-[#D3A94E]/40 text-sm font-mono mb-2 break-all">{newKey.plaintext}</code>
          <p className="text-[#A06B12] text-sm flex items-center gap-1"><EyeOff size={14} /> {newKey.warning || "请立即复制并安全保存，此密钥仅显示一次"}</p>
          <button onClick={() => setNewKey(null)} className="mt-3 text-sm text-[#4F6BED] hover:underline">我已保存，关闭提示</button>
        </div>
      )}

      {showCreate && (
        <div className="mb-6 p-5 glass rounded-[22px]">
          <h3 className="font-display font-semibold mb-4">创建新密钥</h3>
          <div className="space-y-4">
            {/* Boundary 1: Identity - Name */}
            <div>
              <Label htmlFor="key-name" className="text-[12px] font-semibold text-[#5C6472]">密钥名称</Label>
              <Input id="key-name" type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：生产环境、测试环境" />
            </div>

            {/* Boundary 2: Model boundary - Model whitelist */}
            <div>
              <Label htmlFor="key-models" className="text-[12px] font-semibold text-[#5C6472]"><Shield size={14} className="inline mr-1" />模型白名单</Label>
              <Input
                id="key-models"
                type="text"
                value={allowedModels}
                onChange={(e) => setAllowedModels(e.target.value)}
                placeholder="例如: gpt-4o, claude-sonnet"
                className="font-mono"
              />
              <p className="text-xs text-[#5C6472]/80 mt-1">用逗号分隔，留空表示不限制</p>
            </div>

            {/* Boundary 3: Source boundary - IP whitelist */}
            <div>
              <Label htmlFor="key-ips" className="text-[12px] font-semibold text-[#5C6472]"><Shield size={14} className="inline mr-1" />IP 白名单</Label>
              <textarea
                id="key-ips"
                value={sourceWhitelist}
                onChange={(e) => setSourceWhitelist(e.target.value)}
                placeholder="例如: 192.168.1.0/24, 10.0.0.0/8"
                rows={2}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-[#4F6BED]/25 resize-none"
              />
              <p className="text-xs text-[#5C6472]/80 mt-1">支持 CIDR 格式，用逗号分隔，留空表示不限制</p>
            </div>

            {/* Boundary 4: Budget boundary - Spend limits */}
            <div className="border-t border-black/[0.06] pt-4">
              <h4 className="text-sm font-medium text-[#161A23] mb-3 flex items-center gap-1">
                <AlertTriangle size={14} className="text-[#D3A94E]" />
                消费限额
              </h4>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <Label htmlFor="key-monthly" className="text-xs font-medium text-[#5C6472] mb-1 block">月度限额 (CNY/月)</Label>
                  <Input
                    id="key-monthly"
                    type="number"
                    min="0"
                    step="0.01"
                    value={monthlyLimit}
                    onChange={(e) => setMonthlyLimit(e.target.value)}
                    placeholder="月度消费上限"
                  />
                </div>
                <div>
                  <Label htmlFor="key-weekly" className="text-xs font-medium text-[#5C6472] mb-1 block">周限额 (CNY/周)</Label>
                  <Input
                    id="key-weekly"
                    type="number"
                    min="0"
                    step="0.01"
                    value={weeklyLimit}
                    onChange={(e) => setWeeklyLimit(e.target.value)}
                    placeholder="周消费上限"
                  />
                </div>
                <div>
                  <Label htmlFor="key-cumulative" className="text-xs font-medium text-[#5C6472] mb-1 block">累计限额 (CNY 总计)</Label>
                  <Input
                    id="key-cumulative"
                    type="number"
                    min="0"
                    step="0.01"
                    value={cumulativeLimit}
                    onChange={(e) => setCumulativeLimit(e.target.value)}
                    placeholder="累计消费上限"
                  />
                </div>
              </div>
            </div>

            {/* Over-limit action */}
            <div className="border-t border-black/[0.06] pt-4">
              <span className="block text-sm font-medium text-[#161A23] mb-2">超限动作</span>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="over_limit_action"
                    value="block"
                    checked={overLimitAction === "block"}
                    onChange={(e) => setOverLimitAction(e.target.value)}
                    className="accent-[#4F6BED]"
                    aria-label="阻止 (block)"
                  />
                  <span className="text-sm text-[#161A23]">阻止 (block)</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="over_limit_action"
                    value="warn"
                    checked={overLimitAction === "warn"}
                    onChange={(e) => setOverLimitAction(e.target.value)}
                    className="accent-[#4F6BED]"
                    aria-label="警告 (warn)"
                  />
                  <span className="text-sm text-[#161A23]">警告 (warn)</span>
                </label>
              </div>
            </div>
          </div>
          <div className="flex gap-3 mt-4">
            <Button onClick={handleCreate}>确认创建</Button>
            <Button variant="outline" onClick={resetCreateForm}>取消</Button>
          </div>
        </div>
      )}

      {loadError && (
        <div className="mb-4 p-4 glass-soft rounded-xl border-[#E5484D]/25 text-sm text-[#C4372C]">
          <p className="font-medium">加载失败</p>
          <p className="mt-1">{loadError}</p>
          <Button variant="destructive" size="sm" onClick={() => refetch()} className="mt-2">重试</Button>
        </div>
      )}
      {isLoading && <LoadingState message="加载 API 密钥..." />}
      <div className="glass rounded-[22px] overflow-hidden">
        {!isLoading && keys.length === 0 && (
          <div className="p-12 text-center">
            <Key size={40} className="mx-auto mb-3 text-[#5C6472]/30" />
            <p className="text-[#5C6472] font-medium">暂无 API 密钥</p>
            <p className="text-sm mt-1 text-[#5C6472]/80">点击上方按钮创建第一个密钥</p>
          </div>
        )}
        {keys.map((key) => {
          const isExpanded = expandedId === key.id;
          const modelCount = key.allowed_models?.length || 0;
          const hasIpWhitelist = key.source_whitelist && key.source_whitelist.length > 0;
          const canToggle = key.status === "active" || key.status === "disabled";

          return (
            <div key={key.id} className="border-b border-black/[0.06] last:border-b-0">
              <div
                className="p-4 flex items-center justify-between hover:bg-white/60 cursor-pointer transition-colors"
                onClick={() => toggleExpand(key.id)}
              >
                <div className="flex items-start gap-3 min-w-0 flex-1">
                  {/* Expand toggle + 7d active indicator */}
                  <div className="flex items-center gap-1.5 pt-0.5">
                    {isExpanded ? <ChevronDown size={14} className="text-[#5C6472]/60" /> : <ChevronRight size={14} className="text-[#5C6472]/60" />}
                    {key.last_7d_active && (
                      <span className="inline-block w-2 h-2 rounded-full bg-[#1BA878] shadow-[0_0_8px_#1BA878]" title="最近 7 天活跃" />
                    )}
                  </div>

                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <p className="font-medium text-sm truncate">{key.name}</p>
                      <Badge variant={statusVariant(key.status) as "success" | "secondary" | "destructive" | "warning"}>{statusLabel(key.status)}</Badge>
                    </div>
                    <code className="text-xs text-[#5C6472] font-mono">{key.masked_key}</code>

                    {/* Per-key usage stats */}
                    {(() => { const u = keyUsage[key.id]; if (!u) return null; return (
                      <div className="flex items-center gap-3 mt-1 text-xs text-[#5C6472]">
                        <span className="flex items-center gap-1"><Zap size={10} />{u.calls} 次调用</span>
                        <span className="flex items-center gap-1"><BarChart3 size={10} />{u.tokens.toLocaleString()} tokens</span>
                        <span className="font-mono text-[#4F6BED]">{formatAmount(u.cost)} CNY</span>
                      </div>); })()}

                    {/* Boundary indicators row */}
                    <div className="flex items-center gap-3 mt-1.5 text-xs text-[#5C6472] flex-wrap">
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
                        className="flex items-center gap-1 text-[#4F6BED] hover:underline"
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
                      className="p-1.5 text-[#5C6472]/70 hover:text-[#161A23] hover:bg-white/70 rounded-lg transition-colors"
                      title={key.status === "active" ? "停用" : "启用"}
                      onClick={() => handleToggleStatus(key)}
                      aria-label={key.status === "active" ? "停用" : "启用"}
                    >
                      {key.status === "active" ? <Ban size={14} /> : <Play size={14} />}
                    </button>
                  )}
                  <button className="p-1.5 text-[#5C6472]/70 hover:text-[#161A23] hover:bg-white/70 rounded-lg transition-colors" title="复制" onClick={() => handleCopy(key)}>
                    <Copy size={14} />
                  </button>
                  <button className="p-1.5 text-[#5C6472]/70 hover:text-[#C4372C] hover:bg-white/70 rounded-lg transition-colors" title="删除" onClick={() => handleDelete(key)} aria-label="删除">
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>

              {/* Expanded detail view */}
              {isExpanded && (
                <div className="px-4 pb-4 pt-0 border-t border-black/[0.06] bg-white/40">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-4">
                    {/* Identity */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2 flex items-center gap-1">
                        <Key size={12} /> 身份边界
                      </h4>
                      <p className="text-sm font-medium">{key.name}</p>
                      <p className="text-xs text-[#5C6472]/80 mt-1 font-mono">ID: {key.id}</p>
                      <p className="text-xs text-[#5C6472]/80">创建于 {new Date(key.created_at).toLocaleDateString("zh-CN")}</p>
                    </div>

                    {/* Model boundary */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2">模型边界</h4>
                      {modelCount > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {key.allowed_models!.map((m) => (
                            <span key={m} className="px-2 py-0.5 bg-[#4F6BED]/10 text-[#4F6BED] rounded-md text-xs font-mono">{m}</span>
                          ))}
                        </div>
                      ) : (
                        <p className="text-xs text-[#5C6472]/80">未限制 (所有模型可用)</p>
                      )}
                    </div>

                    {/* Source boundary */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2 flex items-center gap-1">
                        <Shield size={12} /> 来源边界
                      </h4>
                      {hasIpWhitelist ? (
                        <div className="flex flex-wrap gap-1">
                          {key.source_whitelist!.map((ip) => (
                            <span key={ip} className="px-2 py-0.5 bg-[#0FA88B]/10 text-[#0FA88B] rounded-md text-xs font-mono">{ip}</span>
                          ))}
                        </div>
                      ) : (
                        <p className="text-xs text-[#5C6472]/80">未限制 (允许所有 IP)</p>
                      )}
                    </div>

                    {/* Budget boundary */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2 flex items-center gap-1">
                        <AlertTriangle size={12} className="text-[#D3A94E]" /> 预算边界
                      </h4>
                      <div className="space-y-1.5 text-xs">
                        <div className="flex justify-between">
                          <span className="text-[#5C6472]">月度限额:</span>
                          <span className="font-mono">
                            {key.monthly_limit && key.monthly_limit !== "0"
                              ? computeMonthlyProgress(key)
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-[#5C6472]">周限额:</span>
                          <span className="font-mono">
                            {key.weekly_limit && key.weekly_limit !== "0"
                              ? computeWeeklyProgress(key)
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-[#5C6472]">累计限额:</span>
                          <span className="font-mono">
                            {key.cumulative_limit && key.cumulative_limit !== "0"
                              ? `${key.cumulative_limit} CNY`
                              : "未限制"}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-[#5C6472]">超限动作:</span>
                          <span className={`font-medium ${key.over_limit_action === "block" ? "text-[#C4372C]" : "text-[#A06B12]"}`}>
                            {key.over_limit_action === "block" ? "阻止" : "警告"}
                          </span>
                        </div>
                      </div>
                    </div>

                    {/* Status boundary */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2">状态边界</h4>
                      <div className="flex items-center gap-2">
                        <Badge variant={statusVariant(key.status) as "success" | "secondary" | "destructive" | "warning"}>{statusLabel(key.status)}</Badge>
                        {canToggle && (
                          <button
                            onClick={() => handleToggleStatus(key)}
                            className="text-xs text-[#4F6BED] hover:underline"
                          >
                            {key.status === "active" ? "停用" : "启用"}
                          </button>
                        )}
                      </div>
                    </div>

                    {/* Evidence boundary */}
                    <div className="glass-soft rounded-xl p-3">
                      <h4 className="text-xs font-semibold text-[#5C6472] uppercase tracking-wider mb-2 flex items-center gap-1">
                        <Clock size={12} /> 证据边界
                      </h4>
                      <div className="space-y-1.5 text-xs">
                        <div className="flex items-center gap-2">
                          <span className="text-[#5C6472]">最后使用:</span>
                          <span>{formatRelativeTime(key.last_used_at || "")}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-[#5C6472]">7天活跃:</span>
                          {key.last_7d_active ? (
                            <span className="flex items-center gap-1 text-[#0C7A55]">
                              <span className="inline-block w-1.5 h-1.5 rounded-full bg-[#1BA878] shadow-[0_0_6px_#1BA878]" />
                              活跃
                            </span>
                          ) : (
                            <span className="text-[#5C6472]/80">不活跃</span>
                          )}
                        </div>
                        <a
                          href={`/logs?key_id=${key.id}`}
                          className="flex items-center gap-1 text-[#4F6BED] hover:underline mt-1"
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
