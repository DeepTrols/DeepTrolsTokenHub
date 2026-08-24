import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
      <div><h2 className="font-display text-[25px] font-bold tracking-tight">路由策略</h2><p className="text-[13px] text-[#5C6472] mt-1">配置请求路由规则和 fallback 策略</p></div>
      <Button onClick={()=>{reset();setShowForm(true);}}><Plus size={16} className="mr-1.5"/>创建策略</Button>
    </div>

    {showForm&&<div className="mb-6 p-6 glass rounded-2xl">
      <h3 className="font-display font-semibold mb-4">{editing?"编辑策略":"创建策略"}</h3>
      <div className="grid grid-cols-2 gap-4">
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">名称*</label><Input aria-label="策略名称" value={name} onChange={e=>setName(e.target.value)}/></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">用户等级</label><Input value={userLevel} onChange={e=>setUserLevel(e.target.value)} placeholder="vip / free"/></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">模型ID*</label><Input aria-label="模型编码" value={modelId} onChange={e=>setModelId(e.target.value)} className="font-mono"/></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">优先级</label><Input type="number" value={priority} onChange={e=>setPriority(+e.target.value)}/></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">Channel IDs</label><Input value={channelIds} onChange={e=>setChannelIds(e.target.value)} placeholder="UUID1,UUID2" className="font-mono"/></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">Fallback</label><select value={fallback} onChange={e=>setFallback(e.target.value)} className="w-full glass-soft rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20">{FALLBACKS.map(f=><option key={f.v} value={f.v}>{f.l}</option>)}</select></div>
        <div><label className="block text-[12px] font-semibold text-[#5C6472] mb-1.5">租户ID</label><Input value={tenantId} onChange={e=>setTenantId(e.target.value)} placeholder="留空=平台级" className="font-mono"/></div>
      </div>
      <div className="flex gap-3 mt-6"><Button onClick={handleSubmit}>{editing?"保存":"创建"}</Button><Button variant="outline" onClick={reset}>取消</Button></div>
    </div>}

    {loadError && <ErrorState error={loadError} onRetry={()=>refetch()} />}
    {isLoading && <LoadingState message="加载路由策略..." />}
    <div className="glass rounded-2xl overflow-hidden">
      {!isLoading && policies.length===0&&<EmptyState icon={GitBranch} title="暂无路由策略" />}
      {policies.map(p=><div key={p.id} className="p-4 border-b border-black/[0.06] flex items-center justify-between hover:bg-white/60 transition-colors">
        <div>
          <div className="flex items-center gap-2">
            <p className="font-medium text-sm">{p.name}</p>
            <span className={`status-pill ${p.is_active?"ok":"run"}`}><i/>{p.is_active?"启用":"已停用"}</span>
            <span className="status-pill text-[#4F6BED]"><i className="bg-[#4F6BED] shadow-[0_0_8px_#4F6BED]"/>{p.fallback_policy}</span>
          </div>
          <p className="text-xs text-[#5C6472] mt-0.5">等级:{p.user_level||"不限"} · 优先级:{p.priority} · Channels:{p.candidate_channel_names?.join(",")||p.candidate_channel_ids.length+"个"}</p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={()=>{setEditing(p);setName(p.name);setUserLevel(p.user_level);setModelId(p.model_id);setPriority(p.priority);setChannelIds(p.candidate_channel_ids.join(","));setFallback(p.fallback_policy);setTenantId(p.tenant_id||"");setShowForm(true);}} className="p-1.5 text-[#5C6472]/70 hover:text-[#161A23] rounded-lg hover:bg-white/70 transition-colors"><Edit size={14}/></button>
          <button onClick={()=>handleDelete(p)} className="p-1.5 text-[#5C6472]/70 hover:text-[#C4372C] rounded-lg hover:bg-white/70 transition-colors"><Trash2 size={14}/></button>
        </div>
      </div>)}
    </div>
  </div>;
}
