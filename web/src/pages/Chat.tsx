import { useState } from "react";
import { ArrowLeft, ExternalLink, KeyRound, MessageSquare } from "lucide-react";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { APIKeyData } from "../lib/api";
import { useSiteInfo } from "../lib/site";
import {
  chatLinkRequiresApiKey,
  parseChatConfig,
  resolveChatUrl,
  type ChatPreset,
} from "../lib/chat-links";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import "../i18n";
import { useTranslation } from "react-i18next";

const TYPE_LABEL: Record<ChatPreset["type"], string> = {
  web: "chat.typeWeb",
  "custom-protocol": "chat.typeCustom",
  fluent: "Fluent",
};

export default function Chat() {
  const { t } = useTranslation();
  const { site } = useSiteInfo();
  const presetsQuery = useConsoleQuery<Array<Record<string, string>>>("/chat/presets");
  const [selected, setSelected] = useState<ChatPreset | null>(null);

  const presets = parseChatConfig(presetsQuery.data as never);

  // For templates containing {key}, resolve the first enabled API key.
  const keysQuery = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const activeKeyId = (keysQuery.data?.data ?? []).find((k) => k.status === "active")?.id;
  const secretQuery = useConsoleQuery<{ plaintext: string }>(
    activeKeyId ? `/api-keys/${activeKeyId}/secret` : "",
    { enabled: !!activeKeyId },
  );

  const requiresKey = selected ? chatLinkRequiresApiKey(selected.url) : false;
  const keyReady = !requiresKey || Boolean(secretQuery.data?.plaintext);
  const iframeSrc =
    selected && selected.type === "web" && keyReady
      ? resolveChatUrl({
          template: selected.url,
          apiKey: secretQuery.data?.plaintext,
          serverAddress: site.server_address,
        })
      : "";

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("chat.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("chat.subtitle")}</p>
      </div>

      {selected && selected.type === "web" ? (
        <div className="glass rounded-[22px] p-4">
          <div className="flex items-center justify-between mb-3">
            <Button variant="ghost" size="sm" onClick={() => setSelected(null)}>
              <ArrowLeft size={15} className="mr-1" /> {t("chat.back")}
            </Button>
            <span className="text-sm font-semibold">{selected.name}</span>
            {requiresKey && !secretQuery.data?.plaintext ? (
              <span className="flex items-center gap-1 text-xs text-[#D3A94E]">
                <KeyRound size={13} /> {t("chat.waitingKey")}
              </span>
            ) : (
              <a
                href={iframeSrc}
                target="_blank"
                rel="noreferrer"
                className="text-xs text-[#4F6BED] hover:underline flex items-center gap-1"
              >
                <ExternalLink size={13} /> {t("chat.openNewWindow")}
              </a>
            )}
          </div>
          <iframe
            title={selected.name}
            src={iframeSrc}
            className="w-full h-[calc(100vh-240px)] min-h-[480px] rounded-xl bg-white"
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
          />
        </div>
      ) : presetsQuery.isLoading ? (
        <LoadingState message={t("chat.loading")} />
      ) : presetsQuery.isError ? (
        <ErrorState error={presetsQuery.error} onRetry={() => presetsQuery.refetch()} title={t("chat.loadFailed")} />
      ) : presets.length === 0 ? (
        <EmptyState
          icon={MessageSquare}
          title={t("chat.empty")}
          description={t("chat.emptyDesc")}
        />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {presets.map((p) => {
            const needKey = chatLinkRequiresApiKey(p.url);
            const disabled = p.type === "web" && needKey && !secretQuery.data?.plaintext;
            return (
              <button
                key={p.id}
                onClick={() => {
                  if (p.type === "web") setSelected(p);
                  else if (p.type === "custom-protocol" || p.type === "fluent") {
                    window.open(resolveChatUrl({ template: p.url, serverAddress: site.server_address }), "_blank");
                  }
                }}
                disabled={disabled}
                className="glass rounded-[22px] p-5 text-left transition-all hover:shadow-[0_14px_34px_rgba(79,107,237,0.12)] disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <div className="flex items-start justify-between gap-3">
                  <span className="grid w-10 h-10 place-items-center rounded-xl bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white shadow-[0_6px_16px_rgba(79,107,237,0.35)]">
                    <MessageSquare size={17} />
                  </span>
                  <span className="text-[11px] font-semibold text-[#8C93A1] bg-white/60 rounded-full px-2.5 py-1">
                    {t(TYPE_LABEL[p.type])}
                  </span>
                </div>
                <h3 className="font-display font-semibold mt-3">{p.name}</h3>
                <p className="text-[12px] text-[#5C6472] mt-1 font-mono truncate">{p.url}</p>
                {needKey && <p className="mt-2 text-[11px] text-[#D3A94E] flex items-center gap-1"><KeyRound size={11} /> {t("chat.injectKey")}</p>}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
