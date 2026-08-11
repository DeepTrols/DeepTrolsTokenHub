import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Book, Code, Box, Coins, Loader2, AlertCircle } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";

type TabKey = "quickstart" | "api" | "models" | "billing";
interface ModelInfo { id: string; object: string; created: number; owned_by: string; }

function QuickstartSection() {
  return <div className="space-y-8">
    <div><h3 className="text-lg font-semibold mb-4">注册与认证</h3>
      <div className="space-y-4">{[{ s: "1", t: "注册账号", d: "访问控制台注册 DeepTrols 账号并登录" }, { s: "2", t: "创建 API 密钥", d: "在「API 密钥」页面创建密钥" }, { s: "3", t: "调用 API", d: "使用下方任意一种语言的示例代码开始调用模型" }].map(i => <div key={i.s} className="flex gap-3"><span className="flex-shrink-0 w-7 h-7 rounded-full bg-primary/10 text-primary flex items-center justify-center text-sm font-bold">{i.s}</span><div><p className="font-medium">{i.t}</p><p className="text-sm text-muted-foreground">{i.d}</p></div></div>)}</div></div>
    <div><h3 className="text-lg font-semibold mb-3">curl 示例</h3><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>{'curl http://localhost:8080/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -d \'{"model":"gpt-4o","messages":[{"role":"user","content":"Hello!"}]}\''}</code></pre></div>
    <div><h3 className="text-lg font-semibold mb-3">Python 示例</h3><p className="text-sm text-muted-foreground mb-2">使用 <code className="bg-muted px-1 rounded">openai</code> 库：</p><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>{"from openai import OpenAI\n\nclient = OpenAI(\n    base_url=\"http://localhost:8080/v1\",\n    api_key=\"YOUR_API_KEY\"\n)\n\nresponse = client.chat.completions.create(\n    model=\"gpt-4o\",\n    messages=[{\"role\":\"user\",\"content\":\"Hello!\"}]\n)\nprint(response.choices[0].message.content)"}</code></pre></div>
    <div><h3 className="text-lg font-semibold mb-3">Node.js 示例</h3><p className="text-sm text-muted-foreground mb-2">使用 <code className="bg-muted px-1 rounded">openai</code> 库：</p><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>{"import OpenAI from \"openai\"\n\nconst client = new OpenAI({\n  baseURL: \"http://localhost:8080/v1\",\n  apiKey: \"YOUR_API_KEY\",\n})\n\nconst response = await client.chat.completions.create({\n  model: \"gpt-4o\",\n  messages: [{ role: \"user\", content: \"Hello!\" }],\n})\nconsole.log(response.choices[0].message.content)"}</code></pre></div>
  </div>;
}

function ApiReferenceSection() {
  return <div className="space-y-8">
    <div><h3 className="text-lg font-semibold mb-3">认证方式</h3><p className="text-sm text-muted-foreground mb-3">所有 API 请求需要在请求头中携带 API 密钥：</p><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>Authorization: Bearer YOUR_API_KEY</code></pre></div>
    <div><h3 className="text-lg font-semibold mb-3">Chat Completions</h3><div className="bg-muted border rounded-lg p-4 mb-3"><div className="flex items-center gap-2 mb-2"><Badge className="bg-emerald-100 text-emerald-700 hover:bg-emerald-100 font-mono">POST</Badge><code className="text-sm font-mono">/v1/chat/completions</code></div><p className="text-sm text-muted-foreground">兼容 OpenAI Chat Completions API，支持流式和非流式调用。</p></div>
      <h4 className="text-sm font-semibold mb-2">请求示例</h4><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>{'{\n  "model": "gpt-4o",\n  "messages": [\n    { "role": "user", "content": "Hello!" }\n  ]\n}'}</code></pre>
      <h4 className="text-sm font-semibold mb-2 mt-4">响应示例</h4><pre className="bg-gray-900 text-gray-100 p-4 rounded-lg text-sm overflow-x-auto"><code>{'{\n  "id": "chatcmpl-123",\n  "object": "chat.completion",\n  "choices": [\n    { "index": 0, "message": { "role": "assistant", "content": "Hello! How can I help?" } }\n  ]\n}'}</code></pre>
    </div>
    <div><h3 className="text-lg font-semibold mb-3">List Models</h3><div className="bg-muted border rounded-lg p-4 mb-3"><div className="flex items-center gap-2 mb-2"><Badge className="bg-blue-100 text-blue-700 hover:bg-blue-100 font-mono">GET</Badge><code className="text-sm font-mono">/v1/models</code></div><p className="text-sm text-muted-foreground">获取当前可用的模型列表。</p></div></div>
    <div><h3 className="text-lg font-semibold mb-3">错误码</h3>
      <Table><TableHeader><TableRow><TableHead>错误码</TableHead><TableHead>含义</TableHead><TableHead>说明</TableHead></TableRow></TableHeader>
        <TableBody>{[{ code: "401", m: "未认证", d: "API 密钥缺失或无效" }, { code: "403", m: "无权限", d: "API 密钥无权限访问该模型" }, { code: "429", m: "速率限制", d: "请求频率超过限额" }, { code: "500", m: "服务器错误", d: "上游服务异常" }].map(e => <TableRow key={e.code}><TableCell className="font-mono">{e.code}</TableCell><TableCell>{e.m}</TableCell><TableCell className="text-muted-foreground">{e.d}</TableCell></TableRow>)}</TableBody></Table>
    </div>
  </div>;
}

function ModelsSection() {
  const { data: modelsData, isLoading, isError, error } = useQuery<ModelInfo[]>({ queryKey: ["gateway", "models", "demo"], queryFn: async () => { const res = await fetch("/v1/models", { headers: { "Content-Type": "application/json", Authorization: "Bearer demo" } }); if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error((err as { error?: { message?: string } }).error?.message || "Failed to fetch models"); } const d = await res.json() as { data?: ModelInfo[] }; return d.data || []; } });
  const models = modelsData ?? [];
  return <div className="space-y-4">
    <h3 className="text-lg font-semibold mb-3">可用模型</h3><p className="text-sm text-muted-foreground">以下模型通过 GET /v1/models 接口获取。</p>
    {isLoading && <div className="flex items-center gap-2 text-sm text-muted-foreground py-8 justify-center"><Loader2 size={18} className="animate-spin" />加载中...</div>}
    {isError && <div className="p-4 bg-destructive/10 border border-destructive/20 rounded-lg"><div className="flex items-start gap-2"><AlertCircle size={16} className="text-destructive mt-0.5" /><div><p className="text-sm font-medium text-destructive">加载模型列表失败</p><p className="text-xs mt-1">{error instanceof Error ? error.message : String(error)}</p></div></div></div>}
    {!isLoading && !isError && models.length === 0 && <div className="text-center py-12 text-muted-foreground"><Box size={40} className="mx-auto mb-3 opacity-30" /><p>暂无可用模型</p></div>}
    {!isLoading && !isError && models.length > 0 && <Table><TableHeader><TableRow><TableHead>模型 ID</TableHead><TableHead>提供商</TableHead></TableRow></TableHeader><TableBody>{models.map(m => <TableRow key={m.id}><TableCell className="font-mono text-sm">{m.id}</TableCell><TableCell>{m.owned_by}</TableCell></TableRow>)}</TableBody></Table>}
  </div>;
}

function BillingSection() {
  return <div className="space-y-8">
    <div><h3 className="text-lg font-semibold mb-3">计费维度</h3><p className="text-sm text-muted-foreground mb-4">DeepTrols 按多种维度精确计费：</p>
      <Table><TableHeader><TableRow><TableHead>维度</TableHead><TableHead>说明</TableHead><TableHead>计费单位</TableHead></TableRow></TableHeader>
        <TableBody>{[{ dim: "input", desc: "输入 Token（用户提示词）", unit: "每 1K tokens" }, { dim: "output", desc: "输出 Token（模型响应）", unit: "每 1K tokens" }, { dim: "cache_read", desc: "缓存命中时读取的 Token", unit: "每 1K tokens" }, { dim: "cache_write", desc: "写入缓存的 Token", unit: "每 1K tokens" }, { dim: "reasoning", desc: "推理过程的 Token", unit: "每 1K tokens" }, { dim: "image", desc: "图片生成/处理", unit: "每张" }, { dim: "audio", desc: "音频处理", unit: "每分钟" }, { dim: "video", desc: "视频生成/处理", unit: "每秒" }].map(d => <TableRow key={d.dim}><TableCell className="font-mono text-xs">{d.dim}</TableCell><TableCell className="text-muted-foreground">{d.desc}</TableCell><TableCell>{d.unit}</TableCell></TableRow>)}</TableBody></Table>
    </div>
    <div><h3 className="text-lg font-semibold mb-3">钱包余额</h3><p className="text-sm text-muted-foreground mb-4">所有 API 调用费用从您的钱包余额中扣除。</p><div className="bg-muted border rounded-lg p-4 space-y-2">{[["计费周期", "实时扣费，每次调用后立即结算"], ["定价快照", "调用时按当前定价快照计费"], ["余额预警", "在「钱包账单」页面可设置预警阈值"], ["充值方式", "在「钱包账单」页面进行充值操作"]].map(([k, v]) => <div key={k} className="flex justify-between text-sm"><span className="text-muted-foreground">{k}</span><span>{v}</span></div>)}</div></div>
  </div>;
}

const tabs = [{ key: "quickstart" as TabKey, label: "快速开始", icon: Book }, { key: "api", label: "API 参考", icon: Code }, { key: "models", label: "模型列表", icon: Box }, { key: "billing", label: "计费说明", icon: Coins }];

export default function Docs() {
  return (
    <div>
      <div className="mb-6"><h2 className="text-2xl font-bold">开发文档</h2><p className="text-sm text-muted-foreground mt-1">集成 DeepTrols AI 模型聚合平台的完整开发指南</p></div>
      <Tabs defaultValue="quickstart">
        <TabsList className="mb-6">{tabs.map(t => <TabsTrigger key={t.key} value={t.key}><t.icon size={16} className="mr-2" />{t.label}</TabsTrigger>)}</TabsList>
        <Card><CardContent className="p-6">
          <TabsContent value="quickstart"><QuickstartSection /></TabsContent>
          <TabsContent value="api"><ApiReferenceSection /></TabsContent>
          <TabsContent value="models"><ModelsSection /></TabsContent>
          <TabsContent value="billing"><BillingSection /></TabsContent>
        </CardContent></Card>
      </Tabs>
    </div>
  );
}
