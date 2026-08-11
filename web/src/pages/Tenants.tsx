import { EmptyState, ErrorState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { useState } from "react";import { useNavigate } from "react-router-dom";import { useAdminMutation, useAdminQuery } from "../lib/hooks/use-api";import { Plus, Edit, Trash2, Users, Globe, Settings2 } from "lucide-react";
interface Domain { id:string;domain:string;is_primary:boolean }
interface TenantData { id:string;code:string;name:string;status:string;status_reason?:string;member_count?:number;domains?:Domain[] }
const SS=[{v:"pending_review",l:"待审核"},{v:"active",l:"已激活"},{v:"suspended",l:"已停用"},{v:"terminated",l:"已终止"},{v:"rejected",l:"已拒绝"}];
function sb(s:string):"success"|"destructive"|"secondary"|"outline" {if(s==="active")return"success";if(s==="suspended"||s==="terminated")return"destructive";if(s==="pending_review")return"secondary";return"outline"}
function sl(s:string):string {return SS.find(x=>x.v===s)?.l||s}
export default function Tenants(){
  const nav=useNavigate();
  const{data:td,isLoading,isError,error,refetch}=useAdminQuery<{data:TenantData[]}>("/tenants");
  const tenants=td?.data??[];
  const le=isError?(error instanceof Error?error.message:String(error)):"";
  const[di,setDi]=useState<string|null>(null);
  const{data:detail}=useAdminQuery<TenantData>(di?"/tenants/"+di:"",{enabled:!!di});
  const[sf,setSf]=useState(false);const[ed,setEd]=useState<TenantData|null>(null);
  const[nm,setNm]=useState("");const[cd,setCd]=useState("");const[st,setSt]=useState("pending_review");const[rs,setRs]=useState("");
  const[df,setDf]=useState<string|null>(null);const[dn,setDn]=useState("");const[ip,setIp]=useState(false);
  const cM=useAdminMutation<unknown,Record<string,unknown>>("post","/tenants");
  const uM=useAdminMutation<unknown,{id:string}&Record<string,unknown>>("put",(v:any)=>"/tenants/"+v.id,"/tenants");
  const dM=useAdminMutation<unknown,{id:string}>("delete",(v:any)=>"/tenants/"+v.id,"/tenants");
  const aM=useAdminMutation<unknown,{id:string;domain:string;is_primary:boolean}>("post",(v:any)=>"/tenants/"+v.id+"/domains",(v:any)=>"/tenants/"+v.id);
  const rM=useAdminMutation<unknown,{id:string;domainId:string}>("delete",(v:any)=>"/tenants/"+v.id+"/domains/"+v.domainId,(v:any)=>"/tenants/"+v.id);
  const reset=()=>{setNm("");setCd("");setSt("pending_review");setRs("");setEd(null);setSf(false)};
  const hs=async()=>{if(!nm.trim()||!cd.trim())return;const b:Record<string,unknown>={name:nm.trim(),code:cd.trim()};if(ed){b.status=st;if(rs)b.status_reason=rs}if(ed)await uM.mutateAsync({id:ed.id,...b});else await cM.mutateAsync(b);reset()};
  const hd=async(t:TenantData)=>{if(!confirm("Terminate "+t.name+"?"))return;await dM.mutateAsync({id:t.id})};
  const ad=async(tid:string)=>{if(!dn.trim())return;await aM.mutateAsync({id:tid,domain:dn.trim(),is_primary:ip});setDf(null);setDn("")};
  const rd=async(tid:string,did:string)=>{await rM.mutateAsync({id:tid,domainId:did})};
  if(isLoading)return <div><h2 className="text-2xl font-bold mb-6">租户管理</h2><Card><CardContent className="p-12 text-center"><div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3"/><p className="text-muted-foreground">加载中...</p></CardContent></Card></div>;
  return <div>
    <div className="flex items-center justify-between mb-6"><div><h2 className="text-2xl font-bold">租户管理</h2><p className="text-sm text-muted-foreground mt-1">管理租户生命周期</p></div><Button onClick={()=>{reset();setSf(true)}}><Plus size={16} className="mr-1.5"/>创建租户</Button></div>
    {le&&<Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><Button variant="destructive" size="sm" onClick={()=>refetch()}>重试</Button></CardContent></Card>}
    {!isLoading&&tenants.length===0&&<Card><CardContent className="p-12 text-center text-muted-foreground"><Users size={40} className="mx-auto mb-3 opacity-30"/><p>暂无租户</p></CardContent></Card>}
    <div className="space-y-2">{tenants.map(t=><Card key={t.id}><CardContent className="p-4"><div className="flex items-center justify-between"><div className="flex-1 cursor-pointer" onClick={()=>di===t.id?setDi(null):setDi(t.id)}><div className="flex items-center gap-2"><p className="font-medium text-sm">{t.name}</p><Badge variant={sb(t.status)}>{sl(t.status)}</Badge></div><p className="text-xs text-muted-foreground mt-0.5"><code>{t.code}</code><span className="ml-2 inline-flex items-center gap-1"><Users size={12}/>{t.member_count ?? 0} 名成员</span>{t.status_reason?" · "+t.status_reason:""}</p></div><div className="flex items-center gap-2"><Button variant="outline" size="sm" onClick={()=>nav("/admin/tenants/"+t.id+"/members")}><Settings2 size={14} className="mr-1"/>管理成员</Button><Button variant="ghost" size="icon" onClick={()=>{setEd(t);setNm(t.name);setCd(t.code);setSt(t.status);setRs(t.status_reason||"");setSf(true)}}><Edit size={14}/></Button><Button variant="ghost" size="icon" className="hover:text-destructive" onClick={()=>hd(t)}><Trash2 size={14}/></Button></div></div>
    {di===t.id&&detail&&<div className="mt-3 pt-3 border-t"><div className="flex items-center justify-between mb-3"><h4 className="text-sm font-semibold">域名</h4><Button variant="outline" size="sm" onClick={()=>setDf(t.id)}><Plus size={12} className="mr-1"/>添加</Button></div>
    {df===t.id&&<div className="flex gap-2 items-end mb-3"><Input value={dn} onChange={e=>setDn(e.target.value)} placeholder="api.example.com" className="flex-1 h-8 text-xs"/><label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={ip} onChange={e=>setIp(e.target.checked)}/>主域名</label><Button size="sm" onClick={()=>ad(t.id)}>添加</Button><Button variant="outline" size="sm" onClick={()=>setDf(null)}>取消</Button></div>}
    {(detail.domains||[]).length===0?<p className="text-xs text-muted-foreground">暂无域名</p>:detail.domains?.map(d=><div key={d.id} className="flex items-center justify-between p-2 bg-muted rounded-lg mb-1"><div className="flex items-center gap-2"><Globe size={14} className="text-muted-foreground"/><code className="text-xs">{d.domain}</code>{d.is_primary&&<Badge variant="secondary" className="text-xs">主域名</Badge>}</div><Button variant="ghost" size="icon" className="h-6 w-6 hover:text-destructive" onClick={()=>rd(t.id,d.id)}><Trash2 size={12}/></Button></div>)}</div>}</CardContent></Card>)}</div>
    <Dialog open={sf} onOpenChange={setSf}><DialogContent><DialogHeader><DialogTitle>{ed?"编辑租户":"创建租户"}</DialogTitle></DialogHeader>
    <div className="grid grid-cols-2 gap-4"><div className="space-y-2"><Label>名称*</Label><Input value={nm} onChange={e=>setNm(e.target.value)}/></div><div className="space-y-2"><Label>编码*</Label><Input value={cd} onChange={e=>setCd(e.target.value)} disabled={!!ed}/></div>
    {ed&&<><div className="space-y-2"><Label>状态</Label><Select value={st} onValueChange={setSt}><SelectTrigger><SelectValue/></SelectTrigger><SelectContent>{SS.map(s=><SelectItem key={s.v} value={s.v}>{s.l}</SelectItem>)}</SelectContent></Select></div><div className="space-y-2"><Label>原因</Label><Input value={rs} onChange={e=>setRs(e.target.value)}/></div></>}</div>
    <DialogFooter><Button variant="outline" onClick={reset}>取消</Button><Button onClick={hs}>{ed?"保存":"创建"}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}