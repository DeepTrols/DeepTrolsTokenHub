import { Fragment, useMemo, useState } from "react";
import { useAdminQuery } from "../lib/hooks/use-api";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatAmount } from "../lib/format";
import { Search, X, Activity, ChevronDown, Landmark } from "lucide-react";

interface ModelUsageRow {
  model: string;
  calls: number;
  tokens: number;
  cost: string;
}

interface LedgerRow {
  id: string;
  email: string;
  display_name: string;
  role: string;
  status: string;
  user_type: string;
  tenant_id?: string;
  tenant_name?: string;
  balance: string;
  frozen: string;
  total_topup: string;
  total_spend: string;
  request_count: number;
  total_tokens: number;
  model_usage: ModelUsageRow[];
}

export default function Finance() {
  const { data, isLoading, isError, error, refetch } = useAdminQuery<{ data: LedgerRow[]; total: number }>("/ledger");
  const rows = data?.data ?? [];
  const total = rows.length;

  const [q, setQ] = useState("");
  const filtered = useMemo(() => {
    if (!q.trim()) return rows;
    const lq = q.toLowerCase();
    return rows.filter(
      (r) =>
        r.email.toLowerCase().includes(lq) ||
        (r.display_name || "").toLowerCase().includes(lq) ||
        (r.tenant_name || "").toLowerCase().includes(lq),
    );
  }, [rows, q]);

  // 展开的行：显示该账号所有调用过的模型及每次调用的聚合（次数/token/费用）。
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const toggleRow = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Header>
          <SectionPageLayout.HeaderBlock>
            <SectionPageLayout.Title>账务管理</SectionPageLayout.Title>
          </SectionPageLayout.HeaderBlock>
        </SectionPageLayout.Header>
        <SectionPageLayout.Content>
          <LoadingState message="加载账务数据..." />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Header>
        <SectionPageLayout.HeaderBlock>
          <SectionPageLayout.Title>账务管理</SectionPageLayout.Title>
          <SectionPageLayout.Description>共 {total} 个账号（个人用户 + 企业用户；企业员工余额与消费已并入企业账号）</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        <div className="mb-4 flex items-center gap-2">
          <div className="relative max-w-sm flex-1">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索账号 / 企业"
              className="flex h-10 w-full glass-soft rounded-xl pl-9 pr-9 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:border-[#4F6BED] focus-visible:ring-2 focus-visible:ring-[#4F6BED]/20 disabled:cursor-not-allowed disabled:opacity-50"
            />
            {q && (
              <button
                type="button"
                onClick={() => setQ("")}
                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
              >
                <X size={14} />
              </button>
            )}
          </div>
        </div>

        {isError && <ErrorState error={error} onRetry={() => refetch()} />}

        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8" />
                <TableHead>账号</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>所属企业</TableHead>
                <TableHead className="text-right">余额</TableHead>
                <TableHead className="text-right">累计充值</TableHead>
                <TableHead className="text-right">累计消费</TableHead>
                <TableHead className="text-right">调用量</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8}>
                    <EmptyState icon={Landmark} title={q ? "未找到" : "暂无账务数据"} />
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((r) => {
                const isOpen = expanded.has(r.id);
                const modelUsage = r.model_usage ?? [];
                return (
                  <Fragment key={r.id}>
                    <TableRow className={isOpen ? "bg-muted/40" : ""}>
                      <TableCell>
                        <button
                          type="button"
                          aria-expanded={isOpen}
                          aria-label="展开模型详情"
                          onClick={() => toggleRow(r.id)}
                          className="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
                        >
                          <ChevronDown size={14} className={`transition-transform ${isOpen ? "" : "-rotate-90"}`} />
                        </button>
                      </TableCell>
                      <TableCell>
                        <p className="font-medium text-sm">{r.email}</p>
                        {r.display_name && <p className="text-xs text-muted-foreground">{r.display_name}</p>}
                      </TableCell>
                      <TableCell>
                        <Badge variant={r.user_type === "enterprise" ? "default" : "secondary"}>
                          {r.user_type === "enterprise" ? "企业" : "个人"}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">{r.tenant_name || "—"}</TableCell>
                      <TableCell className="text-right font-mono text-sm">{formatAmount(r.balance)}</TableCell>
                      <TableCell className="text-right font-mono text-sm">{formatAmount(r.total_topup)}</TableCell>
                      <TableCell className="text-right font-mono text-sm text-orange-500">{formatAmount(r.total_spend)}</TableCell>
                      <TableCell className="text-right text-sm tabular-nums">{r.request_count.toLocaleString()}</TableCell>
                    </TableRow>
                    {isOpen && (
                      <TableRow className="bg-muted/30">
                        <TableCell colSpan={8}>
                          <div className="py-1">
                            <p className="text-xs text-muted-foreground mb-2 flex items-center gap-1">
                              <Activity size={12} />
                              模型调用明细
                            </p>
                            {modelUsage.length === 0 ? (
                              <p className="text-xs text-muted-foreground">暂无调用记录</p>
                            ) : (
                              <Table>
                                <TableHeader>
                                  <TableRow>
                                    <TableHead>模型</TableHead>
                                    <TableHead className="text-right">调用次数</TableHead>
                                    <TableHead className="text-right">Tokens</TableHead>
                                    <TableHead className="text-right">费用</TableHead>
                                  </TableRow>
                                </TableHeader>
                                <TableBody>
                                  {modelUsage.map((m) => (
                                    <TableRow key={m.model}>
                                      <TableCell className="font-mono text-xs">{m.model}</TableCell>
                                      <TableCell className="text-right tabular-nums">{m.calls.toLocaleString()}</TableCell>
                                      <TableCell className="text-right tabular-nums">{m.tokens.toLocaleString()}</TableCell>
                                      <TableCell className="text-right font-mono text-xs">{formatAmount(m.cost)} CNY</TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
