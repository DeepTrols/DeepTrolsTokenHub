import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { useState } from "react";
import { ModelData } from "../lib/api";
import { useConsoleQuery } from "../lib/hooks/use-api";
import { Cpu, Search } from "lucide-react";

const cl: Record<string, string> = { chat: "对话", embedding: "向量", image: "图片", audio: "音频", video: "视频" };
const pl: Record<string, string> = {
  deepseek: "DeepSeek",
  openai: "OpenAI",
  anthropic: "Anthropic",
  google: "Google Gemini",
  qwen: "Qwen",
  zhipu: "智谱AI",
  moonshot: "Moonshot",
  baidu: "百度文心",
  xfyun: "讯飞星火",
  bytedance: "字节豆包",
  tencent: "腾讯混元",
  lingyi: "零一万物",
  openrouter: "OpenRouter",
  siliconflow: "SiliconFlow",
};
function pLabel(p: string): string {
  return pl[p] || p;
}

export default function ModelMarket() {
  const { data: md, isLoading, isError, error, refetch } = useConsoleQuery<{ data: ModelData[] }>("/models");
  const models = md?.data ?? [];
  const [s, setS] = useState("");
  const filtered = models.filter((m) => {
    if (!s) return true;
    const q = s.toLowerCase();
    return m.code.toLowerCase().includes(q) || m.display_name.toLowerCase().includes(q) || m.provider.toLowerCase().includes(q);
  });

  if (isLoading)
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">模型广场</h2>
        </div>
        <Card>
          <CardContent className="p-12 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-[#4F6BED] border-t-transparent rounded-full mx-auto mb-3" />
            <p className="text-muted-foreground">加载模型列表...</p>
          </CardContent>
        </Card>
      </div>
    );
  if (isError)
    return (
      <div>
        <div className="mb-6">
          <h2 className="font-display text-[25px] font-bold tracking-tight">模型广场</h2>
        </div>
        <Card className="border-destructive/20">
          <CardContent className="p-6 text-center">
            <p className="text-destructive mb-3">{error instanceof Error ? error.message : "加载失败"}</p>
            <Button variant="destructive" size="sm" onClick={() => refetch()}>
              重试
            </Button>
          </CardContent>
        </Card>
      </div>
    );

  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">模型广场</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">浏览可用 AI 模型</p>
      </div>
      <div className="mb-4 relative">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="搜索..." value={s} onChange={(e) => setS(e.target.value)} className="pl-10 h-10 text-sm" />
      </div>
      {filtered.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center text-muted-foreground">暂无匹配模型</CardContent>
        </Card>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {filtered.map((m) => (
          <div key={m.code} className="glass-soft rounded-xl p-4 hover:bg-white/85 transition-all">
            <div className="flex items-start gap-3 mb-3">
              <div className="p-2 bg-[#4F6BED]/10 rounded-xl shrink-0">
                <Cpu size={20} className="text-[#4F6BED]" />
              </div>
              <div className="flex-1">
                <h4 className="font-semibold text-sm truncate">{m.display_name}</h4>
                <p className="text-xs text-muted-foreground mt-0.5 flex flex-wrap gap-1">
                  <Badge variant="secondary" className="text-xs">
                    {pLabel(m.provider)}
                  </Badge>
                  <Badge variant="secondary" className="text-xs">
                    {cl[m.category] || m.category}
                  </Badge>
                  {m.context_window > 0 && (
                    <Badge variant="secondary" className="text-xs">
                      {m.context_window.toLocaleString()} ctx
                    </Badge>
                  )}
                </p>
              </div>
            </div>
            {m.pricing && Object.keys(m.pricing).length > 0 && (
              <div className="border-t border-black/10 pt-2.5">
                <p className="text-xs text-muted-foreground mb-1.5">定价</p>
                <div className="grid grid-cols-2 gap-1 text-xs">
                  {Object.entries(m.pricing).map(([dim, price]) => {
                    const dimL: Record<string, string> = {
                      input: "输入",
                      output: "输出",
                      cache_read: "缓存读",
                      cache_write: "缓存写",
                      reasoning: "推理",
                      image: "图片",
                      audio: "音频",
                      tts: "语音合成",
                      video: "视频",
                    };
                    return (
                      <div key={dim} className="flex justify-between py-1 px-2 glass-soft rounded-lg">
                        <span className="text-muted-foreground">{dimL[dim] || dim}</span>
                        <span className="font-mono font-medium">{price === "0" || price === "0.00" ? "免费" : price + " CNY"}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
