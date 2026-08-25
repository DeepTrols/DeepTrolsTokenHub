import { useState } from "react";
import { useConsoleMutation, useConsoleQuery } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Loader2, PiggyBank } from "lucide-react";

interface TeamBudget {
  id: string;
  period: string;
  limit_amount: string;
  spent_amount: string;
  status: string;
}

interface TeamBudgetRequest {
  id: string;
  requested_amount: string;
  reason: string;
  status: string;
  created_at: string;
}

export default function BudgetTeam() {
  const { data, isLoading, refetch } = useConsoleQuery<{ budgets: TeamBudget[]; requests: TeamBudgetRequest[] }>("/team/budget");
  const createMut = useConsoleMutation<{ id: string }, { amount: string; reason: string }>("post", "/team/budget/requests");
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");

  const budgets = data?.budgets ?? [];
  const requests = data?.requests ?? [];

  const submit = async () => {
    setError("");
    try {
      await createMut.mutateAsync({ amount, reason });
      setAmount(""); setReason("");
      refetch();
    } catch (e) {
      setError(e instanceof Error ? e.message : "提交失败");
    }
  };

  if (isLoading) {
    return <Card><CardContent className="p-12 text-center text-muted-foreground">加载预算...</CardContent></Card>;
  }
  return (
    <div>
      <div className="mb-5">
        <h2 className="font-display text-[25px] font-bold tracking-tight">团队预算</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">查看月度预算与用量，额度不足可向平台申请加额</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        {budgets.length === 0 && (
          <Card className="md:col-span-3"><CardContent className="p-10 text-center text-muted-foreground flex flex-col items-center gap-2">
            <PiggyBank size={28} className="opacity-30" />暂无预算，可在下方申请
          </CardContent></Card>
        )}
        {budgets.map((b) => (
          <Card key={b.id}>
            <CardContent className="p-5">
              <p className="text-sm text-muted-foreground">{b.period === "monthly" ? "月度预算" : b.period}</p>
              <p className="font-mono text-[22px] font-semibold mt-1">{b.limit_amount} CNY</p>
              <p className="text-xs text-muted-foreground mt-1">已用 {b.spent_amount} CNY</p>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card className="mb-6">
        <CardContent className="p-5">
          <h3 className="font-semibold mb-3">申请加额</h3>
          {error && <p className="text-destructive text-sm mb-3">{error}</p>}
          <div className="grid grid-cols-[160px_1fr_auto] gap-3 items-end">
            <div className="space-y-2">
              <Label>金额（CNY）</Label>
              <Input type="number" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="1000" />
            </div>
            <div className="space-y-2">
              <Label>原因</Label>
              <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="业务扩容" />
            </div>
            <Button onClick={submit} disabled={createMut.isPending || !amount || Number(amount) <= 0}>
              {createMut.isPending && <Loader2 size={14} className="mr-1.5 animate-spin" />}提交申请
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className="overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>申请金额</TableHead>
              <TableHead>原因</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {requests.length === 0 && <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">暂无申请记录</TableCell></TableRow>}
            {requests.map((r) => (
              <TableRow key={r.id}>
                <TableCell className="font-mono">{r.requested_amount} CNY</TableCell>
                <TableCell className="text-xs">{r.reason || "—"}</TableCell>
                <TableCell><Badge variant={r.status === "approved" ? "success" : r.status === "pending" ? "secondary" : "destructive"}>{r.status}</Badge></TableCell>
                <TableCell className="text-xs">{r.created_at?.replace("T", " ").slice(0, 16)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
