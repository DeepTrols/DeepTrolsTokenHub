import { EmptyState, LoadingState } from "@/components/StateViews";
import { SectionPageLayout } from "@/components/SectionPageLayout";
import { useAdminQuery } from "../lib/hooks/use-api";
import { BarChart3 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import "../i18n";
import { useTranslation } from "react-i18next";

interface ReconciliationRun {
  id: string; run_type: string; status: string; started_at: string; completed_at: string | null;
  total_usage_logs: number; matched_count: number; diff_count: number; period_start: string; period_end: string;
}

function fmtDT(iso: string): string { try { return new Date(iso).toLocaleString("zh-CN",{year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit"}); } catch { return iso; } }
const ll: Record<string,string> = { L0:"reconciliation.runL0", L1:"reconciliation.runL1" };
function sv(s: string): "success" | "secondary" | "destructive" | "outline" { if(s==="completed") return "success"; if(s==="running") return "secondary"; if(s==="failed") return "destructive"; return "outline"; }
const sl: Record<string,string> = { completed:"reconciliation.statusCompleted", running:"reconciliation.statusRunning", failed:"reconciliation.statusFailed" };
function mr(m:number,t:number):number { return t<=0?100:Math.round((m/t)*100); }
function rc(r:number):string { if(r>=99) return "text-[#0C7A55]"; if(r>=95) return "text-[#A06B12]"; return "text-destructive"; }

export default function Reconciliation() {
  const { t } = useTranslation();
  const { data: runData, isLoading, isError, error, refetch } = useAdminQuery<{ data: ReconciliationRun[] }>("/reconciliation");
  const { data: summary } = useAdminQuery<{
    usage_logs: number;
    charge_lines: number;
    usage_missing_charge: number;
    balanced: boolean;
    l2?: {
      usage_logs: number;
      with_charge: number;
      with_evidence: number;
      both_missing: number;
      balanced: boolean;
      available: boolean;
    };
  }>("/reconciliation/summary");
  const runs = Array.isArray(runData?.data) ? runData.data : [];
  const loadError = isError ? (error instanceof Error ? error.message : String(error)) : "";
  if (isLoading) return <SectionPageLayout><SectionPageLayout.Header><SectionPageLayout.HeaderBlock><SectionPageLayout.Title>{t("reconciliation.title")}</SectionPageLayout.Title></SectionPageLayout.HeaderBlock></SectionPageLayout.Header><SectionPageLayout.Content><LoadingState message={t("reconciliation.loading")} /></SectionPageLayout.Content></SectionPageLayout>;
  const tr=runs.length, cr=runs.filter(r=>r.status==="completed").length, td=runs.reduce((s,r)=>s+r.diff_count,0);
  return (
    <div>
      <div className="mb-6"><h2 className="font-display text-[25px] font-bold tracking-tight">{t("reconciliation.title")}</h2><p className="text-[13px] text-[#5C6472] mt-1">{t("reconciliation.subtitle")}</p></div>
      {loadError && <Card className="mb-4 border-destructive/20"><CardContent className="p-4 text-destructive text-sm"><p className="font-medium">{t("reconciliation.loadFailed")}</p><p className="mt-1 text-xs break-all">{loadError}</p><Button variant="destructive" size="sm" className="mt-2" onClick={()=>refetch()}>{t("reconciliation.retry")}</Button></CardContent></Card>}
      <div className="grid grid-cols-3 gap-4 mb-6">{[{l:t("reconciliation.statTotal"),v:tr},{l:t("reconciliation.statCompleted"),v:cr},{l:t("reconciliation.statDiffs"),v:td}].map(c=><Card key={c.l}><CardContent className="p-5"><p className="text-[12px] font-semibold text-[#5C6472]">{c.l}</p><p className="font-mono text-[24px] font-semibold tracking-tight mt-1">{String(c.v)}</p></CardContent></Card>)}</div>
      <div className="grid grid-cols-3 gap-4 mb-6">{[{ l: t("reconciliation.statUsage"), v: summary?.usage_logs ?? 0 }, { l: t("reconciliation.statCharge"), v: summary?.charge_lines ?? 0 }, { l: t("reconciliation.statMissing"), v: summary?.usage_missing_charge ?? 0 }].map((c) => <Card key={c.l}><CardContent className="p-5"><p className="text-[12px] font-semibold text-[#5C6472]">{c.l}</p><p className="font-mono text-[24px] font-semibold tracking-tight mt-1">{String(c.v)}</p></CardContent></Card>)}</div>
      <p className="text-xs text-[#5C6472] mb-4">{summary ? (summary.balanced ? t("reconciliation.balanced") : t("reconciliation.unbalanced")) : t("reconciliation.loadingShort")}</p>
      {summary?.l2 && (
        <p className="text-xs text-[#5C6472] mb-4">
          {t("reconciliation.l2Title")}：
          {summary.l2.available ? (
            <>
              {t("reconciliation.l2Line", {
                withCharge: summary.l2.with_charge,
                withEvidence: summary.l2.with_evidence,
                usage: summary.l2.usage_logs,
                both: summary.l2.both_missing,
              })}
              {" · "}
              <span className={summary.l2.balanced ? "text-[#0C7A55]" : "text-[#C4372C]"}>
                {summary.l2.balanced ? t("reconciliation.l2Balanced") : t("reconciliation.l2Unbalanced")}
              </span>
            </>
          ) : (
            t("reconciliation.l2Empty")
          )}
        </p>
      )}
      <Card className="overflow-hidden"><Table>
        <TableHeader><TableRow><TableHead>{t("reconciliation.thLevel")}</TableHead><TableHead>{t("reconciliation.thStatus")}</TableHead><TableHead>{t("reconciliation.thPeriod")}</TableHead><TableHead>{t("reconciliation.thStart")}</TableHead><TableHead>{t("reconciliation.thEnd")}</TableHead><TableHead className="text-right">{t("reconciliation.thRecords")}</TableHead><TableHead className="text-right">{t("reconciliation.thMatched")}</TableHead><TableHead className="text-right">{t("reconciliation.thDiff")}</TableHead><TableHead className="text-right">{t("reconciliation.thRate")}</TableHead></TableRow></TableHeader>
        <TableBody>{runs.length===0 && <TableRow><TableCell colSpan={9}><EmptyState icon={BarChart3} title={t("reconciliation.empty")} /></TableCell></TableRow>}
        {runs.map(r=>{const rate=mr(r.matched_count,r.total_usage_logs);return <TableRow key={r.id}><TableCell className="font-medium">{t(ll[r.run_type]||r.run_type)}</TableCell><TableCell><Badge variant={sv(r.status)}>{t(sl[r.status]||r.status)}</Badge></TableCell><TableCell className="text-muted-foreground text-xs">{fmtDT(r.period_start)} ~ {fmtDT(r.period_end)}</TableCell><TableCell className="text-xs">{fmtDT(r.started_at)}</TableCell><TableCell className="text-xs">{r.completed_at?fmtDT(r.completed_at):"-"}</TableCell><TableCell className="text-right font-mono text-xs">{r.total_usage_logs}</TableCell><TableCell className="text-right font-mono text-xs text-[#0C7A55]">{r.matched_count}</TableCell><TableCell className="text-right font-mono text-xs">{r.diff_count>0?<span className="text-destructive">{r.diff_count}</span>:<span>0</span>}</TableCell><TableCell className="text-right"><span className={"font-mono font-medium "+rc(rate)}>{rate}%</span></TableCell></TableRow>})}</TableBody>
      </Table></Card>
    </div>
  );
}
