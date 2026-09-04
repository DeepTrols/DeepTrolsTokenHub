import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import { ChannelModelItem } from "@/lib/api";
import "../i18n";
import { useTranslation } from "react-i18next";

export function ChannelModelList({ channelId }: { channelId: string }) {
  const { t } = useTranslation();
  const list = useAdminQuery<{ models: ChannelModelItem[] }>(`/channels/${channelId}/models`);
  const toggle = useAdminMutation<{ ok: boolean }, { modelId: string; enabled: boolean }>(
    "put",
    (v) => `/channels/${channelId}/models/${v.modelId}`,
    `/channels/${channelId}/models`,
    { onSuccess: () => list.refetch() },
  );

  const models = list.data?.models ?? [];

  if (list.isLoading) {
    return <p className="text-sm text-[#5C6472]">{t("components.boundLoading")}</p>;
  }
  if (models.length === 0) {
    return <p className="text-sm text-[#5C6472]">{t("components.boundEmpty")}</p>;
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-xs text-[#5C6472]">
            <th className="py-2 pr-3">{t("components.boundUpstream")}</th>
            <th className="py-2 pr-3">{t("components.boundCode")}</th>
            <th className="py-2 pr-3">{t("components.boundStatus")}</th>
            <th className="py-2">{t("components.boundEnabled")}</th>
          </tr>
        </thead>
        <tbody>
          {models.map((m) => (
            <tr key={m.model_id} className="border-b border-black/5">
              <td className="py-2 pr-3 font-mono text-xs">{m.upstream}</td>
              <td className="py-2 pr-3 font-mono text-xs">{m.code}</td>
              <td className="py-2 pr-3 text-xs text-[#5C6472]">{m.status}</td>
              <td className="py-2">
                <input
                  type="checkbox"
                  checked={m.enabled}
                  disabled={toggle.isPending}
                  onChange={(e) => toggle.mutate({ modelId: m.model_id, enabled: e.target.checked })}
                  className="accent-[#F78B28]"
                  aria-label={t("components.boundEnableAria", { code: m.code })}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
