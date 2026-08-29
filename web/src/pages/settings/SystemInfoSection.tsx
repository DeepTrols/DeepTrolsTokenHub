import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { useAdminQuery } from "@/lib/hooks/use-api";
import "../../i18n";
import { useTranslation } from "react-i18next";

interface SystemInfo {
  version: string;
  go_version: string;
  uptime_secs: number;
  counts: Record<string, number>;
}

function fmtUptime(secs: number) {
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = Math.floor(secs % 60);
  return `${h}h ${m}m ${s}s`;
}

const COUNT_LABELS: Record<string, string> = {
  users: "settings.countUsers",
  models: "settings.countModels",
  channels: "settings.countChannels",
  instances: "settings.countInstances",
  wallets: "settings.countWallets",
  usage: "settings.countUsage",
  orders: "settings.countOrders",
};

export default function SystemInfoSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SystemInfo>("/system/info");
  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.systemTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.systemSubtitle")}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.runtime")}</CardTitle>
          <CardDescription>{t("settings.runtimeDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <div>
            <p className="text-xs text-[#5C6472]">{t("settings.version")}</p>
            <p className="font-mono text-lg">{data?.version ?? "-"}</p>
          </div>
          <div>
            <p className="text-xs text-[#5C6472]">Go</p>
            <p className="font-mono text-lg">{data?.go_version ?? "-"}</p>
          </div>
          <div>
            <p className="text-xs text-[#5C6472]">{t("settings.uptime")}</p>
            <p className="text-lg">{data ? fmtUptime(data.uptime_secs) : "-"}</p>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.dataCounts")}</CardTitle>
          <CardDescription>{t("settings.dataCountsDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {Object.entries(COUNT_LABELS).map(([key, label]) => (
            <div key={key} className="glass-soft rounded-xl p-3">
              <p className="text-xs text-[#5C6472]">{t(label)}</p>
              <p className="text-xl font-semibold">{data?.counts?.[key] ?? "-"}</p>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
