import { useState, useEffect } from "react";
import { useAuth } from "../lib/auth";
import { useConsoleQuery, useConsoleMutation } from "../lib/hooks/use-api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ErrorState } from "@/components/StateViews";
import { Building2, Globe, Save } from "lucide-react";

interface Domain {
  id: string;
  domain: string;
  is_primary: boolean;
}

interface EnterpriseData {
  id: string;
  code: string;
  name: string;
  status: string;
  credit_code?: string;
  contact_email: string;
  contact_phone: string;
  business_license?: string;
  brand_config: Record<string, unknown>;
  runtime_config: Record<string, unknown>;
  settlement_config?: Record<string, unknown>;
  member_count: number;
  domains: Domain[];
}

const STATUS_META: Record<string, { label: string; variant: "success" | "destructive" | "secondary" | "outline" }> = {
  active: { label: "已激活", variant: "success" },
  pending_review: { label: "待审核", variant: "secondary" },
  suspended: { label: "已停用", variant: "destructive" },
  terminated: { label: "已终止", variant: "destructive" },
  rejected: { label: "已拒绝", variant: "outline" },
};

export default function EnterpriseSettings() {
  const { user } = useAuth();
  const isOwner = user?.tenant_role === "owner";
  const isAdmin = isOwner || user?.tenant_role === "admin";

  const { data, isLoading, isError, refetch } = useConsoleQuery<EnterpriseData>("/enterprise");

  const [nm, setNm] = useState("");
  const [ce, setCe] = useState("");
  const [cp, setCp] = useState("");
  const [bc, setBc] = useState("");
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  // Keep the editable fields in sync with server state after load or save.
  useEffect(() => {
    if (!data) return;
    setNm(data.name ?? "");
    setCe(data.contact_email ?? "");
    setCp(data.contact_phone ?? "");
    setBc(JSON.stringify(data.brand_config ?? {}, null, 2));
  }, [data]);

  const upd = useConsoleMutation<unknown, { name?: string; contact_email?: string; contact_phone?: string }>(
    "put", "/enterprise", "/enterprise",
  );
  const updBrand = useConsoleMutation<unknown, { brand_config: Record<string, unknown> }>(
    "put", "/enterprise/brand", "/enterprise",
  );

  const saveInfo = async () => {
    setErr("");
    setMsg("");
    const body: { name?: string; contact_email?: string; contact_phone?: string } = {};
    if (isOwner && nm.trim()) body.name = nm.trim();
    if (ce.trim()) body.contact_email = ce.trim();
    body.contact_phone = cp.trim();
    try {
      await upd.mutateAsync(body);
      setMsg("企业信息已保存");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const saveBrand = async () => {
    setErr("");
    setMsg("");
    let parsed: unknown;
    try {
      parsed = JSON.parse(bc);
    } catch {
      setErr("品牌配置不是合法的 JSON");
      return;
    }
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
      setErr("品牌配置必须是 JSON 对象");
      return;
    }
    try {
      await updBrand.mutateAsync({ brand_config: parsed as Record<string, unknown> });
      setMsg("品牌配置已保存");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  if (isLoading) {
    return (
      <div>
        <h2 className="text-2xl font-bold mb-6">企业设置</h2>
        <Card>
          <CardContent className="p-12 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full mx-auto mb-3" />
            <p className="text-muted-foreground">加载中...</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <h2 className="text-2xl font-bold mb-6">企业设置</h2>
        <ErrorState onRetry={() => refetch()} />
      </div>
    );
  }

  const info = data as EnterpriseData;
  const status = STATUS_META[info.status];

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold">企业设置</h2>
        <p className="text-sm text-muted-foreground mt-1 flex items-center gap-2">
          <code>{info.code}</code>
          <Badge variant={status?.variant ?? "outline"}>{status?.label ?? info.status}</Badge>
          <span className="inline-flex items-center gap-1"><Building2 size={14} />{info.member_count} 名成员</span>
        </p>
      </div>

      {msg && <Card className="mb-4 border-green-500/30"><CardContent className="p-3 text-sm text-green-600">{msg}</CardContent></Card>}
      {err && <Card className="mb-4 border-destructive/30"><CardContent className="p-3 text-sm text-destructive">{err}</CardContent></Card>}

      {/* 基本信息 */}
      <Card className="mb-4">
        <CardContent className="p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="font-semibold">基本信息</h3>
            <Button size="sm" onClick={saveInfo} disabled={!isAdmin || upd.isPending}>
              <Save size={14} className="mr-1.5" />保存信息
            </Button>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label>企业名称</Label>
              <Input value={nm} onChange={e => setNm(e.target.value)} disabled={!isOwner} />
              {!isOwner && <p className="text-xs text-muted-foreground">仅所有者可修改企业名称</p>}
            </div>
            <div className="space-y-2">
              <Label>联系邮箱</Label>
              <Input value={ce} onChange={e => setCe(e.target.value)} type="email" disabled={!isAdmin} />
            </div>
            <div className="space-y-2">
              <Label>联系电话</Label>
              <Input value={cp} onChange={e => setCp(e.target.value)} disabled={!isAdmin} />
            </div>
          </div>
          {!isAdmin && <p className="text-xs text-muted-foreground">仅企业管理员及以上可编辑企业信息</p>}
        </CardContent>
      </Card>

      {/* 资质信息（只读，由管理员审核） */}
      <Card className="mb-4">
        <CardContent className="p-5">
          <h3 className="font-semibold mb-3">资质信息</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div><Label>统一社会信用代码</Label><p className="text-sm mt-1">{info.credit_code || "—"}</p></div>
            <div><Label>营业执照号</Label><p className="text-sm mt-1">{info.business_license || "—"}</p></div>
          </div>
          {!isAdmin && <p className="text-xs text-muted-foreground mt-3">资质信息仅企业管理员及以上可见</p>}
        </CardContent>
      </Card>

      {/* 品牌配置（仅所有者可编辑） */}
      {isOwner && (
        <Card className="mb-4">
          <CardContent className="p-5 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="font-semibold">品牌配置</h3>
                <p className="text-xs text-muted-foreground mt-0.5">JSON 对象，最多 50 个键</p>
              </div>
              <Button size="sm" onClick={saveBrand} disabled={updBrand.isPending}>
                <Save size={14} className="mr-1.5" />保存品牌配置
              </Button>
            </div>
            <textarea
              value={bc}
              onChange={e => setBc(e.target.value)}
              spellCheck={false}
              placeholder="{ }"
              className="flex min-h-[180px] w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
            />
          </CardContent>
        </Card>
      )}

      {/* 域名列表（只读） */}
      <Card>
        <CardContent className="p-5">
          <h3 className="font-semibold mb-3">企业域名</h3>
          {(info.domains ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无域名</p>
          ) : (
            <div className="space-y-1.5">
              {info.domains.map(d => (
                <div key={d.id} className="flex items-center justify-between p-2 bg-muted rounded-lg">
                  <div className="flex items-center gap-2">
                    <Globe size={14} className="text-muted-foreground" />
                    <code className="text-sm">{d.domain}</code>
                    {d.is_primary && <Badge variant="secondary" className="text-xs">主域名</Badge>}
                  </div>
                </div>
              ))}
            </div>
          )}
          <p className="text-xs text-muted-foreground mt-3">域名自助管理将在后续版本开放</p>
        </CardContent>
      </Card>
    </div>
  );
}
