import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Transaction } from "../lib/api";
import { formatAmount } from "../lib/format";
import "../i18n";
import { useTranslation } from "react-i18next";

export function TopupTable({ topups }: { topups: Transaction[] }) {
  const { t } = useTranslation();
  if (topups.length === 0) {
    return <p className="py-8 text-center text-[#5C6472]/80 text-sm">{t("components.topupEmpty")}</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("components.topupOrderNo")}</TableHead>
          <TableHead>{t("components.topupStatus")}</TableHead>
          <TableHead className="text-right">{t("components.topupAmount")}</TableHead>
          <TableHead>{t("components.topupMethod")}</TableHead>
          <TableHead className="text-right">{t("components.topupCreated")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {topups.map((tx) => (
          <TableRow key={tx.id}>
            <TableCell className="font-mono text-xs">{tx.order_no || "—"}</TableCell>
            <TableCell>
              <span className={tx.status === "success" ? "text-[#0C7A55]" : "text-[#C4372C]"}>
                {tx.status === "success" ? t("components.topupSuccess") : tx.status || "—"}
              </span>
            </TableCell>
            <TableCell className="text-right font-mono text-xs text-[#0C7A55]">+{formatAmount(tx.amount)} ￥</TableCell>
            <TableCell className="text-xs">
              {tx.payment_method === "alipay" ? t("components.methodAlipay") : tx.payment_method === "wechat" ? t("components.methodWechat") : tx.payment_method || "—"}
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
