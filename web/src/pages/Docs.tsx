import React from "react";
import { Book, Code, Coins } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import "../i18n";
import { useTranslation } from "react-i18next";

type TabKey = "quickstart" | "api" | "billing";

function QuickstartSection() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div>
        <h3 className="text-lg font-semibold mb-4">{t("docs.qsAuth")}</h3>
        <div className="space-y-4">
          {[
            { s: "1", t: t("docs.qsStep1"), d: t("docs.qsStep1Desc") },
            { s: "2", t: t("docs.qsStep2"), d: t("docs.qsStep2Desc") },
            { s: "3", t: t("docs.qsStep3"), d: t("docs.qsStep3Desc") },
          ].map((i) => (
            <div key={i.s} className="flex gap-3">
              <span className="flex-shrink-0 w-7 h-7 rounded-full bg-[#4F6BED]/10 text-[#4F6BED] flex items-center justify-center text-sm font-bold">
                {i.s}
              </span>
              <div>
                <p className="font-medium">{i.t}</p>
                <p className="text-sm text-muted-foreground">{i.d}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.qsCurl")}</h3>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>{'curl http://localhost:8080/v1/chat/completions \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -d \'{"model":"gpt-4o","messages":[{"role":"user","content":"Hello!"}]}\''}</code>
        </pre>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.qsPython")}</h3>
        <p className="text-sm text-muted-foreground mb-2">
          {t("docs.qsLibHint")} <code className="glass-soft px-1 rounded">openai</code>
        </p>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>{"from openai import OpenAI\n\nclient = OpenAI(\n    base_url=\"http://localhost:8080/v1\",\n    api_key=\"YOUR_API_KEY\"\n)\n\nresponse = client.chat.completions.create(\n    model=\"gpt-4o\",\n    messages=[{\"role\":\"user\",\"content\":\"Hello!\"}]\n)\nprint(response.choices[0].message.content)"}</code>
        </pre>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.qsNode")}</h3>
        <p className="text-sm text-muted-foreground mb-2">
          {t("docs.qsLibHint")} <code className="glass-soft px-1 rounded">openai</code>
        </p>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>{"import OpenAI from \"openai\"\n\nconst client = new OpenAI({\n  baseURL: \"http://localhost:8080/v1\",\n  apiKey: \"YOUR_API_KEY\",\n})\n\nconst response = await client.chat.completions.create({\n  model: \"gpt-4o\",\n  messages: [{ role: \"user\", content: \"Hello!\" }],\n})\nconsole.log(response.choices[0].message.content)"}</code>
        </pre>
      </div>
    </div>
  );
}

function ApiReferenceSection() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.apiAuth")}</h3>
        <p className="text-sm text-muted-foreground mb-3">{t("docs.apiAuthDesc")}</p>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>Authorization: Bearer YOUR_API_KEY</code>
        </pre>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.apiChat")}</h3>
        <div className="glass-soft border-[#4F6BED]/10 rounded-xl p-4 mb-3">
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="success" className="font-mono">POST</Badge>
            <code className="text-sm font-mono">/v1/chat/completions</code>
          </div>
          <p className="text-sm text-muted-foreground">{t("docs.apiChatDesc")}</p>
        </div>
        <h4 className="text-sm font-semibold mb-2">{t("docs.apiReqExample")}</h4>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>{'{\n  "model": "gpt-4o",\n  "messages": [\n    { "role": "user", "content": "Hello!" }\n  ]\n}'}</code>
        </pre>
        <h4 className="text-sm font-semibold mb-2 mt-4">{t("docs.apiRespExample")}</h4>
        <pre className="bg-[#161A23] text-[#E8ECF8] p-4 rounded-xl text-sm overflow-x-auto">
          <code>{'{\n  "id": "chatcmpl-123",\n  "object": "chat.completion",\n  "choices": [\n    { "index": 0, "message": { "role": "assistant", "content": "Hello! How can I help?" } }\n  ]\n}'}</code>
        </pre>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.apiListModels")}</h3>
        <div className="glass-soft border-[#4F6BED]/10 rounded-xl p-4 mb-3">
          <div className="flex items-center gap-2 mb-2">
            <Badge className="font-mono">GET</Badge>
            <code className="text-sm font-mono">/v1/models</code>
          </div>
          <p className="text-sm text-muted-foreground">{t("docs.apiListModelsDesc")}</p>
        </div>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.apiErrors")}</h3>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("docs.errCode")}</TableHead>
              <TableHead>{t("docs.errMeaning")}</TableHead>
              <TableHead>{t("docs.errDesc")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {[
              { code: "401", m: t("docs.err401"), d: t("docs.err401Desc") },
              { code: "403", m: t("docs.err403"), d: t("docs.err403Desc") },
              { code: "429", m: t("docs.err429"), d: t("docs.err429Desc") },
              { code: "500", m: t("docs.err500"), d: t("docs.err500Desc") },
            ].map((e) => (
              <TableRow key={e.code}>
                <TableCell className="font-mono">{e.code}</TableCell>
                <TableCell>{e.m}</TableCell>
                <TableCell className="text-muted-foreground">{e.d}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

function BillingSection() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.billDims")}</h3>
        <p className="text-sm text-muted-foreground mb-4">{t("docs.billDimsDesc")}</p>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("docs.billDim")}</TableHead>
              <TableHead>{t("docs.billDesc")}</TableHead>
              <TableHead>{t("docs.billUnit")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {[
              { dim: "input", desc: t("docs.dimInputDesc"), unit: t("docs.unitPer1K") },
              { dim: "output", desc: t("docs.dimOutputDesc"), unit: t("docs.unitPer1K") },
              { dim: "cache_read", desc: t("docs.dimCacheReadDesc"), unit: t("docs.unitPer1K") },
              { dim: "cache_write", desc: t("docs.dimCacheWriteDesc"), unit: t("docs.unitPer1K") },
              { dim: "reasoning", desc: t("docs.dimReasoningDesc"), unit: t("docs.unitPer1K") },
              { dim: "image", desc: t("docs.dimImageDesc"), unit: t("docs.unitPerImage") },
              { dim: "audio", desc: t("docs.dimAudioDesc"), unit: t("docs.unitPerMinute") },
              { dim: "video", desc: t("docs.dimVideoDesc"), unit: t("docs.unitPerSecond") },
            ].map((d) => (
              <TableRow key={d.dim}>
                <TableCell className="font-mono text-xs">{d.dim}</TableCell>
                <TableCell className="text-muted-foreground">{d.desc}</TableCell>
                <TableCell>{d.unit}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <div>
        <h3 className="text-lg font-semibold mb-3">{t("docs.billWallet")}</h3>
        <p className="text-sm text-muted-foreground mb-4">{t("docs.billWalletDesc")}</p>
        <div className="glass-soft border-[#4F6BED]/10 rounded-xl p-4 space-y-2">
          {[
            [t("docs.bwPeriod"), t("docs.bwPeriodV")],
            [t("docs.bwSnapshot"), t("docs.bwSnapshotV")],
            [t("docs.bwAlert"), t("docs.bwAlertV")],
            [t("docs.bwRecharge"), t("docs.bwRechargeV")],
          ].map(([k, v]) => (
            <div key={k} className="flex justify-between text-sm">
              <span className="text-muted-foreground">{k}</span>
              <span>{v}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export default function Docs() {
  const { t } = useTranslation();
  const tabs = [
    { key: "quickstart" as TabKey, label: t("docs.tabQuickstart"), icon: Book },
    { key: "api" as TabKey, label: t("docs.tabApi"), icon: Code },
    { key: "billing" as TabKey, label: t("docs.tabBilling"), icon: Coins },
  ];
  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("docs.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("docs.subtitle")}</p>
      </div>
      <Tabs defaultValue="quickstart">
        <TabsList className="mb-6">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.key} value={tab.key}>
              <tab.icon size={16} className="mr-2" />
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <Card>
          <CardContent className="p-6">
            <TabsContent value="quickstart">
              <QuickstartSection />
            </TabsContent>
            <TabsContent value="api">
              <ApiReferenceSection />
            </TabsContent>
            <TabsContent value="billing">
              <BillingSection />
            </TabsContent>
          </CardContent>
        </Card>
      </Tabs>
    </div>
  );
}
