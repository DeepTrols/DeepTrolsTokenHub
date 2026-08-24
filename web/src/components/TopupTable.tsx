import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Transaction } from "../lib/api";
import { formatAmount } from "../lib/format";

export function TopupTable({ topups }: { topups: Transaction[] }) {
  if (topups.length === 0) {
    return <p className="py-8 text-center text-[#5C6472]/80 text-sm">暂无充值记录</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>订单编号</TableHead>
          <TableHead>状态</TableHead>
          <TableHead className="text-right">金额</TableHead>
          <TableHead>支付方式</TableHead>
          <TableHead className="text-right">创建时间</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {topups.map((tx) => (
          <TableRow key={tx.id}>
            <TableCell className="font-mono text-xs">{tx.order_no || "—"}</TableCell>
            <TableCell>
              <span className={tx.status === "success" ? "text-[#0C7A55]" : "text-[#C4372C]"}>
                {tx.status === "success" ? "成功" : tx.status || "—"}
              </span>
            </TableCell>
            <TableCell className="text-right font-mono text-xs text-[#0C7A55]">+{formatAmount(tx.amount)} ￥</TableCell>
            <TableCell className="text-xs">
              {tx.payment_method === "alipay" ? "支付宝" : tx.payment_method === "wechat" ? "微信" : tx.payment_method || "—"}
            </TableCell>
            <TableCell className="text-right text-xs text-[#5C6472]">
              {new Date(tx.created_at).toLocaleString("zh-CN")}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
