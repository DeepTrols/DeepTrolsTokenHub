import { useState } from "react";
import { adminApi } from "../lib/api";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Plus, Pencil, Trash2, Play, RefreshCw, ListOrdered, History, Loader2 } from "lucide-react";

interface Connector {
  id: string;
  name: string;
  type: string;
  base_url: string;
  status: string;
  schedule_interval_minutes: number;
  config: Record<string, string>;
  credentials_configured: boolean;
  credential_fields: string[];
  last_sync_status: string;
  last_sync_message: string;
  last_sync_at?: string;
  next_sync_at?: string;
}

interface SyncRun {
  id: string;
  trigger: string;
  status: string;
  pages_fetched: number;
  records_seen: number;
  records_inserted: number;
  records_updated: number;
  error_code: string;
  error_message: string;
  started_at: string;
  finished_at?: string;
}

interface BillingRecord {
  id: string;
  external_id: string;
  model: string;
  currency: string;
  net_amount: string;
  usage_quantity: number;
  usage_unit: string;
  usage_start_at: string;
  external_request_id: string;
}

const TYPE_OPTIONS = [
  { value: "aliyun", label: "阿里云账单" },
  { value: "newapi", label: "NewAPI" },
  { value: "oneapi", label: "OneAPI" },
];

export default function BillingSync() {
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: Connector[] }>("/billing/connectors");
  const connectors = data?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";

  const saveMut = useAdminMutation<unknown, Record<string, unknown>>("post", "/billing/connectors", "/billing/connectors");
  const updateMut = useAdminMutation<unknown, { id: string } & Record<string, unknown>>("put", (v) => `/billing/connectors/${v.id}`, "/billing/connectors");
  const deleteMut = useAdminMutation<unknown, { id: string }>("delete", (v) => `/billing/connectors/${v.id}`, "/billing/connectors");
  const testMut = useAdminMutation<{ ok?: boolean; sample_records?: number }, { id: string }>("post", (v) => `/billing/connectors/${v.id}/test`);
  const syncMut = useAdminMutation<SyncRun, { id: string }>("post", (v) => `/billing/connectors/${v.id}/sync`);

  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Connector | null>(null);
  const [name, setName] = useState("");
  const [type, setType] = useState("aliyun");
  const [baseURL, setBaseURL] = useState("");
  const [schedule, setSchedule] = useState("60");
  const [configJSON, setConfigJSON] = useState("{}");
  const [credentialsJSON, setCredentialsJSON] = useState("{}");
  const [feedback, setFeedback] = useState<{ kind: "ok" | "err"; text: string } | null>(null);

  const [detail, setDetail] = useState<{ kind: "records" | "runs"; connectorId: string; title: string } | null>(null);
  const [detailData, setDetailData] = useState<SyncRun[] | BillingRecord[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);

  const openCreate = () => {
    setEditing(null);
    setName(""); setType("aliyun"); setBaseURL(""); setSchedule("60");
    setConfigJSON("{}"); setCredentialsJSON("{}"); setFeedback(null);
    setOpen(true);
  };
  const openEdit = (c: Connector) => {
    setEditing(c);
    setName(c.name); setType(c.type); setBaseURL(c.base_url); setSchedule(String(c.schedule_interval_minutes));
    setConfigJSON(JSON.stringify(c.config ?? {}, null, 2));
    setCredentialsJSON("{}"); setFeedback(null);
    setOpen(true);
  };

  const parseJSON = (raw: string, field: string): Record<string, string> | null => {
    try {
      const v = JSON.parse(raw || "{}");
      if (v && typeof v === "object" && !Array.isArray(v)) return v;
      setFeedback({ kind: "err", text: `${field} 必须是 JSON 对象` });
      return null;
    } catch {
      setFeedback({ kind: "err", text: `${field} JSON 解析失败` });
      return null;
    }
  };

  const handleSave = async () => {
    if (!name.trim() || !baseURL.trim()) { setFeedback({ kind: "err", text: "名称与 Base URL 必填" }); return; }
    const config = parseJSON(configJSON, "config");
    const credentials = parseJSON(credentialsJSON, "credentials");
    if (config === null || credentials === null) return;
    const body: Record<string, unknown> = {
      name: name.trim(), type, base_url: baseURL.trim(), schedule_interval_minutes: Number(schedule) || 0,
      config, credentials,
    };
    if (editing) await updateMut.mutateAsync({ id: editing.id, ...body });
    else await saveMut.mutateAsync(body);
    setOpen(false);
    refetch();
  };

  const loadDetail = async (kind: "records" | "runs", c: Connector) => {
    setDetail({ kind, connectorId: c.id, title: kind === "records" ? `${c.name} · 账单记录` : `${c.name} · 同步历史` });
    setDetailLoading(true);
    try {
      const res = await adminApi.get<{ data: SyncRun[] | BillingRecord[] }>(
        kind === "records" ? `/billing/connectors/${c.id}/records` : `/billing/connectors/${c.id}/runs`,
      );
      setDetailData(res.data ?? []);
    } catch {
      setDetailData([]);
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="font-display text-[25px] font-bold tracking-tight">账单同步</h2>
          <p className="text-[13px] text-[#5C6472] mt-1">OneAPI / NewAPI / 阿里云账单拉取，供对账 L3 使用</p>
        </div>
        <Button onClick={openCreate}><Plus size={16} className="mr-1.5" />添加连接器</Button>
      </div>

      {feedback && <Card className="mb-4 border-destructive/20"><CardContent className="p-3 text-sm text-destructive">{feedback.text}</CardContent></Card>}
      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm">{loadError}</CardContent></Card>}
      {isLoading && <Card><CardContent className="p-12 text-center text-muted-foreground">加载中...</CardContent></Card>}

      {!isLoading && connectors.length === 0 && (
        <Card><CardContent className="p-12 text-center text-muted-foreground">暂无账单连接器，点击「添加连接器」接入第一家</CardContent></Card>
      )}

      <div className="space-y-4">
        {connectors.map((c) => (
          <Card key={c.id}>
            <CardContent className="p-5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <h3 className="font-semibold">{c.name}</h3>
                  <Badge variant="secondary">{TYPE_OPTIONS.find((t) => t.value === c.type)?.label ?? c.type}</Badge>
                  <Badge variant={c.status === "active" ? "success" : "secondary"}>{c.status === "active" ? "启用" : "停用"}</Badge>
                  {c.credentials_configured && <Badge>凭据已配置</Badge>}
                  {c.last_sync_status && <Badge variant={c.last_sync_status === "succeeded" ? "success" : "destructive"}>{c.last_sync_status}</Badge>}
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={async () => {
                    try {
                      const r = await testMut.mutateAsync({ id: c.id });
                      setFeedback({ kind: "ok", text: `测试通过，样例记录 ${r.sample_records ?? 0} 条` });
                    } catch (e) {
                      setFeedback({ kind: "err", text: e instanceof Error ? e.message : "测试失败" });
                    }
                  }} disabled={testMut.isPending}>
                    {testMut.isPending ? <Loader2 size={14} className="mr-1 animate-spin" /> : <Play size={14} className="mr-1" />}测试
                  </Button>
                  <Button variant="outline" size="sm" onClick={async () => {
                    try {
                      const run = await syncMut.mutateAsync({ id: c.id });
                      setFeedback({ kind: "ok", text: `同步完成：${run.status}，插入 ${run.records_inserted} / 更新 ${run.records_updated}` });
                      refetch();
                    } catch (e) {
                      setFeedback({ kind: "err", text: e instanceof Error ? e.message : "同步失败" });
                    }
                  }} disabled={syncMut.isPending}>
                    {syncMut.isPending ? <Loader2 size={14} className="mr-1 animate-spin" /> : <RefreshCw size={14} className="mr-1" />}同步
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => loadDetail("records", c)}><ListOrdered size={14} className="mr-1" />记录</Button>
                  <Button variant="outline" size="sm" onClick={() => loadDetail("runs", c)}><History size={14} className="mr-1" />历史</Button>
                  <Button variant="outline" size="sm" onClick={() => openEdit(c)}><Pencil size={14} className="mr-1" />编辑</Button>
                  <Button variant="outline" size="sm" className="border-destructive/30 text-destructive hover:bg-destructive/10"
                    onClick={async () => { if (confirm(`删除连接器「${c.name}」及其同步数据？`)) { await deleteMut.mutateAsync({ id: c.id }); refetch(); } }}>
                    <Trash2 size={14} className="mr-1" />删除
                  </Button>
                </div>
              </div>
              <p className="text-xs text-muted-foreground mt-2 font-mono">{c.base_url}</p>
              <p className="text-xs text-muted-foreground mt-1">
                周期 {c.schedule_interval_minutes} 分钟 · 上次同步 {c.last_sync_at || "—"} · 下次 {c.next_sync_at || "—"}
                {c.last_sync_message && ` · ${c.last_sync_message}`}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{editing ? "编辑连接器" : "添加连接器"}</DialogTitle>
            <DialogDescription>配置上游账单源；凭据仅保存在服务端（加密）</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>名称 *</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="阿里云生产账单" />
              </div>
              <div className="space-y-2">
                <Label>类型</Label>
                <Select value={type} onValueChange={setType}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>{TYPE_OPTIONS.map((t) => <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>)}</SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-2">
              <Label>Base URL *</Label>
              <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="https://..." className="font-mono" />
            </div>
            <div className="space-y-2">
              <Label>同步周期（分钟）</Label>
              <Input type="number" value={schedule} onChange={(e) => setSchedule(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>Config（JSON，可选）</Label>
              <textarea value={configJSON} onChange={(e) => setConfigJSON(e.target.value)} rows={3}
                className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none" placeholder='{"product_code":"dbaudit"}' />
            </div>
            <div className="space-y-2">
              <Label>凭据（JSON）</Label>
              <textarea value={credentialsJSON} onChange={(e) => setCredentialsJSON(e.target.value)} rows={3}
                className="w-full px-3 py-2 glass-soft rounded-xl text-xs font-mono focus:outline-none"
                placeholder={type === "aliyun" ? '{"access_key_id":"...","access_key_secret":"..."}' : '{"access_token":"..."}'} />
            </div>
            <div className="flex justify-end gap-2 pt-2 border-t">
              <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
              <Button onClick={handleSave} disabled={saveMut.isPending || updateMut.isPending}>保存</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={detail !== null} onOpenChange={(open) => { if (!open) setDetail(null); }}>
        <DialogContent className="max-w-3xl max-h-[80vh] overflow-auto">
          <DialogHeader>
            <DialogTitle>{detail?.title}</DialogTitle>
          </DialogHeader>
          {detailLoading ? (
            <p className="text-center py-8 text-muted-foreground">加载中...</p>
          ) : detail?.kind === "records" ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>外部 ID</TableHead>
                  <TableHead>模型</TableHead>
                  <TableHead className="text-right">金额</TableHead>
                  <TableHead className="text-right">用量</TableHead>
                  <TableHead>请求 ID</TableHead>
                  <TableHead>时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(detailData as BillingRecord[]).length === 0 && <TableRow><TableCell colSpan={6} className="py-8 text-center text-muted-foreground">暂无记录</TableCell></TableRow>}
                {(detailData as BillingRecord[]).map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className="font-mono text-xs">{r.external_id}</TableCell>
                    <TableCell>{r.model || "—"}</TableCell>
                    <TableCell className="text-right font-mono text-xs">{r.net_amount} {r.currency}</TableCell>
                    <TableCell className="text-right tabular-nums">{r.usage_quantity} {r.usage_unit}</TableCell>
                    <TableCell className="font-mono text-xs">{r.external_request_id || "—"}</TableCell>
                    <TableCell className="text-xs">{r.usage_start_at?.slice(0, 16)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>触发</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className="text-right">页数</TableHead>
                  <TableHead className="text-right">记录</TableHead>
                  <TableHead className="text-right">插入/更新</TableHead>
                  <TableHead>错误</TableHead>
                  <TableHead>开始</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(detailData as SyncRun[]).length === 0 && <TableRow><TableCell colSpan={7} className="py-8 text-center text-muted-foreground">暂无同步记录</TableCell></TableRow>}
                {(detailData as SyncRun[]).map((run) => (
                  <TableRow key={run.id}>
                    <TableCell>{run.trigger}</TableCell>
                    <TableCell><Badge variant={run.status === "succeeded" ? "success" : "destructive"}>{run.status}</Badge></TableCell>
                    <TableCell className="text-right tabular-nums">{run.pages_fetched}</TableCell>
                    <TableCell className="text-right tabular-nums">{run.records_seen}</TableCell>
                    <TableCell className="text-right tabular-nums">{run.records_inserted}/{run.records_updated}</TableCell>
                    <TableCell className="text-xs text-destructive max-w-[180px] truncate">{run.error_message || run.error_code || "—"}</TableCell>
                    <TableCell className="text-xs">{run.started_at?.slice(0, 16)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
