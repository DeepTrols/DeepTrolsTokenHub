import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { useState } from "react";
import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";
import { Plus, Edit, Trash2, GitBranch } from "lucide-react";

interface PolicyData {
  id: string; name: string; tenant_id: string|null; user_level: string;
  model_id: string; priority: number; candidate_channel_ids: string[];
  candidate_channel_names?: string[]; fallback_policy: string; is_active: boolean;
}
const FALLBACKS = [
  {v:"disabled",l:"禁用"},{v:"tenant_default",l:"租户默认"},
  {v:"shared_allowed",l:"允许共享"},{v:"next_policy",l:"下一条策略"},
];

export default function Policies() {
  const {
    data: policyData,
    isLoading,
    isError,
    error,
    refetch,
  } = useAdminQuery<{ data: PolicyData[] }>("/policies");
  const policies = policyData?.data ?? [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<PolicyData|null>(null);
  const [name, setName] = useState(""); const [userLevel, setUserLevel] = useState("");
  const [modelId, setModelId] = useState(""); const [priority, setPriority] = useState(0);
  const [channelIds, setChannelIds] = useState(""); const [fallback, setFallback] = useState("disabled");
  const [tenantId, setTenantId] = useState("");

  const createMutation = useAdminMutation<unknown, Record<string, unknown>>("post", "/policies");
  const updateMutation = useAdminMutation<unknown, { id: string } & Record<string, unknown>>(
    "put",
    (v) => `/policies/${v.id}`,
    "/policies",
  );
  const deleteMutation = useAdminMutation<unknown, { id: string }>(
    "delete",
    (v) => `/policies/${v.id}`,
    "/policies",
  );

  const reset = () => {setName("");setUserLevel("");setModelId("");setPriority(0);setChannelIds("");setFallback("disabled");setTenantId("");setEditing(null);setShowForm(false);};

  const handleSubmit = async () => {
    if(!name.trim()||!modelId.trim())return;
    const b:Record<string,unknown>={name:name.trim(),user_level:userLevel,model_id:modelId.trim(),priority,fallback_policy:fallback,
      candidate_channel_ids:channelIds.split(",").map(s=>s.trim()).filter(Boolean)};
    if(tenantId.trim())b.tenant_id=tenantId.trim();
    if (editing) {
      await updateMutation.mutateAsync({ id: editing.id, ...b });
    } else {
      await createMutation.mutateAsync(b);
    }
    reset();
  };
  const handleDelete = async (p:PolicyData) => {if(!confirm(`删除 "${p.name}"？`))return;await deleteMutation.mutateAsync({id:p.id});};

  return <div>
    <div className="flex items-center justify-between mb-6">
      <div><h2 className="text-2xl font-bold">路由策略</h2><p className="text-sm text-gray-500 mt-1">配置请求路由规则和 fallback 策略</p></div>
      <button onClick={()=>{reset();setShowForm(true);}} className="flex items-center gap-2 px-4 py-2 bg-gray-800 text-white rounded-lg hover:bg-gray-700 text-sm font-medium"><Plus size={16}/>创建策略</button>
    </div>

    {showForm&&<div className="mb-6 p-6 bg-white border border-gray-200 rounded-xl">
      <h3 className="font-semibold mb-4">{editing?"编辑策略":"创建策略"}</h3>
      <div className="grid grid-cols-2 gap-4">
        <div><label className="block text-sm font-medium text-gray-700 mb-1">名称*</label><input aria-label="策略名称" value={name} onChange={e=>setName(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"/></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">用户等级</label><input value={userLevel} onChange={e=>setUserLevel(e.target.value)} placeholder="vip / free" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"/></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">模型ID*</label><input aria-label="模型编码" value={modelId} onChange={e=>setModelId(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"/></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">优先级</label><input type="number" value={priority} onChange={e=>setPriority(+e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"/></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">Channel IDs</label><input value={channelIds} onChange={e=>setChannelIds(e.target.value)} placeholder="UUID1,UUID2" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"/></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">Fallback</label><select value={fallback} onChange={e=>setFallback(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm">{FALLBACKS.map(f=><option key={f.v} value={f.v}>{f.l}</option>)}</select></div>
        <div><label className="block text-sm font-medium text-gray-700 mb-1">租户ID</label><input value={tenantId} onChange={e=>setTenantId(e.target.value)} placeholder="留空=平台级" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"/></div>
      </div>
      <div className="flex gap-3 mt-6"><button onClick={handleSubmit} className="px-4 py-2 bg-gray-800 text-white rounded-lg hover:bg-gray-700 text-sm">{editing?"保存":"创建"}</button><button onClick={reset} className="px-4 py-2 border rounded-lg text-sm text-gray-600">取消</button></div>
    </div>}

    {loadError && <ErrorState error={loadError} onRetry={()=>refetch()} />}
    {isLoading && (
      <div className="p-12 text-center bg-white rounded-xl border">
        <div className="animate-spin w-8 h-8 border-2 border-primary-600 border-t-transparent rounded-full mx-auto mb-3" />
        <p className="text-gray-500">加载路由策略...</p>
      </div>
    )}
    <div className="bg-white rounded-xl border border-gray-200">
      {!isLoading && policies.length===0&&<EmptyState icon={GitBranch} title="暂无路由策略" />}
      {policies.map(p=><div key={p.id} className="p-4 border-b border-gray-100 flex items-center justify-between hover:bg-gray-50">
        <div>
          <div className="flex items-center gap-2">
            <p className="font-medium text-sm">{p.name}</p>
            <span className={`px-2 py-0.5 rounded text-xs ${p.is_active?"bg-green-100 text-green-700":"bg-gray-100 text-gray-500"}`}>{p.is_active?"启用":"已停用"}</span>
            <span className="px-2 py-0.5 rounded text-xs bg-blue-50 text-blue-700">{p.fallback_policy}</span>
          </div>
          <p className="text-xs text-gray-500 mt-0.5">等级:{p.user_level||"不限"} · 优先级:{p.priority} · Channels:{p.candidate_channel_names?.join(",")||p.candidate_channel_ids.length+"个"}</p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={()=>{setEditing(p);setName(p.name);setUserLevel(p.user_level);setModelId(p.model_id);setPriority(p.priority);setChannelIds(p.candidate_channel_ids.join(","));setFallback(p.fallback_policy);setTenantId(p.tenant_id||"");setShowForm(true);}} className="p-1.5 text-gray-400 hover:text-gray-600 rounded"><Edit size={14}/></button>
          <button onClick={()=>handleDelete(p)} className="p-1.5 text-gray-400 hover:text-red-600 rounded"><Trash2 size={14}/></button>
        </div>
      </div>)}
    </div>
  </div>;
}
