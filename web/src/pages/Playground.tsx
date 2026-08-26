import { Button } from "@/components/ui/button";
import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Send, Loader2, RotateCcw, AlertCircle } from "lucide-react";
import { APIKeyData } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { getKeySecret } from "../lib/keyMemory";

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
      (body as { error?: { message?: string } }).error?.message || `请求失败 (${res.status})`;
    throw new Error(message);
  }
  const data = (await res.json()) as { data?: GatewayModel[] };
  return data.data ?? [];
}

export default function Playground() {
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
  const [error, setError] = useState("");

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
        `获取模型列表失败: ${modelsErrorObj instanceof Error ? modelsErrorObj.message : String(modelsErrorObj)}`,
      );
    }
  }, [modelsError, modelsErrorObj]);

  const handleSend = async () => {
    const key = apiKeyText.trim();
    if (!key) { setError("请先输入 API Key"); return; }
    if (!selectedModel) { setError("请选择模型"); return; }
    setLoading(true); setResponse(""); setUsageInfo(""); setError("");
    try {
      const res = await fetch("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": "Bearer " + key, "X-Request-ID": "playground-" + Date.now() },
        body: JSON.stringify({ model: selectedModel, messages: [{ role: "user", content: prompt }] }),
      });
      const data = await res.json();
      if (data.error) { setError(data.error.message || "请求失败"); }
      else {
        setResponse(data.choices?.[0]?.message?.content || JSON.stringify(data, null, 2));
        const u = data.usage;
        if (u && (u.total_tokens != null)) {
          setUsageInfo(`${u.total_tokens} tokens`);
        }
      }
    } catch (e) { setError("网络错误: " + String(e)); }
    setLoading(false);
  };

  const handleReset = () => { setPrompt(""); setResponse(""); setUsageInfo(""); setError(""); };

  const noKeysAvailable = !keysLoading && apiKeys.length === 0;

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">在线体验</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">使用真实 API Key 在线测试模型调用效果</p>
      </div>
      <div className="glass rounded-2xl p-6">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div>
            {/* API Key Selection */}
            <div className="mb-4">
              <label htmlFor="api-key-select" className="block text-[12px] font-semibold text-[#5C6472] mb-2">选择 API 密钥</label>
              <select id="api-key-select" value={selectedKeyId} onChange={(e) => setSelectedKeyId(e.target.value)}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm mb-2 focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20">
                <option value="">-- 选择密钥以加载模型 --</option>
                {apiKeys.filter(k => k.status === "active").map(k => (
                  <option key={k.id} value={k.id}>{k.name}</option>
                ))}
              </select>
              {modelsLoading ? (
                <div className="flex items-center gap-2 text-sm text-[#5C6472]/70 py-2">
                  <Loader2 size={14} className="animate-spin" /> 加载模型列表中...
                </div>
              ) : (
                <select value={selectedModel} onChange={(e) => setSelectedModel(e.target.value)}
                  className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20"
                  disabled={availableModels.length === 0}>
                  {availableModels.length === 0 ? (
                    <option>{selectedKeyId ? "暂无模型" : "请先选择密钥"}</option>
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
              <div className="mb-4 p-4 glass-soft border-[#4F6BED]/30 rounded-xl">
                <p className="text-sm text-[#4F6BED] font-medium">请先创建 API 密钥</p>
                <p className="text-xs text-[#5C6472] mt-1">创建密钥后即可在控制台在线体验模型调用</p>
              </div>
            )}

            {/* Prompt */}
            <div className="mb-4">
              <label className="block text-[12px] font-semibold text-[#5C6472] mb-2">输入提示词</label>
              <textarea value={prompt} onChange={(e) => setPrompt(e.target.value)}
                placeholder="在此输入您的问题或提示词..." rows={6}
                className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm resize-none focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20" />
            </div>

            <div className="flex gap-3">
              <Button onClick={handleSend} disabled={loading || !apiKeyText.trim()}>
                {loading ? <Loader2 size={16} className="animate-spin"/> : <Send size={16}/>}
                发送请求
              </Button>
              <Button variant="outline" onClick={handleReset}>
                <RotateCcw size={14} /> 重置
              </Button>
            </div>
          </div>

          {/* Response */}
          <div>
            <label className="block text-[12px] font-semibold text-[#5C6472] mb-2">响应结果</label>
            <div className="p-4 glass-soft rounded-xl min-h-[200px] max-h-[400px] overflow-auto">
              {loading ? (
                <div className="flex items-center gap-2 text-[#5C6472]/70"><Loader2 size={16} className="animate-spin"/> 请求中...</div>
              ) : error ? (
                <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/25 rounded-xl text-sm text-[#C4372C] flex items-start gap-2">
                  <AlertCircle size={16} className="mt-0.5"/>{error}
                </div>
              ) : response ? (
                <pre className="text-sm whitespace-pre-wrap font-sans text-[#161A23]">{response}</pre>
              ) : (
                <p className="text-sm text-[#5C6472]/70">在左侧输入提示词并点击发送</p>
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
