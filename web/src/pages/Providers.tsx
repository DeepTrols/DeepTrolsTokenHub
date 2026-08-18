import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Plus, Edit, Trash2, Server, RefreshCw } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";

interface ProviderData { id: string; name: string; provider: string; base_url: string; masked_key: string; status: string; model_count: number; }
const PO = [{ v: "deepseek", l: "DeepSeek" }, { v: "qwen", l: "Qwen 通义千问" }, { v: "zhipu", l: "智谱AI" }, { v: "moonshot", l: "Moonshot" }, { v: "baidu", l: "百度文心" }, { v: "xfyun", l: "讯飞星火" }, { v: "bytedance", l: "字节豆包" }, { v: "tencent", l: "腾讯混元" }, { v: "lingyi", l: "零一万物" }, { v: "openai", l: "OpenAI" }, { v: "anthropic", l: "Anthropic" }, { v: "google", l: "Google Gemini" }, { v: "openrouter", l: "OpenRouter" }, { v: "siliconflow", l: "SiliconFlow" }, { v: "other", l: "Other" }];
const pl = (p: string) => PO.find(o => o.v === p)?.l || p;

export default function Providers() {
  const { data, isLoading, isError, error: qe, refetch } = useAdminQuery<{ data: ProviderData[] }>("/providers");
  const pvs = data?.data ?? [];
  const le = isError ? (qe instanceof Error ? qe.message : String(qe)) : "";
  const [sf, setSf] = useState(false); const [ed, setEd] = useState<ProviderData | null>(null);
  const [nm, setNm] = useState(""); const [pr, setPr] = useState("openai");
  const [bu, setBu] = useState(""); const [ak, setAk] = useState(""); const [er, setEr] = useState("");
  const cM = useAdminMutation<unknown, Record<string, string>>("post", "/providers");
  const uM = useAdminMutation<unknown, { id: string } & Record<string, string>>("put", (v) => "/providers/" + v.id);
  const sM = useAdminMutation<{ synced: number; message: string }, { id: string }>("post", (v) => "/providers/" + v.id + "/sync");
  const dM = useAdminMutation<unknown, { id: string }>("delete", (v) => "/providers/" + v.id);
  const [sy, setSy] = useState<string | null>(null);
  const rf = () => { setNm(""); setPr("openai"); setBu(""); setAk(""); setEd(null); setEr(""); };
  const oc = () => { rf(); setSf(true); };
  const oe = (p: ProviderData) => { setNm(p.name); setPr(p.provider || "openai"); setBu(p.base_url); setAk(""); setEd(p); setSf(true); };
  const hs = async () => { setEr(""); if (!nm.trim()) { setEr("名称为必填"); return; } try { if (ed) await uM.mutateAsync({ id: ed.id, name: nm.trim(), base_url: bu.trim(), api_key: ak.trim() }); else { if (!ak.trim()) { setEr("API Key 必填"); return; } await cM.mutateAsync({ name: nm.trim(), provider: pr, base_url: bu.trim(), api_key: ak.trim() }); } rf(); setSf(false); } catch (e) { setEr(e instanceof Error ? e.message : "失败"); } };
  const hsy = async (p: ProviderData) => { setSy(p.id); try { const r = await sM.mutateAsync({ id: p.id }); alert(r.message); } catch { alert("同步失败"); } finally { setSy(null); } };
  const hd = async (p: ProviderData) => { if (!confirm("确定停用?")) return; try { await dM.mutateAsync({ id: p.id }); } catch { } };

  return <div>
    <div className="flex items-center justify-between mb-6"><div><h2 className="font-display text-[25px] font-bold tracking-tight">Provider 凭证管理</h2><p className="text-[13px] text-[#5C6472] mt-1">管理上游模型提供商的 API Key 凭证</p></div><Button onClick={oc}><Plus size={16} className="mr-1.5" />添加凭证</Button></div>
    {le && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">加载失败</p><Button variant="destructive" size="sm" className="mt-2" onClick={() => refetch()}>重试</Button></CardContent></Card>}
    {isLoading && <Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3" /><p className="text-muted-foreground">加载中...</p></CardContent></Card>}
    {!isLoading && pvs.length === 0 && <Card><CardContent className="p-12 text-center text-muted-foreground"><Server size={40} className="mx-auto mb-3 opacity-30" /><p>暂无凭证</p></CardContent></Card>}
    <div className="space-y-2">{pvs.map(p => <Card key={p.id}><CardContent className="p-4 flex items-center justify-between"><div><div className="flex items-center gap-2"><p className="font-medium text-sm">{p.name || p.base_url}</p><Badge variant={p.status === "active" ? "success" : "secondary"}>{p.status === "active" ? "激活" : "已停用"}</Badge><span className="text-xs text-muted-foreground">{p.model_count} 模型</span></div><p className="text-xs text-muted-foreground mt-0.5"><Badge variant="secondary" className="text-xs mr-1">{pl(p.provider)}</Badge>{p.base_url && <code className="text-xs font-mono ml-1">{p.base_url}</code>}</p><p className="text-xs text-muted-foreground mt-0.5">API Key: <code className="text-xs font-mono">{p.masked_key}</code></p></div><div className="flex items-center gap-2"><Button variant="ghost" size="icon" onClick={() => hsy(p)} disabled={sy === p.id}><RefreshCw size={14} className={sy === p.id ? "animate-spin" : ""} /></Button><Button variant="ghost" size="icon" title="编辑" aria-label="编辑" onClick={() => oe(p)}><Edit size={14} /></Button><Button variant="ghost" size="icon" title="停用" aria-label="停用" className="hover:text-destructive" onClick={() => hd(p)}><Trash2 size={14} /></Button></div></CardContent></Card>)}</div>
    <Dialog open={sf} onOpenChange={setSf}><DialogContent><DialogHeader><DialogTitle>{ed ? "编辑凭证" : "添加凭证"}</DialogTitle></DialogHeader>
      {er && <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg text-destructive text-sm">{er}</div>}
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4"><div className="space-y-2"><Label>名称 *</Label><Input value={nm} onChange={e => setNm(e.target.value)} placeholder="例如: OpenAI Production" /></div><div className="space-y-2"><Label>提供商 *</Label><Select value={pr} onValueChange={setPr}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{PO.map(o => <SelectItem key={o.v} value={o.v}>{o.l}</SelectItem>)}</SelectContent></Select></div></div>
        <div className="space-y-2"><Label>API Base URL</Label><Input value={bu} onChange={e => setBu(e.target.value)} placeholder="留空使用默认" className="font-mono" /></div>
        <div className="space-y-2"><Label>API Key {ed ? "(留空保持不变)" : "*"}</Label><Input type="password" value={ak} onChange={e => setAk(e.target.value)} placeholder={ed ? "留空保持不变" : "sk-..."} className="font-mono" /></div>
      </div>
      <DialogFooter><Button variant="outline" onClick={() => { setSf(false); rf(); }}>取消</Button><Button onClick={hs}>{ed ? "保存" : "创建"}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
