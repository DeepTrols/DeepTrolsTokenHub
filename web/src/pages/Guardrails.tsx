import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { ShieldAlert, Plus, Pencil, Trash2, Loader2 } from "lucide-react";

interface DetectionItem {
  id?: string;
  name: string;
  detector_type: string;
  action: string;
  config?: Record<string, unknown>;
}

interface Binding {
  id?: string;
  scope_type: string;
  scope_id: string;
  checkpoint: string;
  protocol: string;
}

interface PolicyData {
  id: string;
  name: string;
  description: string;
  status: string;
  detection_items: DetectionItem[];
  bindings: Binding[];
}

const DETECTOR_OPTIONS = [
  { value: "pattern", label: "关键词/正则" },
  { value: "sensitive_data", label: "敏感数据" },
  { value: "model", label: "模型检测" },
];
const ACTION_OPTIONS = [
  { value: "block", label: "拦截" },
  { value: "audit", label: "仅审计" },
  { value: "mask", label: "脱敏" },
];

interface ItemDraft { name: string; detector_type: string; action: string; keywords: string }
interface BindingDraft { scope_type: string; scope_id: string; checkpoint: string; protocol: string }

function itemToDraft(item: DetectionItem): ItemDraft {
  const kws = item.config?.keywords;
  const joined = Array.isArray(kws) ? kws.join(",") : "";
  return { name: item.name, detector_type: item.detector_type, action: item.action, keywords: joined };
}

function draftToItems(drafts: ItemDraft[]): DetectionItem[] {
  return drafts
    .filter((d) => d.name.trim() && d.keywords.trim())
    .map((d) => ({
      name: d.name.trim(),
      detector_type: d.detector_type,
      action: d.action,
      config: { keywords: d.keywords.split(",").map((s) => s.trim()).filter(Boolean) },
    }));
}

export default function Guardrails() {
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: PolicyData[] }>("/guardrails");
  const policies = data?.data ?? [];

  const saveMut = useAdminMutation<unknown, Record<string, unknown>>("post", "/guardrails", "/guardrails");
  const deleteMut = useAdminMutation<unknown, { id: string }>("delete", (v) => `/guardrails/${v.id}`, "/guardrails");

  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<PolicyData | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [status, setStatus] = useState("active");
  const [items, setItems] = useState<ItemDraft[]>([{ name: "", detector_type: "pattern", action: "block", keywords: "" }]);
  const [bindings, setBindings] = useState<BindingDraft[]>([{ scope_type: "all_projects", scope_id: "", checkpoint: "before_provider", protocol: "all" }]);

  const openCreate = () => {
    setEditing(null);
    setName(""); setDescription(""); setStatus("active");
    setItems([{ name: "", detector_type: "pattern", action: "block", keywords: "" }]);
    setBindings([{ scope_type: "all_projects", scope_id: "", checkpoint: "before_provider", protocol: "all" }]);
    setOpen(true);
  };
  const openEdit = (p: PolicyData) => {
    setEditing(p);
    setName(p.name); setDescription(p.description); setStatus(p.status);
    setItems(p.detection_items.length ? p.detection_items.map(itemToDraft) : [{ name: "", detector_type: "pattern", action: "block", keywords: "" }]);
    setBindings(p.bindings.length ? p.bindings.map((b) => ({ scope_type: b.scope_type, scope_id: b.scope_id, checkpoint: b.checkpoint, protocol: b.protocol })) : [{ scope_type: "all_projects", scope_id: "", checkpoint: "before_provider", protocol: "all" }]);
    setOpen(true);
  };

  const handleSave = async () => {
    if (!name.trim()) return;
    await saveMut.mutateAsync({
      id: editing?.id,
      name: name.trim(),
      description,
      status,
      detection_items: draftToItems(items),
      bindings: bindings.map((b) => ({ scope_type: b.scope_type, scope_id: b.scope_id, checkpoint: b.checkpoint, protocol: b.protocol })),
    });
    setOpen(false);
    refetch();
  };

  const setItem = (i: number, patch: Partial<ItemDraft>) => {
    setItems((prev) => prev.map((it, idx) => (idx === i ? { ...it, ...patch } : it)));
  };

  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">内容策略</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">出站内容拦截（关键词/正则/敏感数据），网关调用前生效</p>
        </div>
        <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />新建策略</Button>
      </div>

      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">{loadError}</CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center text-muted-foreground">加载中...</CardContent></Card>}

      {!isLoading && policies.length === 0 && (
        <Card><CardContent className="p-12 text-center text-muted-foreground flex flex-col items-center gap-2">
          <ShieldAlert size={32} className="opacity-30" />
          <p>暂无内容策略，点击「新建策略」创建第一条</p>
        </CardContent></Card>
      )}

      <div className="space-y-4">
        {policies.map((p) => (
          <Card key={p.id}>
            <CardContent className="p-5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <h3 className="font-semibold">{p.name}</h3>
                  <Badge variant={p.status === "active" ? "success" : "secondary"}>{p.status === "active" ? "启用" : "停用"}</Badge>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => openEdit(p)}><Pencil size={14} className="mr-1" />编辑</Button>
                  <Button variant="outline" size="sm" className="border-destructive/30 text-destructive hover:bg-destructive/10"
                    onClick={async () => { if (confirm(`删除策略「${p.name}」？`)) { await deleteMut.mutateAsync({ id: p.id }); refetch(); } }}>
                    <Trash2 size={14} className="mr-1" />删除
                  </Button>
                </div>
              </div>
              {p.description && <p className="text-xs text-muted-foreground mt-1">{p.description}</p>}
              <div className="mt-3 space-y-1 text-xs text-muted-foreground">
                {p.detection_items.map((it) => (
                  <div key={it.id || it.name}>检测项：{it.name}（{it.detector_type} · {it.action}）</div>
                ))}
                <div>绑定：{p.bindings.map((b) => `${b.scope_type}${b.scope_id ? ":" + b.scope_id : ""}@${b.checkpoint}/${b.protocol}`).join("；") || "无"}</div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-auto">
          <DialogHeader>
            <DialogTitle>{editing ? "编辑策略" : "新建策略"}</DialogTitle>
            <DialogDescription>配置检测项（关键词用逗号分隔）与绑定范围</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>名称 *</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="敏感词拦截" />
              </div>
              <div className="space-y-2">
                <Label>状态</Label>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">启用</SelectItem>
                    <SelectItem value="disabled">停用</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-2">
              <Label>描述</Label>
              <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="可选" />
            </div>

            <div className="space-y-2">
              <Label>检测项</Label>
              {items.map((it, i) => (
                <div key={i} className="grid grid-cols-[1fr_120px_100px_1fr] gap-2 items-center">
                  <Input value={it.name} onChange={(e) => setItem(i, { name: e.target.value })} placeholder="检测项名" />
                  <Select value={it.detector_type} onValueChange={(v) => setItem(i, { detector_type: v })}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{DETECTOR_OPTIONS.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent>
                  </Select>
                  <Select value={it.action} onValueChange={(v) => setItem(i, { action: v })}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{ACTION_OPTIONS.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}</SelectContent>
                  </Select>
                  <Input value={it.keywords} onChange={(e) => setItem(i, { keywords: e.target.value })} placeholder="关键词，逗号分隔" />
                </div>
              ))}
              <Button variant="outline" size="sm" onClick={() => setItems((prev) => [...prev, { name: "", detector_type: "pattern", action: "block", keywords: "" }])}>
                <Plus size={14} className="mr-1" />添加检测项
              </Button>
            </div>

            <div className="space-y-2">
              <Label>绑定</Label>
              {bindings.map((b, i) => (
                <div key={i} className="grid grid-cols-[140px_1fr_140px_80px] gap-2 items-center">
                  <Select value={b.scope_type} onValueChange={(v) => setBindings((prev) => prev.map((x, idx) => idx === i ? { ...x, scope_type: v } : x))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all_projects">全部租户</SelectItem>
                      <SelectItem value="project">指定租户</SelectItem>
                    </SelectContent>
                  </Select>
                  <Input value={b.scope_id} onChange={(e) => setBindings((prev) => prev.map((x, idx) => idx === i ? { ...x, scope_id: e.target.value } : x))}
                    placeholder="租户 ID（全部租户时留空）" disabled={b.scope_type === "all_projects"} />
                  <Select value={b.checkpoint} onValueChange={(v) => setBindings((prev) => prev.map((x, idx) => idx === i ? { ...x, checkpoint: v } : x))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem value="before_provider">上游前</SelectItem></SelectContent>
                  </Select>
                  <Select value={b.protocol} onValueChange={(v) => setBindings((prev) => prev.map((x, idx) => idx === i ? { ...x, protocol: v } : x))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem value="all">全协议</SelectItem></SelectContent>
                  </Select>
                </div>
              ))}
            </div>

            <div className="flex justify-end gap-2 pt-2 border-t">
              <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
              <Button onClick={handleSave} disabled={saveMut.isPending || !name.trim()}>
                {saveMut.isPending && <Loader2 size={14} className="mr-1.5 animate-spin" />}保存
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
