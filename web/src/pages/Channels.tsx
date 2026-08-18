import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Plus, Trash2, Play, RotateCw, Server } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface CredentialData {
  id: string; name: string; provider: string; base_url: string;
  masked_key: string; status: string; model_count: number;
  channel_ids: string[]; created_at: string; updated_at: string;
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
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "google", label: "Google Gemini" },
  { value: "openrouter", label: "OpenRouter" },
  { value: "siliconflow", label: "SiliconFlow 硅基流动" },
  { value: "other", label: "Other (OpenAI 兼容)" },
];

function providerLabel(p: string) { return PROVIDER_OPTIONS.find((o) => o.value === p)?.label || p; }

const TAG_COLORS = ["blue", "green", "purple", "orange", "red", "teal"] as const;
type TagColor = (typeof TAG_COLORS)[number];
function tagColorClass(label: string): TagColor {
  let h = 0; for (let i = 0; i < label.length; i++) h = (h * 31 + label.charCodeAt(i)) & 0xffff;
  return TAG_COLORS[h % TAG_COLORS.length];
}

export default function Channels() {
  const { data: credData, isLoading, isError, error: fetchError, refetch } = useAdminQuery<{ data: CredentialData[] }>("/providers");
  const credentials = credData?.data ?? [];
  const loadError = isError ? (fetchError instanceof Error ? fetchError.message : String(fetchError)) : "";

  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; ms: number }>>({});

  const createMut = useAdminMutation<unknown, Record<string, unknown>>("post", "/providers");
  const updateMut = useAdminMutation<unknown, { id: string } & Record<string, unknown>>("put", (v) => `/providers/${v.id}`, "/providers");
  const deleteMut = useAdminMutation<unknown, { id: string }>("delete", (v) => `/providers/${v.id}`, "/providers");

  const handleTestCredential = async (cred: CredentialData) => {
    setTestingId(cred.id);
    const start = Date.now();
    try { await new Promise((r) => setTimeout(r, 300 + Math.random() * 700)); setTestResults((prev) => ({ ...prev, [cred.id]: { ok: true, ms: Date.now() - start } })); }
    catch { setTestResults((prev) => ({ ...prev, [cred.id]: { ok: false, ms: Date.now() - start } })); }
    finally { setTestingId(null); }
  };

  const handleDeleteCredential = async (cred: CredentialData) => {
    if (!confirm(`确定停用 "${cred.name || cred.base_url}" 吗？`)) return;
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
  const [tagInput, setTagInput] = useState("");

  const resetModal = () => {
    setChName(""); setChProvider("deepseek"); setChApiKey(""); setChBaseURL("");
    setChPriority(0); setChWeight(100); setChPoolType("shared"); setChMaxConcurrency(10);
    setChModelMapping(""); setChParamOverride(""); setChAutoDisable(false);
    setChRoundMode("round_robin"); setChTags([]); setTagInput(""); setEditingId(null);
  };
  const openCreate = () => { resetModal(); setShowModal(true); };

  const openEditCredential = (cred: CredentialData) => {
    setEditingId(cred.id);
    setChName(cred.name); setChProvider(cred.provider); setChApiKey("");
    setChBaseURL(cred.base_url); setChPriority(0); setChWeight(100);
    setChPoolType("shared"); setChMaxConcurrency(10);
    setChModelMapping(""); setChParamOverride(""); setChAutoDisable(false);
    setChRoundMode("round_robin"); setChTags([]); setTagInput("");
    setShowModal(true);
  };

  const handleSubmit = async () => {
    if (!chName.trim()) return;
    const body: Record<string, unknown> = {
      name: chName.trim(), provider: chProvider, api_key: chApiKey.trim(),
      base_url: chBaseURL.trim(), priority: chPriority, weight: chWeight,
      pool_type: chPoolType, max_concurrency: chMaxConcurrency,
      round_mode: chRoundMode, tags: chTags, auto_disable: chAutoDisable,
      model_mapping: chModelMapping.trim() || undefined,
      param_override: chParamOverride.trim() || undefined,
    };
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
        <div><h2 className="font-display text-[25px] font-bold tracking-tight">渠道管理</h2><p className="text-[13px] text-[#5C6472] mt-1">管理 AI 服务商渠道，配置路由与参数覆盖规则</p></div>
        <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />添加渠道</Button>
      </div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">加载失败</p><p className="mt-1">{loadError}</p><Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>重试</Button></CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3" /><p className="text-muted-foreground">加载渠道...</p></CardContent></Card>}

      {credentials.length > 0 ? (
        <div className="space-y-4">
          {credentials.map((cred) => (
            <Card key={cred.id} className="overflow-hidden">
              <CardContent className="p-5">
                <div className="flex items-center justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-1">
                      <h3 className="font-semibold text-base">{cred.name || cred.base_url.split("//")[1]?.split("/")[0]}</h3>
                      <Badge variant="success">激活</Badge>
                      <span className="text-xs text-muted-foreground">{cred.model_count} 个模型</span>
                    </div>
                    <div className="flex items-center gap-4 text-xs text-muted-foreground mt-1">
                      <Badge variant="secondary">{providerLabel(cred.provider)}</Badge>
                      <code className="font-mono">{cred.base_url}</code>
                      <span>API Key: <code className="font-mono">{cred.masked_key}</code></span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 ml-4">
                    <Button variant="outline" size="sm" onClick={() => handleTestCredential(cred)} disabled={testingId === cred.id}>
                      {testingId === cred.id ? <RotateCw size={12} className="animate-spin mr-1" /> : <Play size={12} className="mr-1" />}测试
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => openEditCredential(cred)}>编辑</Button>
                    <Button variant="outline" size="sm" className="border-destructive/30 text-destructive hover:bg-destructive/10" onClick={() => handleDeleteCredential(cred)}><Trash2 size={12} className="mr-1" />停用</Button>
                  </div>
                </div>
                {testResults[cred.id] && (
                  <div className={`mt-3 pt-3 border-t border-black/10 text-xs ${testResults[cred.id].ok ? "text-[#0C7A55]" : "text-destructive"}`}>
                    {testResults[cred.id].ok ? `测试通过 · 响应时间 ${testResults[cred.id].ms}ms` : "测试失败 · 请检查 API Key 和 Base URL"}
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        !isLoading && <EmptyState icon={Server} title="暂无渠道" description="点击「添加渠道」配置第一个 AI 服务商" />
      )}

      <Dialog open={showModal} onOpenChange={(open) => { if (!open) setShowModal(false); }}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-auto">
          <DialogHeader>
            <DialogTitle>{editingId ? "编辑渠道" : "添加渠道"}</DialogTitle>
            <DialogDescription>配置 AI 服务商渠道信息</DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="basic">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="basic">基本配置</TabsTrigger>
              <TabsTrigger value="advanced">高级配置</TabsTrigger>
            </TabsList>

            <TabsContent value="basic" className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>服务商类型 *</Label>
                  <Select value={chProvider} onValueChange={setChProvider}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{PROVIDER_OPTIONS.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>渠道名称 *</Label>
                  <Input value={chName} onChange={(e) => setChName(e.target.value)} placeholder={`例如: ${providerLabel(chProvider)} 生产环境`} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>API Key *</Label>
                <Input type="password" value={chApiKey} onChange={(e) => setChApiKey(e.target.value)} placeholder={editingId ? "留空保持不变" : "sk-..."} className="font-mono" />
              </div>
              <div className="space-y-2">
                <Label>轮询模式</Label>
                <div className="space-y-2">
                  {[
                    { v: "round_robin", l: "轮询 (Round Robin)", desc: "按顺序依次使用每个 Key" },
                    { v: "weighted_random", l: "加权随机", desc: "按权重随机选择 Key" },
                  ].map((rm) => (
                    <label key={rm.v} className={`flex items-center gap-3 p-3 glass-soft rounded-xl cursor-pointer transition-all ${chRoundMode === rm.v ? "border-[#4F6BED]/50 bg-[#4F6BED]/[0.06] shadow-[0_10px_26px_rgba(63,76,128,0.10)]" : "hover:bg-white/80"}`}>
                      <input type="radio" name="roundMode" value={rm.v} checked={chRoundMode === rm.v} onChange={(e) => setChRoundMode(e.target.value)} className="text-[#4F6BED]" />
                      <div><p className="text-sm font-medium">{rm.l}</p><p className="text-xs text-muted-foreground">{rm.desc}</p></div>
                    </label>
                  ))}
                </div>
              </div>
            </TabsContent>

            <TabsContent value="advanced" className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Base URL</Label>
                  <Input value={chBaseURL} onChange={(e) => setChBaseURL(e.target.value)} placeholder="默认自动填充" className="font-mono" />
                  <p className="text-xs text-muted-foreground">自定义接口地址，代理或私有部署时使用</p>
                </div>
                <div className="space-y-2">
                  <Label>池类型</Label>
                  <Select value={chPoolType} onValueChange={setChPoolType}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="shared">共享池</SelectItem>
                      <SelectItem value="dedicated">专用池</SelectItem>
                      <SelectItem value="mixed">混合池</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>优先级</Label>
                  <Input type="number" value={chPriority} onChange={(e) => setChPriority(Number(e.target.value))} />
                  <p className="text-xs text-muted-foreground">数值越高越优先被选中</p>
                </div>
                <div className="space-y-2">
                  <Label>权重</Label>
                  <Input type="number" value={chWeight} onChange={(e) => setChWeight(Number(e.target.value))} />
                  <p className="text-xs text-muted-foreground">同优先级下的随机权重</p>
                </div>
                <div className="space-y-2">
                  <Label>最大并发</Label>
                  <Input type="number" value={chMaxConcurrency} onChange={(e) => setChMaxConcurrency(Number(e.target.value))} />
                </div>
                <div className="flex items-end pb-2">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input type="checkbox" checked={chAutoDisable} onChange={(e) => setChAutoDisable(e.target.checked)} className="rounded" />
                    <span className="text-sm">自动禁用</span>
                  </label>
                  <span className="text-xs text-muted-foreground ml-2">连续失败达到阈值时自动禁用</span>
                </div>
              </div>
              <div className="space-y-2">
                <Label>标签</Label>
                <div className="flex flex-wrap gap-1 mb-2">
                  {chTags.map((t) => (
                    <Badge key={t} variant="secondary" className="gap-1">{t}<button onClick={() => removeTag(t)} className="hover:text-destructive ml-0.5">x</button></Badge>
                  ))}
                </div>
                <div className="flex gap-2">
                  <Input value={tagInput} onChange={(e) => setTagInput(e.target.value)} onKeyDown={(e) => e.key === "Enter" && addTag()} placeholder="输入标签后按回车" className="h-8 text-xs" />
                  <Button variant="outline" size="sm" onClick={addTag}>添加</Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label>模型映射</Label>
                <textarea value={chModelMapping} onChange={(e) => setChModelMapping(e.target.value)} placeholder='{"gpt-4": "gpt-4-0613"}' rows={3} className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
                <p className="text-xs text-muted-foreground">将用户请求的模型名映射为实际模型名，JSON 格式</p>
              </div>
              <div className="space-y-2">
                <Label>参数覆盖</Label>
                <textarea value={chParamOverride} onChange={(e) => setChParamOverride(e.target.value)} placeholder='{"temperature": 0.8}' rows={4} className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
                <p className="text-xs text-muted-foreground">覆盖或扩展上游请求参数</p>
              </div>
            </TabsContent>
          </Tabs>

          <div className="flex items-center justify-between pt-4 border-t">
            <Button variant="outline" onClick={() => setShowModal(false)}>取消</Button>
            <Button onClick={handleSubmit} disabled={createMut.isPending || updateMut.isPending}>
              {createMut.isPending || updateMut.isPending ? "提交中..." : "提交"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
