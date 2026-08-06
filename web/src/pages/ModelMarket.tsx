import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { useEffect, useState } from "react";import { ModelData } from "../lib/api";import { useConsoleQuery } from "../lib/hooks/use-api";import { Cpu, Search, ChevronDown, ChevronRight, Store, Tags, Factory } from "lucide-react";
type GK="provider"|"plan"|"factory";interface Group{key:string;label:string;icon:React.ReactNode;items:ModelData[]}
const cl:Record<string,string>={chat:"对话",embedding:"向量",image:"图片",audio:"音频",video:"视频"};
const pl:Record<string,string>={deepseek:"DeepSeek",openai:"OpenAI",anthropic:"Anthropic",google:"Google Gemini",qwen:"Qwen",zhipu:"智谱AI",moonshot:"Moonshot",baidu:"百度文心",xfyun:"讯飞星火",bytedance:"字节豆包",tencent:"腾讯混元",lingyi:"零一万物",openrouter:"OpenRouter",siliconflow:"SiliconFlow"};
const po=Object.keys(pl);const fp=new Set(["openrouter","siliconflow"]);
function pLabel(p:string):string{return pl[p]||p}
function isFree(pr:Record<string,string>):boolean{const v=Object.values(pr);if(v.length===0)return true;return v.every(x=>x==="0"||x==="0.00")}
export default function ModelMarket(){
  const{data:md,isLoading,isError,error,refetch}=useConsoleQuery<{data:ModelData[]}>("/models");
  const models=md?.data??[];const[s,setS]=useState("");const[ex,setEx]=useState<Record<string,boolean>>({});const[ag,setAg]=useState<GK>("provider");
  const filtered=models.filter(m=>{if(!s)return true;const q=s.toLowerCase();return m.code.toLowerCase().includes(q)||m.display_name.toLowerCase().includes(q)||m.provider.toLowerCase().includes(q)});
  function bg(k:GK):Group[]{const map=new Map<string,ModelData[]>();for(const m of filtered){let b:string;switch(k){case"provider":b=m.provider||"unknown";break;case"plan":b=isFree(m.pricing||{})?"free":"paid";break;case"factory":b=fp.has((m.provider||"").toLowerCase())?m.provider:"direct"}if(!map.has(b))map.set(b,[]);map.get(b)!.push(m)}
  const gs:Group[]=[];for(const[key,items]of map){let l:string;let ic:React.ReactNode;switch(ag){case"provider":l=pLabel(key);ic=<Store size={16}/>;break;case"plan":l=key==="free"?"免费模型":"付费模型";ic=<Tags size={16}/>;break;case"factory":l=key==="direct"?"官方直供":pLabel(key);ic=<Factory size={16}/>}gs.push({key,label:l+" ("+items.length+")",icon:ic,items})}
  if(ag==="provider")gs.sort((a,b)=>{const ai=po.indexOf(a.key),bi=po.indexOf(b.key);if(ai===-1&&bi===-1)return a.key.localeCompare(b.key);if(ai===-1)return 1;if(bi===-1)return-1;return ai-bi});else if(ag==="plan")gs.sort((a,b)=>a.key==="free"?-1:1);return gs}
  const groups=bg(ag);const toggle=(k:string)=>setEx(p=>({...p,[k]:!p[k]}));
  useEffect(()=>{if(models.length>0){const all:Record<string,boolean>={};for(const g of groups)all[g.key]=true;setEx(p=>({...all,...p}))}},[models.length,s,ag]);
  const tabs:{key:GK;label:string}[]=[{key:"provider",label:"模型商家"},{key:"plan",label:"Token Plan"},{key:"factory",label:"模型工厂"}];
  if(isLoading)return <div><div className="mb-6"><h2 className="text-2xl font-bold">模型广场</h2></div><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载模型列表...</p></CardContent></Card></div>;
  if(isError)return <div><div className="mb-6"><h2 className="text-2xl font-bold">模型广场</h2></div><Card className="border-destructive/20"><CardContent className="p-6 text-center"><p className="text-destructive mb-3">{error instanceof Error?error.message:"加载失败"}</p><Button variant="destructive" size="sm" onClick={()=>refetch()}>重试</Button></CardContent></Card></div>;
  return <div>
    <div className="mb-6"><h2 className="text-2xl font-bold">模型广场</h2><p className="text-sm text-muted-foreground mt-1">浏览可用 AI 模型</p></div>
    <div className="mb-4 relative"><Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"/><Input placeholder="搜索..." value={s} onChange={e=>setS(e.target.value)} className="pl-10 h-10 text-sm"/></div>
    <div className="flex gap-2 mb-6">{tabs.map(t=><Button key={t.key} variant={ag===t.key?"default":"outline"} size="sm" onClick={()=>setAg(t.key)}>{t.label}</Button>)}</div>
    {groups.length===0&&<Card><CardContent className="py-12 text-center text-muted-foreground">暂无匹配模型</CardContent></Card>}
    <div className="space-y-4">{groups.map(g=><Card key={g.key}><div className="flex items-center gap-3 px-5 py-3.5 bg-muted cursor-pointer hover:bg-muted/80" onClick={()=>toggle(g.key)}><span className="text-muted-foreground">{ex[g.key]?<ChevronDown size={18}/>:<ChevronRight size={18}/>}</span><span>{g.icon}</span><h3 className="font-semibold text-sm">{g.label}</h3></div>
    {ex[g.key]&&<CardContent className="grid grid-cols-1 md:grid-cols-2 gap-4 p-4">{g.items.map(m=><div key={m.code} className="border rounded-lg p-4 hover:shadow-sm"><div className="flex items-start gap-3 mb-3"><div className="p-2 bg-primary/10 rounded-lg shrink-0"><Cpu size={20} className="text-primary"/></div><div className="flex-1"><h4 className="font-semibold text-sm truncate">{m.display_name}</h4><p className="text-xs text-muted-foreground mt-0.5 flex flex-wrap gap-1"><Badge variant="secondary" className="text-xs">{pLabel(m.provider)}</Badge><Badge variant="secondary" className="text-xs">{cl[m.category]||m.category}</Badge>{m.context_window>0&&<Badge variant="secondary" className="text-xs">{m.context_window.toLocaleString()} ctx</Badge>}</p></div></div>
    {m.pricing&&Object.keys(m.pricing).length>0&&<div className="border-t pt-2.5"><p className="text-xs text-muted-foreground mb-1.5">定价</p><div className="grid grid-cols-2 gap-1 text-xs">{Object.entries(m.pricing).map(([dim,price])=>{const dimL:Record<string,string>={input:"输入",output:"输出",cache_read:"缓存读",cache_write:"缓存写",reasoning:"推理",image:"图片",audio:"音频",tts:"语音合成",video:"视频"};return <div key={dim} className="flex justify-between py-1 px-2 bg-muted rounded"><span className="text-muted-foreground">{dimL[dim]||dim}</span><span className="font-mono font-medium">{price==="0"||price==="0.00"?"免费":price+" CNY"}</span></div>})}</div></div>}</div>)}</CardContent>}</Card>)}</div>
  </div>;
}