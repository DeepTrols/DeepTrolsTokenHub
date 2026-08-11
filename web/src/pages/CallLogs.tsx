import { EmptyState, ErrorState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useMemo, useState } from "react";
import { UsageLog } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { formatAmount } from "../lib/format";
import { ChevronDown, ChevronRight, Key } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";

interface ModelUsage {
  model: string;
  count: number;
  tokens: number;
  cost: number;
}

interface KeyUsage {
  id: string;
  name: string;
  count: number;
  tokens: number;
  cost: number;
  models: ModelUsage[];
}

export default function CallLogs() {
  const { data: usage, isLoading, isError, error, refetch } =
    useConsoleQuery<{ data: UsageLog[] }>("/usage?limit=200");
  const logs = usage?.data ?? [];
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // Group call logs by API key; each key carries totals plus a per-model
  // breakdown. Keys with no logs are naturally absent — this is a records page.
  const keys: KeyUsage[] = useMemo(() => {
    type Acc = Omit<KeyUsage, "models"> & {
      models: Record<string, ModelUsage>;
    };
    const acc: Record<string, Acc> = {};
    for (const l of logs) {
      const id = l.api_key_id || "";
      if (!acc[id]) acc[id] = { id, name: "", count: 0, tokens: 0, cost: 0, models: {} };
      const k = acc[id];
      if (!k.name && l.api_key_name) k.name = l.api_key_name;
      k.count += 1;
      k.tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
      k.cost += parseFloat(l.cost || "0");
      if (!k.models[l.model]) k.models[l.model] = { model: l.model, count: 0, tokens: 0, cost: 0 };
      const m = k.models[l.model];
      m.count += 1;
      m.tokens += (l.input_tokens || 0) + (l.output_tokens || 0);
      m.cost += parseFloat(l.cost || "0");
    }
    return Object.values(acc)
      .map(k => ({
        ...k,
        name: k.name || (k.id ? k.id.slice(0, 8) + "…" : "未知密钥"),
        models: Object.values(k.models).sort((a, b) => b.count - a.count),
      }))
      .sort((a, b) => b.count - a.count);
  }, [logs]);

  if (isLoading) {
    return (
      <div>
        <h2 className="text-2xl font-bold mb-6">调用记录</h2>
        <Card>
          <CardContent className="p-12 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3" />
            <p className="text-muted-foreground">加载调用记录...</p>
          </CardContent>
        </Card>
      </div>
    );
  }
  if (isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>调用记录</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <ErrorState error={error} onRetry={() => refetch()} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold">调用记录</h2>
        <p className="text-sm text-muted-foreground mt-1">按 API 密钥查看调用量与模型分布</p>
      </div>

      {keys.length === 0 && (
        <Card>
          <CardContent className="p-12">
            <EmptyState title="暂无调用记录" />
          </CardContent>
        </Card>
      )}

      <div className="space-y-3">
        {keys.map(k => {
          const isExpanded = expandedId === k.id;
          return (
            <Card key={k.id || k.name} className="overflow-hidden">
              <button
                type="button"
                aria-expanded={isExpanded}
                onClick={() => setExpandedId(isExpanded ? null : k.id)}
                className="w-full flex items-center justify-between gap-4 px-5 py-4 text-left hover:bg-muted/40 transition-colors"
              >
                <span className="flex items-center gap-2.5 min-w-0">
                  <Key size={16} className="text-muted-foreground shrink-0" />
                  <span className="font-medium truncate">{k.name}</span>
                </span>
                <span className="flex items-center gap-4 shrink-0">
                  <span className="text-sm text-muted-foreground whitespace-nowrap">
                    {k.count} 次调用 · {k.tokens.toLocaleString()} tokens · {formatAmount(k.cost)} CNY
                  </span>
                  {isExpanded ? (
                    <ChevronDown size={16} className="text-muted-foreground" />
                  ) : (
                    <ChevronRight size={16} className="text-muted-foreground" />
                  )}
                </span>
              </button>

              {isExpanded && (
                <div className="border-t px-5 py-2">
                  {k.models.map(m => (
                    <div
                      key={m.model}
                      className="flex items-center justify-between gap-4 py-2 text-sm"
                    >
                      <span className="text-muted-foreground truncate">{m.model}</span>
                      <span className="text-muted-foreground/70 whitespace-nowrap">
                        {m.count} 次 · {m.tokens.toLocaleString()} tokens · {formatAmount(m.cost)} CNY
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          );
        })}
      </div>
    </div>
  );
}
