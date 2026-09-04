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
import "../i18n";
import { useTranslation } from "react-i18next";

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
  expires_at: string;
  groupName: string;
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
  expires_at: "",
  groupName: "",
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
  expires_at: k.expires_at || "",
  groupName: k.group_name || "",
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
  const { t } = useTranslation();
  const set = (patch: Partial<KeyFormState>) => onChange({ ...form, ...patch });
  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="key-name" className="text-[12px] font-semibold text-[#5C6472]">
          {t("apikeys.name")}
        </Label>
        <Input
          id="key-name"
          type="text"
          value={form.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder={t("apikeys.namePlaceholder")}
        />
      </div>
      <div>
        <Label htmlFor="key-models" className="text-[12px] font-semibold text-[#5C6472]">
          {t("apikeys.models")}
        </Label>
        <Input
          id="key-models"
          type="text"
          value={form.allowedModels}
          onChange={(e) => set({ allowedModels: e.target.value })}
          placeholder={t("apikeys.modelsPlaceholder")}
          className="font-mono"
        />
        <p className="text-xs text-[#5C6472]/80 mt-1">{t("apikeys.modelsHint")}</p>
      </div>
      <div>
        <Label htmlFor="key-ips" className="text-[12px] font-semibold text-[#5C6472]">
          {t("apikeys.ips")}
        </Label>
        <Input
          id="key-ips"
          type="text"
          value={form.sourceWhitelist}
          onChange={(e) => set({ sourceWhitelist: e.target.value })}
          placeholder={t("apikeys.ipsPlaceholder")}
          className="font-mono"
        />
        <p className="text-xs text-[#5C6472]/80 mt-1">{t("apikeys.ipsHint")}</p>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <Label htmlFor="key-monthly" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.monthlyLabel")}
          </Label>
          <Input
            id="key-monthly"
            type="number"
            min="0"
            step="0.01"
            value={form.monthlyLimit}
            onChange={(e) => set({ monthlyLimit: e.target.value })}
            placeholder={t("apikeys.monthlyPlaceholder")}
          />
        </div>
        <div>
          <Label htmlFor="key-weekly" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.weeklyLabel")}
          </Label>
          <Input
            id="key-weekly"
            type="number"
            min="0"
            step="0.01"
            value={form.weeklyLimit}
            onChange={(e) => set({ weeklyLimit: e.target.value })}
            placeholder={t("apikeys.weeklyPlaceholder")}
          />
        </div>
        <div>
          <Label htmlFor="key-cumulative" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.cumulativeLabel")}
          </Label>
          <Input
            id="key-cumulative"
            type="number"
            min="0"
            step="0.01"
            value={form.cumulativeLimit}
            onChange={(e) => set({ cumulativeLimit: e.target.value })}
            placeholder={t("apikeys.cumulativePlaceholder")}
          />
        </div>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <Label htmlFor="key-rpm" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.rpmLabel")}
          </Label>
          <Input
            id="key-rpm"
            type="number"
            min="0"
            step="1"
            value={form.rateLimitRpm}
            onChange={(e) => set({ rateLimitRpm: e.target.value })}
            placeholder={t("apikeys.rpmPlaceholder")}
          />
        </div>
        <div>
          <Label htmlFor="key-tpm" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.tpmLabel")}
          </Label>
          <Input
            id="key-tpm"
            type="number"
            min="0"
            step="1"
            value={form.rateLimitTpm}
            onChange={(e) => set({ rateLimitTpm: e.target.value })}
            placeholder={t("apikeys.tpmPlaceholder")}
          />
        </div>
        <div>
          <Label htmlFor="key-expiry" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.expiryLabel")}
          </Label>
          <Input
            id="key-expiry"
            type="datetime-local"
            value={form.expires_at}
            onChange={(e) => set({ expires_at: e.target.value })}
            placeholder={t("apikeys.expiryPlaceholder")}
          />
        </div>
        <div>
          <Label htmlFor="key-group" className="text-xs font-medium text-[#5C6472] mb-1 block">
            {t("apikeys.groupLabel")}
          </Label>
          <Input
            id="key-group"
            value={form.groupName}
            onChange={(e) => set({ groupName: e.target.value })}
            placeholder={t("apikeys.groupPlaceholder")}
          />
          <p className="mt-1 text-[11px] text-[#8C93A1]">{t("apikeys.groupHint")}</p>
        </div>
      </div>
      <p className="text-xs text-[#5C6472]/80">{t("apikeys.rateHint")}</p>
      <div>
        <span className="block text-sm font-medium text-[#161A23] mb-2">{t("apikeys.overLimitAction")}</span>
        <div className="flex gap-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="over_limit_action"
              value="block"
              checked={form.overLimitAction === "block"}
              onChange={(e) => set({ overLimitAction: e.target.value })}
              className="accent-[#F78B28]"
              aria-label={t("apikeys.block")}
            />
            <span className="text-sm text-[#161A23]">{t("apikeys.block")}</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              name="over_limit_action"
              value="warn"
              checked={form.overLimitAction === "warn"}
              onChange={(e) => set({ overLimitAction: e.target.value })}
              className="accent-[#F78B28]"
              aria-label={t("apikeys.warn")}
            />
            <span className="text-sm text-[#161A23]">{t("apikeys.warn")}</span>
          </label>
        </div>
      </div>
      {showStatus && (
        <div>
          <Label htmlFor="key-status" className="text-[12px] font-semibold text-[#5C6472]">
            {t("apikeys.status")}
          </Label>
          <select
            id="key-status"
            value={form.status}
            onChange={(e) => set({ status: e.target.value })}
            className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-[#F78B28]/25"
          >
            <option value="active">{t("apikeys.active")}</option>
            <option value="disabled">{t("apikeys.disabled")}</option>
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
  const { t } = useTranslation();
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
    if (createForm.expires_at) body.expires_at = new Date(createForm.expires_at).toISOString();
    if (createForm.groupName) body.group_name = createForm.groupName;

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
    if (!confirm(t("apikeys.deleteConfirm", { name: k.name || k.masked_key }))) return;
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
      setViewError(e instanceof Error ? e.message : t("apikeys.fetchFailed"));
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
            {t("apikeys.description")}
          </p>
        </div>
        <button className={blackButton} onClick={() => setCreateOpen(true)}>
          <Plus size={15} />
          {t("apikeys.create")}
        </button>
      </div>

      {isLoading && <div className="py-12 text-center text-[#5C6472]">{t("apikeys.loading")}</div>}

      {!isLoading && isError && (
        <div className="glass rounded-2xl border-[#E5484D]/25 p-5 text-[#C4372C] text-sm mb-4">
          <p className="font-medium">{t("apikeys.loadFailed")}</p>
          <button onClick={() => refetch()} className="mt-2 rounded-lg bg-[#E5484D] px-4 py-2 text-sm font-semibold text-white hover:brightness-110">
            {t("apikeys.retry")}
          </button>
        </div>
      )}

      {!isLoading && keys.length === 0 && (
        <div className="glass rounded-2xl py-12 text-center">
          <p className="text-[#5C6472] font-medium">{t("apikeys.empty")}</p>
          <p className="text-sm mt-1 text-[#5C6472]/80">{t("apikeys.emptyHint")}</p>
        </div>
      )}

      {!isLoading && keys.length > 0 && (
        <div className="glass rounded-2xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="divide-x divide-black/10 bg-white/55">
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thName")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thKey")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thCreated")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thLastUsed")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thExpires")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thGroup")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thRateLimit")}</th>
                <th className="px-4 py-3 text-left text-[13px] font-bold text-[#161A23]">{t("apikeys.thActions")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-black/10">
              {keys.map((k) => (
                <tr key={k.id} className="divide-x divide-black/10 hover:bg-white/60">
                  <td className="px-4 py-3 text-[#161A23]">{k.name}</td>
                  <td className="px-4 py-3 font-mono text-[13px] text-[#161A23]">{k.masked_key}</td>
                  <td className="px-4 py-3 text-[#161A23]">{gmt8DateTime(k.created_at)}</td>
                  <td className="px-4 py-3 text-[#161A23]">{k.last_used_at ? gmt8DateTime(k.last_used_at) : t("apikeys.neverUsed")}</td>
                  <td className="px-4 py-3 text-[#161A23]">{k.expires_at ? gmt8DateTime(k.expires_at) : t("apikeys.neverExpires")}</td>
                  <td className="px-4 py-3 text-[#161A23]">{k.group_name || "-"}</td>
                  <td className="px-4 py-3 text-[13px] text-[#161A23]">{formatRateLimit(k.rate_limit_rpm, k.rate_limit_tpm)}</td>
                  <td className="px-4 py-3 whitespace-nowrap text-[13px]">
                    <button className="text-primary-700 hover:underline" onClick={() => openView(k)}>
                      {t("apikeys.viewKey")}
                    </button>
                    <button className="text-primary-700 hover:underline ml-3" onClick={() => openEdit(k)}>
                      {t("apikeys.edit")}
                    </button>
                    <button className="text-primary-700 hover:underline ml-3" onClick={() => handleDelete(k)}>
                      {t("apikeys.delete")}
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
            <DialogTitle>{t("apikeys.createTitle")}</DialogTitle>
          </DialogHeader>
          <KeyFields form={createForm} onChange={setCreateForm} />
          <DialogFooter>
            <button className={outlineButton} onClick={() => setCreateOpen(false)}>
              {t("apikeys.cancel")}
            </button>
            <button className={blackButton} onClick={handleCreate}>
              {t("apikeys.confirmCreate")}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 新密钥明文（仅显示一次） */}
      <Dialog open={newKey !== null} onOpenChange={(o) => !o && setNewKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apikeys.createdTitle")}</DialogTitle>
          </DialogHeader>
          {newKey?.plaintext && (
            <>
              <code className="block bg-black/[0.04] border border-black/10 rounded-lg px-3 py-2.5 text-sm font-mono break-all">
                {newKey.plaintext}
              </code>
              <p className="text-xs text-[#A06B12]">{newKey.warning || t("apikeys.copyWarning")}</p>
            </>
          )}
          <DialogFooter>
            {newKey?.plaintext && (
              <button className={blackButton} onClick={() => copyText(newKey.plaintext as string)}>
                <Copy size={14} />
                {t("apikeys.copy")}
              </button>
            )}
            <button className={outlineButton} onClick={() => setNewKey(null)}>
              {t("apikeys.savedClose")}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑密钥 */}
      <Dialog open={editKey !== null} onOpenChange={(o) => !o && setEditKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apikeys.editTitle")}</DialogTitle>
          </DialogHeader>
          <KeyFields form={editForm} onChange={setEditForm} showStatus />
          <DialogFooter>
            <button className={outlineButton} onClick={() => setEditKey(null)}>
              {t("apikeys.cancel")}
            </button>
            <button className={blackButton} onClick={handleSaveEdit}>
              {t("apikeys.save")}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 查看 key */}
      <Dialog open={viewKey !== null} onOpenChange={(o) => !o && setViewKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("apikeys.viewTitle")}</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-[#5C6472]">{viewKey?.name}</div>
          {viewLoading && <p className="text-sm text-[#5C6472]">{t("apikeys.fetching")}</p>}
          {viewError && <p className="text-sm text-[#C4372C]">{viewError}</p>}
          {viewPlaintext && (
            <>
              <code className="block bg-black/[0.04] border border-black/10 rounded-lg px-3 py-2.5 text-sm font-mono break-all">
                {viewPlaintext}
              </code>
              <p className="text-xs text-[#C4372C]">{t("apikeys.shareWarning")}</p>
            </>
          )}
          <DialogFooter>
            {viewPlaintext && (
              <button className={blackButton} onClick={() => copyText(viewPlaintext)}>
                <Copy size={14} />
                {t("apikeys.copy")}
              </button>
            )}
            <button className={outlineButton} onClick={() => setViewKey(null)}>
              {t("apikeys.close")}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
