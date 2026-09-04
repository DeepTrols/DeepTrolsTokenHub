import { EmptyState } from "@/components/StateViews";
import { useState } from "react";
import { toast } from "sonner";
import { adminApi } from "../lib/api";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { parseHeaderLines, formatHeaderLines } from "../lib/domain/headers";
import { Plus, Trash2, Play, RotateCw, Server } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ProviderSyncDialog } from "@/components/ProviderSyncDialog";
import { ChannelModelList } from "@/components/ChannelModelList";
import "../i18n";
import { useTranslation } from "react-i18next";

interface CredentialData {
  id: string; name: string; provider: string; base_url: string;
  masked_key: string; status: string; model_count: number;
  channel_ids: string[]; created_at: string; updated_at: string;
  custom_headers?: Record<string, string>;
  upstream_format?: string;
  custom_override?: string;
  pool_type?: string;
  weight?: number;
  max_concurrency?: number;
  group_name?: string;
}

interface ProbeResult {
  ok: boolean;
  ms: number;
  models?: number;
  model_codes?: string[];
  capabilities?: Record<string, number>;
  error?: string;
}

const PROVIDER_OPTIONS = [
  { value: "deepseek", label: "DeepSeek 深度求索" },
  { value: "qwen", label: "Qwen 通义千问" },
  { value: "zhipu", label: "智谱AI ChatGLM" },
  { value: "moonshot", label: "Moonshot 月之暗面 (Kimi)" },
  { value: "baidu", label: "百度文心一言" },
  { value: "xfyun", label: "讯飞星火" },
  { value: "bytedance", label: "字节豆包" },
  { value: "tencent", label: "腾讯混元" },
  { value: "lingyi", label: "零一万物 Yi" },
  { value: "siliconflow", label: "SiliconFlow 硅基流动" },
  { value: "other", label: "Other (OpenAI 兼容)" },
];

function providerLabel(p: string) { return PROVIDER_OPTIONS.find((o) => o.value === p)?.label || p; }

export default function Channels() {
  const { t } = useTranslation();
  const { data: credData, isLoading, isError, error: fetchError, refetch } = useAdminQuery<{ data: CredentialData[] }>("/providers");
  const credentials = credData?.data ?? [];
  const loadError = isError ? (fetchError instanceof Error ? fetchError.message : String(fetchError)) : "";

  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; ms: number; detail?: string }>>({});
  const [syncId, setSyncId] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const createMut = useAdminMutation<unknown, Record<string, unknown>>("post", "/providers");
  const updateMut = useAdminMutation<unknown, { id: string } & Record<string, unknown>>("put", (v) => `/providers/${v.id}`, "/providers");
  const deleteMut = useAdminMutation<unknown, { id: string }>("delete", (v) => `/providers/${v.id}`, "/providers");
  const batchTest = useAdminMutation<{ results: { ok: boolean }[]; total: number }, void>(
    "post",
    "/channels/test-all",
    "/channels",
    {
      onSuccess: (r) => {
        const ok = (r.results ?? []).filter((x) => x.ok).length;
        toast.success(t("channels.testAllDone", { ok, fail: r.total - ok }));
      },
      onError: (e) => toast.error(e instanceof Error ? e.message : t("channels.batchTestFailed")),
    },
  );

  const handleTestCredential = async (cred: CredentialData) => {
    setTestingId(cred.id);
    try {
      const res = await adminApi.post<ProbeResult>(`/providers/${cred.id}/test`);
      setTestResults((prev) => ({
        ...prev,
        [cred.id]: {
          ok: res.ok,
          ms: res.ms ?? 0,
          detail: res.ok
            ? t("channels.probeModels", { count: res.models ?? 0 })
            : (res.error === "no active instance"
              ? t("channels.noActiveInstance")
              : (res.error || t("channels.probeFailed"))),
        },
      }));
    } catch (e) {
      setTestResults((prev) => ({
        ...prev,
        [cred.id]: { ok: false, ms: 0, detail: e instanceof Error ? e.message : t("channels.probeError") },
      }));
    } finally {
      setTestingId(null);
    }
  };

  const handleDeleteCredential = async (cred: CredentialData) => {
    if (!confirm(t("channels.deleteConfirm", { name: cred.name || cred.base_url }))) return;
    await deleteMut.mutateAsync({ id: cred.id });
  };

  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [chName, setChName] = useState("");
  const [chProvider, setChProvider] = useState("deepseek");
  const [chApiKey, setChApiKey] = useState("");
  const [chBaseURL, setChBaseURL] = useState("");
  const [chPriority, setChPriority] = useState(0);
  const [chWeight, setChWeight] = useState(100);
  const [chPoolType, setChPoolType] = useState("shared");
  const [chMaxConcurrency, setChMaxConcurrency] = useState(10);
  const [chModelMapping, setChModelMapping] = useState("");
  const [chParamOverride, setChParamOverride] = useState("");
  const [chAutoDisable, setChAutoDisable] = useState(false);
  const [chRoundMode, setChRoundMode] = useState("round_robin");
  const [chTags, setChTags] = useState<string[]>([]);
  const [chGroupName, setChGroupName] = useState("");
  const [tagInput, setTagInput] = useState("");
  const [chCustomHeaders, setChCustomHeaders] = useState("");
  const [chUpstreamFormat, setChUpstreamFormat] = useState("openai");
  const [chCustomOverride, setChCustomOverride] = useState("");

  const FORMAT_OPTIONS = [
    { value: "openai", label: t("channels.fmtOpenai") },
    { value: "gemini", label: t("channels.fmtGemini") },
    { value: "anthropic", label: t("channels.fmtAnthropic") },
    { value: "ollama", label: t("channels.fmtOllama") },
    { value: "azure", label: t("channels.fmtAzure") },
    { value: "custom", label: t("channels.fmtCustom") },
  ];

  const resetModal = () => {
    setChName(""); setChProvider("deepseek"); setChApiKey(""); setChBaseURL("");
    setChPriority(0); setChWeight(100); setChPoolType("shared"); setChMaxConcurrency(10);
    setChModelMapping(""); setChParamOverride(""); setChAutoDisable(false);
    setChRoundMode("round_robin"); setChTags([]); setChGroupName(""); setTagInput("");
    setChCustomHeaders(""); setEditingId(null);
    setChUpstreamFormat("openai"); setChCustomOverride("");
  };
  const openCreate = () => { resetModal(); setShowModal(true); };

  const openEditCredential = (cred: CredentialData) => {
    setEditingId(cred.id);
    setChName(cred.name); setChProvider(cred.provider); setChApiKey("");
    setChBaseURL(cred.base_url); setChPriority(0); setChWeight(cred.weight || 100);
    setChPoolType(cred.pool_type || "shared"); setChMaxConcurrency(cred.max_concurrency || 10);
    setChModelMapping(""); setChParamOverride(""); setChAutoDisable(false);
    setChRoundMode("round_robin"); setChTags([]); setChGroupName(cred.group_name || ""); setTagInput("");
    setChCustomHeaders(cred.custom_headers ? formatHeaderLines(cred.custom_headers) : "");
    setChUpstreamFormat(cred.upstream_format || "openai");
    setChCustomOverride(cred.custom_override || "");
    setShowModal(true);
  };

  const handleSubmit = async () => {
    if (!chName.trim()) return;
    const body: Record<string, unknown> = {
      name: chName.trim(), provider: chProvider, api_key: chApiKey.trim(),
      base_url: chBaseURL.trim(), priority: chPriority, weight: chWeight,
      pool_type: chPoolType, max_concurrency: chMaxConcurrency,
      round_mode: chRoundMode, tags: chTags, auto_disable: chAutoDisable,
      group_name: chGroupName.trim() || undefined,
      upstream_format: chUpstreamFormat || undefined,
      custom_override: chCustomOverride.trim() || undefined,
      model_mapping: chModelMapping.trim() || undefined,
      param_override: chParamOverride.trim() || undefined,
    };
    const customHeaders = parseHeaderLines(chCustomHeaders);
    if (Object.keys(customHeaders).length > 0) body.custom_headers = customHeaders;
    try {
      if (editingId) { await updateMut.mutateAsync({ id: editingId, ...body }); }
      else { await createMut.mutateAsync(body); }
      setShowModal(false); resetModal();
    } catch { /* mutation handles error */ }
  };

  const addTag = () => {
    if (tagInput.trim() && !chTags.includes(tagInput.trim())) setChTags([...chTags, tagInput.trim()]);
    setTagInput("");
  };
  const removeTag = (t: string) => setChTags(chTags.filter((x) => x !== t));

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div><h2 className="font-display text-[25px] font-bold tracking-tight">{t("channels.title")}</h2><p className="text-[13px] text-[#5C6472] mt-1">{t("channels.subtitle")}</p></div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => batchTest.mutate()} disabled={batchTest.isPending}>
            {batchTest.isPending ? t("channels.testing") : t("channels.batchTest")}
          </Button>
          <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />{t("channels.addChannel")}</Button>
        </div>
      </div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">{t("channels.loadFailed")}</p><p className="mt-1">{loadError}</p><Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>{t("channels.retry")}</Button></CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-[#F78B28] border-t-transparent rounded-full mx-auto mb-3" /><p className="text-muted-foreground">{t("channels.loading")}</p></CardContent></Card>}

      {credentials.length > 0 ? (
        <div className="space-y-4">
          {credentials.map((cred) => (
            <Card key={cred.id} className="overflow-hidden">
              <CardContent className="p-5">
                <div className="flex items-center justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-1">
                      <h3 className="font-semibold text-base">{cred.name || cred.base_url.split("//")[1]?.split("/")[0]}</h3>
                      {cred.status === "inactive" ? (
                        <Badge variant="secondary">{t("channels.inactive")}</Badge>
                      ) : (
                        <Badge variant="success">{t("channels.active")}</Badge>
                      )}
                      <span className="text-xs text-muted-foreground">{t("channels.modelsCount", { count: cred.model_count })}</span>
                    </div>
                    <div className="flex items-center gap-4 text-xs text-muted-foreground mt-1">
                      <Badge variant="secondary">{providerLabel(cred.provider)}</Badge>
                      <code className="font-mono">{cred.base_url}</code>
                      <span>API Key: <code className="font-mono">{cred.masked_key}</code></span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 ml-4">
                    <Button variant="outline" size="sm" onClick={() => handleTestCredential(cred)} disabled={testingId === cred.id}>
                      {testingId === cred.id ? <RotateCw size={12} className="animate-spin mr-1" /> : <Play size={12} className="mr-1" />}{t("channels.test")}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => setSyncId(cred.id)}>
                      <RotateCw size={12} className="mr-1" />{t("channels.syncModels")}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => openEditCredential(cred)}>{t("channels.edit")}</Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="border-destructive/30 text-destructive hover:bg-destructive/10"
                      onClick={() => handleDeleteCredential(cred)}
                    >
                      <Trash2 size={12} className="mr-1" />{t("channels.delete")}
                    </Button>
                  </div>
                </div>
                {testResults[cred.id] && (
                  <div className={`mt-3 pt-3 border-t border-black/10 text-xs ${testResults[cred.id].ok ? "text-[#0C7A55]" : "text-destructive"}`}>
                    {testResults[cred.id].ok
                      ? t("channels.testPassed", { detail: testResults[cred.id].detail, ms: testResults[cred.id].ms })
                      : t("channels.testFailed", { detail: testResults[cred.id].detail })}
                  </div>
                )}
                <div className="mt-3 pt-3 border-t border-black/10">
                  <button
                    onClick={() => setExpandedId(expandedId === cred.id ? null : cred.id)}
                    className="text-sm font-medium text-primary-700 hover:underline"
                  >
                    {expandedId === cred.id ? t("channels.collapse") : t("channels.boundModels", { count: cred.model_count })}
                  </button>
                  {expandedId === cred.id && (
                    <div className="mt-3">
                      <ChannelModelList channelId={cred.id} />
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        !isLoading && <EmptyState icon={Server} title={t("channels.empty")} description={t("channels.emptyDesc")} />
      )}

      <Dialog open={showModal} onOpenChange={(open) => { if (!open) setShowModal(false); }}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-auto">
          <DialogHeader>
            <DialogTitle>{editingId ? t("channels.editTitle") : t("channels.addTitle")}</DialogTitle>
            <DialogDescription>{t("channels.dialogDesc")}</DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="basic">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="basic">{t("channels.tabBasic")}</TabsTrigger>
              <TabsTrigger value="advanced">{t("channels.tabAdvanced")}</TabsTrigger>
            </TabsList>

            <TabsContent value="basic" className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("channels.providerType")}</Label>
                  <Select value={chProvider} onValueChange={setChProvider}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{PROVIDER_OPTIONS.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("channels.channelName")}</Label>
                  <Input value={chName} onChange={(e) => setChName(e.target.value)} placeholder={t("channels.namePlaceholder", { provider: providerLabel(chProvider) })} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.baseUrl")}</Label>
                <Input value={chBaseURL} onChange={(e) => setChBaseURL(e.target.value)} placeholder={t("channels.baseUrlPlaceholder")} className="font-mono" />
                <p className="text-xs text-muted-foreground">{t("channels.baseUrlHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.apiKey")}</Label>
                <Input type="password" value={chApiKey} onChange={(e) => setChApiKey(e.target.value)} placeholder={editingId ? t("channels.apiKeyEditPlaceholder") : "sk-..."} className="font-mono" />
              </div>
              <div className="space-y-2">
                <Label>{t("channels.roundMode")}</Label>
                <div className="space-y-2">
                  {[
                    { v: "round_robin", l: t("channels.roundRobin"), desc: t("channels.roundRobinDesc") },
                    { v: "weighted_random", l: t("channels.weightedRandom"), desc: t("channels.weightedRandomDesc") },
                  ].map((rm) => (
                    <label key={rm.v} className={`flex items-center gap-3 p-3 glass-soft rounded-xl cursor-pointer transition-all ${chRoundMode === rm.v ? "border-[#F78B28]/50 bg-[#F78B28]/[0.06] shadow-[0_10px_26px_rgba(137,76,32,0.10)]" : "hover:bg-white/80"}`}>
                      <input type="radio" name="roundMode" value={rm.v} checked={chRoundMode === rm.v} onChange={(e) => setChRoundMode(e.target.value)} className="text-primary-700" />
                      <div><p className="text-sm font-medium">{rm.l}</p><p className="text-xs text-muted-foreground">{rm.desc}</p></div>
                    </label>
                  ))}
                </div>
              </div>
            </TabsContent>

            <TabsContent value="advanced" className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("channels.poolType")}</Label>
                  <Select value={chPoolType} onValueChange={setChPoolType}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="shared">{t("channels.sharedPool")}</SelectItem>
                      <SelectItem value="dedicated">{t("channels.dedicatedPool")}</SelectItem>
                      <SelectItem value="mixed">{t("channels.mixedPool")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("channels.priority")}</Label>
                  <Input type="number" value={chPriority} onChange={(e) => setChPriority(Number(e.target.value))} />
                  <p className="text-xs text-muted-foreground">{t("channels.priorityHint")}</p>
                </div>
                <div className="space-y-2">
                  <Label>{t("channels.weight")}</Label>
                  <Input type="number" value={chWeight} onChange={(e) => setChWeight(Number(e.target.value))} />
                  <p className="text-xs text-muted-foreground">{t("channels.weightHint")}</p>
                </div>
                <div className="space-y-2">
                  <Label>{t("channels.maxConcurrency")}</Label>
                  <Input type="number" value={chMaxConcurrency} onChange={(e) => setChMaxConcurrency(Number(e.target.value))} />
                </div>
                <div className="flex items-end pb-2">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input type="checkbox" checked={chAutoDisable} onChange={(e) => setChAutoDisable(e.target.checked)} className="rounded" />
                    <span className="text-sm">{t("channels.autoDisable")}</span>
                  </label>
                  <span className="text-xs text-muted-foreground ml-2">{t("channels.autoDisableHint")}</span>
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.tags")}</Label>
                <div className="flex flex-wrap gap-1 mb-2">
                  {chTags.map((t) => (
                    <Badge key={t} variant="secondary" className="gap-1">{t}<button onClick={() => removeTag(t)} className="hover:text-destructive ml-0.5">x</button></Badge>
                  ))}
                </div>
                <div className="flex gap-2">
                  <Input value={tagInput} onChange={(e) => setTagInput(e.target.value)} onKeyDown={(e) => e.key === "Enter" && addTag()} placeholder={t("channels.tagPlaceholder")} className="h-8 text-xs" />
                  <Button variant="outline" size="sm" onClick={addTag}>{t("channels.add")}</Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.group")}</Label>
                <Input value={chGroupName} onChange={(e) => setChGroupName(e.target.value)} placeholder={t("channels.groupPlaceholder")} className="h-8 text-xs" />
              </div>
              <div className="space-y-2">
                <Label>{t("channels.modelMapping")}</Label>
                <textarea value={chModelMapping} onChange={(e) => setChModelMapping(e.target.value)} placeholder='{"gpt-4": "gpt-4-0613"}' rows={3} className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
                <p className="text-xs text-muted-foreground">{t("channels.modelMappingHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.paramOverride")}</Label>
                <textarea value={chParamOverride} onChange={(e) => setChParamOverride(e.target.value)} placeholder='{"temperature": 0.8}' rows={4} className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
                <p className="text-xs text-muted-foreground">{t("channels.paramOverrideHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.customHeaders")}</Label>
                <textarea value={chCustomHeaders} onChange={(e) => setChCustomHeaders(e.target.value)} placeholder={"X-Gateway-Id: gw-east-1\nX-Tenant: acme"} rows={4} className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
                <p className="text-xs text-muted-foreground">{t("channels.customHeadersHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("channels.upstreamFormat")}</Label>
                <Select value={chUpstreamFormat} onValueChange={setChUpstreamFormat}>
                  <SelectTrigger className="h-8 text-xs"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {FORMAT_OPTIONS.map((f) => (
                      <SelectItem key={f.value} value={f.value}>{f.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("channels.upstreamFormatHint")}</p>
              </div>
              {chUpstreamFormat === "custom" && (
                <div className="space-y-2">
                  <Label>{t("channels.customOverride")}</Label>
                  <textarea
                    value={chCustomOverride}
                    onChange={(e) => setChCustomOverride(e.target.value)}
                    placeholder={'{"method":"POST","url":"https://api.example.com/v1/{model}/generate","headers":{"X-Custom":"dt-{model}"},"body":"{\\"model\\":\\"{model}\\"}"}'}
                    rows={5}
                    className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
                  />
                  <p className="text-xs text-muted-foreground">{t("channels.customOverrideHint")}</p>
                </div>
              )}
            </TabsContent>
          </Tabs>

          <div className="flex items-center justify-between pt-4 border-t">
            <Button variant="outline" onClick={() => setShowModal(false)}>{t("channels.cancel")}</Button>
            <Button onClick={handleSubmit} disabled={createMut.isPending || updateMut.isPending}>
              {createMut.isPending || updateMut.isPending ? t("channels.submitting") : t("channels.submit")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <ProviderSyncDialog
        open={!!syncId}
        providerId={syncId ?? ""}
        onClose={() => setSyncId(null)}
        onSynced={() => { setSyncId(null); refetch(); }}
      />
    </div>
  );
}
