import { Fragment, useMemo, useState } from "react";
import { useAdminQuery } from "../lib/hooks/use-api";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatAmount } from "../lib/format";
import {
  Search,
  X,
  Wallet as WalletIcon,
  TrendingUp,
  TrendingDown,
  Activity,
  ChevronDown,
  Landmark,
} from "lucide-react";

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
  top_models: string[];
}

function statusVariant(s: string): "success" | "destructive" | "secondary" {
  if (s === "active") return "success";
  if (s === "banned") return "destructive";
  return "secondary";
}
function statusLabel(s: string): string {
  if (s === "active") return "正常";
  if (s === "banned") return "已封禁";
  return s;
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

  // 汇总卡片：总体指标。
  const summary = useMemo(() => {
    let balance = 0;
    let topup = 0;
    let spend = 0;
    let calls = 0;
    for (const r of rows) {
      balance += Number(r.balance) || 0;
      topup += Number(r.total_topup) || 0;
      spend += Number(r.total_spend) || 0;
      calls += r.request_count || 0;
    }
    return {
      balance: formatAmount(balance.toFixed(2)),
      topup: formatAmount(topup.toFixed(2)),
      spend: formatAmount(spend.toFixed(2)),
      calls,
    };
  }, [rows]);

  // 展开的行：显示该账号 Top-3 模型。
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
          <SectionPageLayout.Description>共 {total} 个账号（个人 + 企业）</SectionPageLayout.Description>
        </SectionPageLayout.HeaderBlock>
      </SectionPageLayout.Header>

      <SectionPageLayout.Content>
        {/* 总体指标 */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">总余额</CardTitle>
              <WalletIcon size={16} className="text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-bold tabular-nums">{summary.balance} CNY</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">累计充值</CardTitle>
              <TrendingUp size={16} className="text-green-500" />
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-bold tabular-nums">{summary.topup} CNY</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">累计消费</CardTitle>
              <TrendingDown size={16} className="text-orange-500" />
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-bold tabular-nums">{summary.spend} CNY</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">总调用量</CardTitle>
              <Activity size={16} className="text-blue-500" />
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-bold tabular-nums">{summary.calls.toLocaleString()}</p>
            </CardContent>
          </Card>
        </div>

        <div className="mb-4 flex items-center gap-2">
          <div className="relative max-w-sm flex-1">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索账号 / 企业"
              className="flex h-9 w-full rounded-md border border-input bg-background pl-9 pr-9 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
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
                return (
                  <Fragment key={r.id}>
                    <TableRow className={isOpen ? "bg-muted/40" : ""}>
                      <TableCell>
                        <button
                          type="button"
                          aria-expanded={isOpen}
                          aria-label="展开 Top 模型"
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
                            <p className="text-xs text-muted-foreground mb-1.5 flex items-center gap-1">
                              <Activity size={12} />
                              调用最多的模型（Top {r.top_models.length ? Math.min(r.top_models.length, 3) : 0}）
                            </p>
                            {r.top_models.length === 0 ? (
                              <p className="text-xs text-muted-foreground">暂无调用记录</p>
                            ) : (
                              <div className="flex flex-wrap gap-1.5">
                                {r.top_models.map((m) => (
                                  <Badge key={m} variant="secondary" className="font-mono text-xs">
                                    {m}
                                  </Badge>
                                ))}
                              </div>
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
