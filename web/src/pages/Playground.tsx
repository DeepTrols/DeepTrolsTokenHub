import { Button } from "@/components/ui/button";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Send, Loader2, RotateCcw, AlertCircle, Square } from "lucide-react";
import { APIKeyData } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { getKeySecret } from "../lib/keyMemory";
import { streamChatCompletion } from "../lib/streaming";
import i18n from "../i18n";
import { useTranslation } from "react-i18next";

interface GatewayModel {
  id: string;
  object?: string;
  owned_by?: string;
}

/** Fetches the model list from the OpenAI-compatible gateway using the selected API key as Bearer. */
async function fetchGatewayModels(apiKey: string): Promise<GatewayModel[]> {
  const res = await fetch("/v1/models", {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${apiKey}`,
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const message =
      (body as { error?: { message?: string } }).error?.message || i18n.t("playground.requestFailed", { status: res.status });
    throw new Error(message);
  }
  const data = (await res.json()) as { data?: GatewayModel[] };
  return data.data ?? [];
}

export default function Playground() {
  const { t } = useTranslation();
  const { data: keysData, isLoading: keysLoading } = useConsoleQuery<{ data: APIKeyData[] }>("/api-keys");
  const apiKeys = keysData?.data ?? [];
  const [selectedKeyId, setSelectedKeyId] = useState("");
  const [apiKeyText, setApiKeyText] = useState("");

  const { data: secretData } = useConsoleQuery<{ plaintext: string }>(
    selectedKeyId ? `/api-keys/${selectedKeyId}/secret` : "",
    { enabled: !!selectedKeyId },
  );
  const {
    data: gatewayModels,
    isLoading: modelsLoading,
    isError: modelsError,
    error: modelsErrorObj,
  } = useQuery<GatewayModel[], Error>({
    queryKey: ["gateway", "models", apiKeyText],
    queryFn: () => fetchGatewayModels(apiKeyText),
    enabled: !!selectedKeyId && !!apiKeyText,
    // 模型列表必须实时反映上游状态，禁止复用旧的缓存结果。
    staleTime: 0,
  });
  const availableModels = useMemo(
    () => (gatewayModels ?? []).map((m) => ({ code: m.id, display_name: m.id })),
    [gatewayModels],
  );
  const [selectedModel, setSelectedModel] = useState("");
  const [modelsLoadError, setModelsLoadError] = useState("");

  const [prompt, setPrompt] = useState("");
  const [response, setResponse] = useState("");
  const [usageInfo, setUsageInfo] = useState("");
  const [loading, setLoading] = useState(false);
  const [streaming, setStreaming] = useState(false);
  const [reasoning, setReasoning] = useState("");
  const [error, setError] = useState("");
  const abortRef = useRef<AbortController | null>(null);

  // Sync API key secret into state when the secret query resolves.
  useEffect(() => {
    if (secretData?.plaintext) setApiKeyText(secretData.plaintext);
  }, [secretData]);

  // When a key is selected, reuse the in-memory plaintext revealed earlier in
  // this session (never localStorage), and let the secret query override.
  // The key ID (UUID) is NOT a valid API key — sending it as Bearer makes the
  // gateway reject the request with "Invalid API key" — so we never fall back
  // to it; the models request stays disabled until a real plaintext lands.
  useEffect(() => {
    if (!selectedKeyId) return;
    setApiKeyText("");
    setModelsLoadError("");
    setResponse("");
    setUsageInfo("");
    setError("");
    const cached = getKeySecret(selectedKeyId);
    if (cached) setApiKeyText(cached);
  }, [selectedKeyId]);

  // Auto-select the first model once models load.
  useEffect(() => {
    if (availableModels.length > 0) setSelectedModel(availableModels[0].code);
  }, [availableModels]);

  // Surface a model-list load error to the UI.
  useEffect(() => {
    if (modelsError) {
      setModelsLoadError(
        `${t("playground.fetchModelsFailed")}: ${modelsErrorObj instanceof Error ? modelsErrorObj.message : String(modelsErrorObj)}`,
      );
    }
  }, [modelsError, modelsErrorObj, t]);

  const handleSend = async () => {
    const key = apiKeyText.trim();
    if (!key) { setError(t("playground.needKey")); return; }
    if (!selectedModel) { setError(t("playground.needModel")); return; }
    setLoading(true); setStreaming(true); setResponse(""); setReasoning(""); setUsageInfo(""); setError("");
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const result = await streamChatCompletion({
        url: "/v1/chat/completions",
        apiKey: key,
        model: selectedModel,
        messages: [{ role: "user", content: prompt }],
        requestId: "playground-" + Date.now(),
        signal: controller.signal,
        callbacks: {
          onDelta: (text) => setResponse((prev) => prev + text),
          onReasoning: (text) => setReasoning((prev) => prev + text),
          onUsage: (u) => setUsageInfo(u.total_tokens != null ? `${u.total_tokens} tokens` : ""),
        },
      });
      if (result.usage && result.usage.total_tokens != null) {
        setUsageInfo(`${result.usage.total_tokens} tokens`);
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError") {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setLoading(false);
      setStreaming(false);
      abortRef.current = null;
    }
  };

  const handleStop = () => {
    abortRef.current?.abort();
  };

  const handleReset = () => {
    handleStop();
    setPrompt(""); setResponse(""); setReasoning(""); setUsageInfo(""); setError("");
  };

  const noKeysAvailable = !keysLoading && apiKeys.length === 0;

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("playground.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("playground.subtitle")}</p>
      </div>
      <div className="glass rounded-2xl p-6">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div>
            {/* API Key Selection */}
            <div className="mb-4">
              <label htmlFor="api-key-select" className="block text-[12px] font-semibold text-[#5C6472] mb-2">{t("playground.selectKey")}</label>
              <select id="api-key-select" value={selectedKeyId} onChange={(e) => setSelectedKeyId(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm mb-2 focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20">
                <option value="">{t("playground.selectKeyPlaceholder")}</option>
                {apiKeys.filter(k => k.status === "active").map(k => (
                  <option key={k.id} value={k.id}>{k.name}</option>
                ))}
              </select>
              {modelsLoading ? (
                <div className="flex items-center gap-2 text-sm text-[#5C6472]/70 py-2">
                  <Loader2 size={14} className="animate-spin" /> {t("playground.loadingModels")}
                </div>
              ) : (
                <select value={selectedModel} onChange={(e) => setSelectedModel(e.target.value)}
                  className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20"
                  disabled={availableModels.length === 0}>
                  {availableModels.length === 0 ? (
                    <option>{selectedKeyId ? t("playground.noModels") : t("playground.selectKeyFirst")}</option>
                  ) : (
                    availableModels.map((m) => (
                      <option key={m.code} value={m.code}>{m.display_name || m.code}</option>
                    ))
                  )}
                </select>
              )}
              {modelsLoadError && (
                <div className="mt-2 flex items-start gap-2 text-sm text-[#C4372C]">
                  <AlertCircle size={14} className="mt-0.5 shrink-0" />
                  <span>{modelsLoadError}</span>
                </div>
              )}
            </div>

            {noKeysAvailable && (
              <div className="mb-4 p-4 glass-soft border-[#F78B28]/30 rounded-xl">
                <p className="text-sm text-primary-700 font-medium">{t("playground.noKeysTitle")}</p>
                <p className="text-xs text-[#5C6472] mt-1">{t("playground.noKeysDesc")}</p>
              </div>
            )}

            {/* Prompt */}
            <div className="mb-4">
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-2">{t("playground.prompt")}</label>
              <textarea value={prompt} onChange={(e) => setPrompt(e.target.value)}
                placeholder={t("playground.promptPlaceholder")} rows={6}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm resize-none focus:outline-none focus:border-[#F78B28] focus:ring-2 focus:ring-[#F78B28]/20" />
            </div>

            <div className="flex gap-3">
              <Button onClick={handleSend} disabled={loading || !apiKeyText.trim()}>
                {loading ? <Loader2 size={16} className="animate-spin"/> : <Send size={16}/>}
                {t("playground.send")}
              </Button>
              {streaming && (
                <Button variant="outline" onClick={handleStop}>
                  <Square size={14} /> {t("playground.stop")}
                </Button>
              )}
              <Button variant="outline" onClick={handleReset}>
                <RotateCcw size={14} /> {t("playground.reset")}
              </Button>
            </div>
          </div>

          {/* Response */}
          <div>
            <label className="block text-[12px] font-semibold text-[#5C6472] mb-2">{t("playground.response")}</label>
            <div className="p-4 glass-soft rounded-xl min-h-[200px] max-h-[400px] overflow-auto">
              {loading ? (
                <div className="flex items-center gap-2 text-[#5C6472]/70">
                  <Loader2 size={16} className="animate-spin"/> {streaming ? t("playground.streaming") : t("playground.requesting")}
                </div>
              ) : error ? (
                <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/25 rounded-xl text-sm text-[#C4372C] flex items-start gap-2">
                  <AlertCircle size={16} className="mt-0.5"/>{error}
                </div>
              ) : response ? (
                <>
                  {reasoning && (
                    <div className="mb-3 p-3 rounded-xl bg-[#E85D3F]/8 border border-[#E85D3F]/20 text-[13px] text-[#5C6472] whitespace-pre-wrap">
                      <div className="text-[11px] font-semibold uppercase tracking-wide text-[#B94723] mb-1">{t("playground.thinking")}</div>
                      {reasoning}
                    </div>
                  )}
                  <pre className="text-sm whitespace-pre-wrap font-sans text-[#161A23]">{response}</pre>
                </>
              ) : (
                <p className="text-sm text-[#5C6472]/70">{t("playground.emptyHint")}</p>
              )}
              {usageInfo && (
                <p className="mt-2 text-xs text-[#5C6472] font-mono">{usageInfo}</p>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
