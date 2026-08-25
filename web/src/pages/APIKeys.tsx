import { useState } from "react";
import { APIKeyData } from "../lib/api";
import { api } from "../lib/api";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { setKeySecret } from "../lib/keyMemory";
import { buildKeyLimitsBody, formatRateLimit } from "../lib/domain/apiKey";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Copy, Plus } from "lucide-react";

const GMT8_MS = 8 * 60 * 60 * 1000;

// Dates are shown in GMT+8 (the platform's billing/usage timezone).
export function gmt8DateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const g = new Date(d.getTime() + GMT8_MS);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${g.getUTCFullYear()}-${p(g.getUTCMonth() + 1)}-${p(g.getUTCDate())} ${p(g.getUTCHours())}:${p(g.getUTCMinutes())}:${p(g.getUTCSeconds())}`;
}

interface KeyFormState {
  name: string;
  allowedModels: string;
  sourceWhitelist: string;
  monthlyLimit: string;
  weeklyLimit: string;
  cumulativeLimit: string;
  rateLimitRpm: string;
  rateLimitTpm: string;
  overLimitAction: string;
  status: string;
}

const emptyForm = (): KeyFormState => ({
  name: "",
  allowedModels: "",
  sourceWhitelist: "",
  monthlyLimit: "",
  weeklyLimit: "",
  cumulativeLimit: "",
  rateLimitRpm: "",
  rateLimitTpm: "",
  overLimitAction: "block",
  status: "active",
});

const formFromKey = (k: APIKeyData): KeyFormState => ({
  name: k.name || "",
  allowedModels: (k.allowed_models || []).join(", "),
  sourceWhitelist: (k.source_whitelist || []).join(", "),
  monthlyLimit: k.monthly_limit || "",
  weeklyLimit: k.weekly_limit || "",
  cumulativeLimit: k.cumulative_limit || "",
  rateLimitRpm: k.rate_limit_rpm ? String(k.rate_limit_rpm) : "",
  rateLimitTpm: k.rate_limit_tpm ? String(k.rate_limit_tpm) : "",
  overLimitAction: k.over_limit_action || "block",
  status: k.status || "active",
});

function KeyFields({
  form,
  onChange,
  showStatus,
}: {
  form: KeyFormState;
  onChange: (f: KeyFormState) => void;
  showStatus?: boolean;
}) {
  const set = (patch: Partial<KeyFormState>) => onChange({ ...form, ...patch });
  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="key-name" className="text-[12px] font-semibold text-[#5C6472]">
          密钥名称
        </Label>
        <Input
          id="key-name"
          type="text"
          value={form.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="例如：生产环境、测试环境"
        />
      </div>
      <div>
        <Label htmlFor="key-models" className="text-[12px] font-semibold text-[#5C6472]">
          模型白名单
        </Label>
        <Input
          id="key-models"
          type="text"
          value={form.allowedModels}
          onChange={(e) => set({ allowedModels: e.target.value })}
          placeholder="例如: gpt-4o, deepseek-v4-flash"
          className="font-mono"
        />
        <p className="text-xs text-[#5C6472]/80 mt-1">用逗号分隔，留空表示不限制</p>
      </div>
      <div>
        <Label htmlFor="key-ips" className="text-[12px] font-semibold text-[#5C6472]">
          IP 白名单
        </Label>
        <Input
          id="key-ips"
          type="text"
          value={form.sourceWhitelist}
          onChange={(e) => set({ sourceWhitelist: e.target.value })}
          placeholder="例如: 192.168.1.0/24, 10.0.0.0/8"
          className="font-mono"
        />
        <p className="text-xs text-[#5C6472]/80 mt-1">支持 CIDR 格式，用逗号分隔，留空表示不限制</p>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <Label htmlFor="key-monthly" className="text-xs font-medium text-[#5C6472] mb-1 block">
            月度限额 (CNY/月)
          </Label>
          <Input
            id="key-monthly"
            type="number"
            min="0"
            step="0.01"
            value={form.monthlyLimit}
            onChange={(e) => set({ monthlyLimit: e.target.value })}
            placeholder="月度消费上限"
          />
        </div>
        <div>
          <Label htmlFor="key-weekly" className="text-xs font-medium text-[#5C6472] mb-1 block">
            周限额 (CNY/周)
          </Label>
          <Input
            id="key-weekly"
            type="number"
            min="0"
            step="0.01"
            value={form.weeklyLimit}
            onChange={(e) => set({ weeklyLimit: e.target.value })}
            placeholder="周消费上限"
          />
        </div>
        <div>
          <Label htmlFor="key-cumulative" className="text-xs font-medium text-[#5C6472] mb-1 block">
            累计限额 (CNY 总计)
          </Label>
          <Input
            id="key-cumulative"
            type="number"
            min="0"
            step="0.01"
            value={form.cumulativeLimit}
            onChange={(e) => set({ cumulativeLimit: e.target.value })}
            placeholder="累计消费上限"
          />
        </div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <Label htmlFor="key-rpm" className="text-xs font-medium text-[#5C6472] mb-1 block">
            RPM 限流 (请求/分钟)
          </Label>
          <Input
            id="key-rpm"
            type="number"
            min="0"
            step="1"
            value={form.rateLimitRpm}
            onChange={(e) => set({ rateLimitRpm: e.target.value })}
            placeholder="每分钟请求上限，0 或留空=不限"
          />
        </div>
        <div>
          <Label htmlFor="key-tpm" className="text-xs font-medium text-[#5C6472] mb-1 block">
            TPM 限流 (Token/分钟)
          </Label>
          <Input
            id="key-tpm"
            type="number"
            min="0"
            step="1"
            value={form.rateLimitTpm}
            onChange={(e) => set({ rateLimitTpm: e.target.value })}
            placeholder="每分钟 Token 上限，0 或留空=不限"
          />
        </div>
      </div>
      <p className="text-xs text-[#5C6472]/80">RPM/TPM 按分钟桶执行，超限返回 429 并附带 X-RateLimit-* 响应头</p>
      <div>
        <span className="block text-sm font-medium text-[#161A23] mb-2">超限动作</span>
        <div className="flex gap-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="over_limit_action"
              value="block"
              checked={form.overLimitAction === "block"}
              onChange={(e) => set({ overLimitAction: e.target.value })}
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
              checked={form.overLimitAction === "warn"}
              onChange={(e) => set({ overLimitAction: e.target.value })}
              className="accent-[#4F6BED]"
              aria-label="警告 (warn)"
            />
            <span className="text-sm text-[#161A23]">警告 (warn)</span>
          </label>
        </div>
      </div>
      {showStatus && (
        <div>
          <Label htmlFor="key-status" className="text-[12px] font-semibold text-[#5C6472]">
            状态
          </Label>
          <select
            id="key-status"
            value={form.status}
            onChange={(e) => set({ status: e.target.value })}
            className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-[#4F6BED]/25"
          >
            <option value="active">启用</option>
            <option value="disabled">停用</option>
          </select>
        </div>
      )}
    </div>
  );
}

const blackButton =
  "rounded-lg bg-neutral-900 text-white text-sm font-semibold px-4 py-2 hover:bg-neutral-800 inline-flex items-center gap-1.5";
const outlineButton = "rounded-lg border border-black/10 bg-white text-sm font-semibold px-4 py-2 hover:bg-black/5";

export default function APIKeys() {
  const { data: keyData, isLoading, isError, refetch } = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const keys = keyData?.data ?? [];

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<KeyFormState>(emptyForm());
  const [newKey, setNewKey] = useState<{ id?: string; plaintext?: string; warning?: string } | null>(null);

  const [editKey, setEditKey] = useState<APIKeyData | null>(null);
  const [editForm, setEditForm] = useState<KeyFormState>(emptyForm());

  const [viewKey, setViewKey] = useState<APIKeyData | null>(null);
  const [viewPlaintext, setViewPlaintext] = useState("");
  const [viewLoading, setViewLoading] = useState(false);
  const [viewError, setViewError] = useState("");

  const createMutation = useConsoleMutation<Record<string, unknown>, Record<string, unknown>>("post", "/api-keys");
  const updateMutation = useConsoleMutation<unknown, { id: string } & Record<string, unknown>>(
    "put",
    (v) => `/api-keys/${v.id}`,
    "/api-keys",
  );
  const deleteMutation = useConsoleMutation<unknown, { id: string }>("delete", (v) => `/api-keys/${v.id}`, "/api-keys");

  const handleCreate = async () => {
    if (!createForm.name.trim()) return;
    const body: Record<string, unknown> = { name: createForm.name.trim() };
    const models = createForm.allowedModels.split(",").map((s) => s.trim()).filter(Boolean);
    const ips = createForm.sourceWhitelist.split(",").map((s) => s.trim()).filter(Boolean);
    if (models.length) body.allowed_models = models;
    if (ips.length) body.source_whitelist = ips;
    const { body: limits, errors } = buildKeyLimitsBody(createForm);
    if (errors.length) {
      window.alert(errors.join("\n"));
      return;
    }
    Object.assign(body, limits);
    body.over_limit_action = createForm.overLimitAction;

    const res = await createMutation.mutateAsync(body);
    if (res.plaintext && res.id) setKeySecret(String(res.id), String(res.plaintext));
    setNewKey(res);
    setCreateOpen(false);
    setCreateForm(emptyForm());
  };

  const openEdit = (k: APIKeyData) => {
    setEditKey(k);
    setEditForm(formFromKey(k));
  };

  const handleSaveEdit = async () => {
    if (!editKey || !editForm.name.trim()) return;
    const { body: limits, errors } = buildKeyLimitsBody(editForm);
    if (errors.length) {
      window.alert(errors.join("\n"));
      return;
    }
    const body: Record<string, unknown> = {
      name: editForm.name.trim(),
      allowed_models: editForm.allowedModels.split(",").map((s) => s.trim()).filter(Boolean),
      source_whitelist: editForm.sourceWhitelist.split(",").map((s) => s.trim()).filter(Boolean),
      ...limits,
      over_limit_action: editForm.overLimitAction,
      status: editForm.status,
    };
    await updateMutation.mutateAsync({ id: editKey.id, ...body });
    setEditKey(null);
  };

  const handleDelete = async (k: APIKeyData) => {
    if (!confirm(`确定要删除 API key「${k.name || k.masked_key}」吗？删除后立即失效。`)) return;
    await deleteMutation.mutateAsync({ id: k.id });
  };

  const openView = async (k: APIKeyData) => {
    setViewKey(k);
    setViewPlaintext("");
    setViewError("");
    setViewLoading(true);
    try {
      const res = await api.get<{ plaintext: string }>(`/api-keys/${k.id}/secret`);
      setViewPlaintext(res.plaintext);
      setKeySecret(k.id, res.plaintext);
    } catch (e) {
      setViewError(e instanceof Error ? e.message : "获取失败");
    } finally {
      setViewLoading(false);
    }
  };

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* clipboard unavailable in tests / restricted contexts */
    }
  };

  return (
    <div>
      <div className="flex items-start justify-between gap-4 mb-5">
        <div>
          <h2
            className="text-[25px] font-bold tracking-tight"
            style={{ fontFamily: "'Inter','PingFang SC','Microsoft YaHei',system-ui,sans-serif" }}
          >
            API keys
          </h2>
          <p className="text-[13px] text-[#5C6472] mt-2 leading-relaxed max-w-[720px]">
            列表内是你的全部 API key，API key 请妥善保存。不要与他人共享你的 API key，或将其暴露在浏览器或其他客户端代码中。为了保护你的帐户安全，我们可能会自动禁用我们发现已公开泄露的 API key。
          </p>
        </div>
        <button className={blackButton} onClick={() => setCreateOpen(true)}>
          <Plus size={15} />
          创建密钥
        </button>
      </div>

      {isLoading && <div className="py-12 text-center text-[#5C6472]">加载 API keys...</div>}

      {!isLoading && isError && (
        <div className="glass rounded-2xl border-[#E5484D]/25 p-5 text-[#C4372C] text-sm mb-4">
          <p className="font-medium">加载失败</p>
          <button onClick={() => refetch()} className="mt-2 rounded-lg bg-[#E5484D] px-4 py-2 text-sm font-semibold text-white hover:brightness-110">
            重试
          </button>
        </div>
      )}

      {!isLoading && keys.length === 0 && (
        <div className="glass rounded-2xl py-12 text-center">
          <p className="text-[#5C6472] font-medium">暂无 API key</p>
          <p className="text-sm mt-1 text-[#5C6472]/80">点击右上角"创建密钥"创建第一个 key</p>
        </div>
      )}

      {!isLoading && keys.length > 0 && (
        <div className="glass rounded-2xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="divide-x divide-black/10 bg-white/55">
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">名称</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">key</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">创建日期</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">最新使用日期</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">限流</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-black/10">
              {keys.map((k) => (
                <tr key={k.id} className="divide-x divide-black/10 hover:bg-white/60">
                  <td className="px-4 py-3 text-[#161A23]">{k.name}</td>
                  <td className="px-4 py-3 font-mono text-[13px] text-[#161A23]">{k.masked_key}</td>
                  <td className="px-4 py-3 text-[#161A23]">{gmt8DateTime(k.created_at)}</td>
                  <td className="px-4 py-3 text-[#161A23]">{k.last_used_at ? gmt8DateTime(k.last_used_at) : "从未使用"}</td>
                  <td className="px-4 py-3 text-[13px] text-[#161A23]">{formatRateLimit(k.rate_limit_rpm, k.rate_limit_tpm)}</td>
                  <td className="px-4 py-3 whitespace-nowrap text-[13px]">
                    <button className="text-[#4F6BED] hover:underline" onClick={() => openView(k)}>
                      查看key
                    </button>
                    <button className="text-[#4F6BED] hover:underline ml-3" onClick={() => openEdit(k)}>
                      编辑
                    </button>
                    <button className="text-[#4F6BED] hover:underline ml-3" onClick={() => handleDelete(k)}>
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 创建密钥 */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建新密钥</DialogTitle>
          </DialogHeader>
          <KeyFields form={createForm} onChange={setCreateForm} />
          <DialogFooter>
            <button className={outlineButton} onClick={() => setCreateOpen(false)}>
              取消
            </button>
            <button className={blackButton} onClick={handleCreate}>
              确认创建
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 新密钥明文（仅显示一次） */}
      <Dialog open={newKey !== null} onOpenChange={(o) => !o && setNewKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新 key 已创建</DialogTitle>
          </DialogHeader>
          {newKey?.plaintext && (
            <>
              <code className="block bg-black/[0.04] border border-black/10 rounded-lg px-3 py-2.5 text-sm font-mono break-all">
                {newKey.plaintext}
              </code>
              <p className="text-xs text-[#A06B12]">{newKey.warning || "请立即复制并安全保存，此 key 仅显示一次"}</p>
            </>
          )}
          <DialogFooter>
            {newKey?.plaintext && (
              <button className={blackButton} onClick={() => copyText(newKey.plaintext as string)}>
                <Copy size={14} />
                复制
              </button>
            )}
            <button className={outlineButton} onClick={() => setNewKey(null)}>
              我已保存，关闭
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑密钥 */}
      <Dialog open={editKey !== null} onOpenChange={(o) => !o && setEditKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑 key</DialogTitle>
          </DialogHeader>
          <KeyFields form={editForm} onChange={setEditForm} showStatus />
          <DialogFooter>
            <button className={outlineButton} onClick={() => setEditKey(null)}>
              取消
            </button>
            <button className={blackButton} onClick={handleSaveEdit}>
              保存更改
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 查看 key */}
      <Dialog open={viewKey !== null} onOpenChange={(o) => !o && setViewKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>查看 key</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-[#5C6472]">{viewKey?.name}</div>
          {viewLoading && <p className="text-sm text-[#5C6472]">正在获取...</p>}
          {viewError && <p className="text-sm text-[#C4372C]">{viewError}</p>}
          {viewPlaintext && (
            <>
              <code className="block bg-black/[0.04] border border-black/10 rounded-lg px-3 py-2.5 text-sm font-mono break-all">
                {viewPlaintext}
              </code>
              <p className="text-xs text-[#C4372C]">请勿与他人共享此 key，也不要在浏览器或其他客户端代码中暴露。</p>
            </>
          )}
          <DialogFooter>
            {viewPlaintext && (
              <button className={blackButton} onClick={() => copyText(viewPlaintext)}>
                <Copy size={14} />
                复制
              </button>
            )}
            <button className={outlineButton} onClick={() => setViewKey(null)}>
              关闭
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
