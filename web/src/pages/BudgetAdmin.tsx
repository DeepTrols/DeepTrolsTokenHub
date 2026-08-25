import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Wallet, Check, X } from "lucide-react";

interface BudgetRow {
  id: string;
  tenant_name: string;
  period: string;
  limit_amount: string;
  spent_amount: string;
  status: string;
}

interface BudgetRequestRow {
  id: string;
  tenant_name: string;
  requested_amount: string;
  reason: string;
  status: string;
  created_at: string;
}

export default function BudgetAdmin() {
  const budgets = useAdminQuery<{ data: BudgetRow[] }>("/budgets");
  const requests = useAdminQuery<{ data: BudgetRequestRow[] }>("/budgets/requests");
  const approveMut = useAdminMutation<unknown, { id: string }>("post", (v) => `/budgets/requests/${v.id}/approve`, "/budgets/requests");
  const rejectMut = useAdminMutation<unknown, { id: string }>("post", (v) => `/budgets/requests/${v.id}/reject`, "/budgets/requests");

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      requests.refetch();
      budgets.refetch();
    } catch { /* mutation surfaces error */ }
  };

  return (
    <div>
      <div className="mb-5">
        <h2 className="font-display text-[25px] font-bold tracking-tight">预算管理</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">企业月度预算与加额申请审批</p>
      </div>

      <Card className="mb-6 overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>企业</TableHead>
              <TableHead>周期</TableHead>
              <TableHead className="text-right">限额</TableHead>
              <TableHead className="text-right">已用</TableHead>
              <TableHead>状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(budgets.data?.data ?? []).length === 0 && (
              <TableRow><TableCell colSpan={5} className="py-10 text-center text-muted-foreground flex flex-col items-center gap-2">
                <Wallet size={28} className="opacity-30" />暂无预算
              </TableCell></TableRow>
            )}
            {(budgets.data?.data ?? []).map((b) => (
              <TableRow key={b.id}>
                <TableCell className="font-medium">{b.tenant_name}</TableCell>
                <TableCell>{b.period}</TableCell>
                <TableCell className="text-right font-mono">{b.limit_amount} CNY</TableCell>
                <TableCell className="text-right font-mono">{b.spent_amount} CNY</TableCell>
                <TableCell><Badge variant={b.status === "active" ? "success" : "secondary"}>{b.status}</Badge></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>企业</TableHead>
              <TableHead className="text-right">申请金额</TableHead>
              <TableHead>原因</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(requests.data?.data ?? []).length === 0 && (
              <TableRow><TableCell colSpan={5} className="py-10 text-center text-muted-foreground">暂无加额申请</TableCell></TableRow>
            )}
            {(requests.data?.data ?? []).map((r) => (
              <TableRow key={r.id}>
                <TableCell className="font-medium">{r.tenant_name}</TableCell>
                <TableCell className="text-right font-mono">{r.requested_amount} CNY</TableCell>
                <TableCell className="text-xs max-w-[240px] truncate">{r.reason || "—"}</TableCell>
                <TableCell><Badge variant={r.status === "approved" ? "success" : r.status === "pending" ? "secondary" : "destructive"}>{r.status}</Badge></TableCell>
                <TableCell className="text-right">
                  {r.status === "pending" && (
                    <div className="flex gap-1 justify-end">
                      <Button size="sm" onClick={() => act(() => approveMut.mutateAsync({ id: r.id }))}>
                        <Check size={14} className="mr-1" />通过
                      </Button>
                      <Button size="sm" variant="outline" className="border-destructive/30 text-destructive hover:bg-destructive/10" onClick={() => act(() => rejectMut.mutateAsync({ id: r.id }))}>
                        <X size={14} className="mr-1" />拒绝
                      </Button>
                    </div>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
