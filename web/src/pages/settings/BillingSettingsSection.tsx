import React, { useEffect, useState } from "react";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { useAdminQuery, useAdminMutation } from "@/lib/hooks/use-api";
import { PaymentOrder } from "@/lib/api";
import "../../i18n";
import { useTranslation } from "react-i18next";

type SiteSettings = Record<string, string>;

interface UserGroupEntry {
  name: string;
  ratio: string;
}

interface DiscountTierEntry {
  min_tokens: string;
  ratio: string;
}

const PAYMENT_KEYS = [
  "payment_enabled",
  "payment_compliance_confirmed",
  "pay_address",
  "epay_id",
  "epay_key",
  "min_topup",
  "max_topup",
  "amount_options",
  "callback_base_url",
] as const;

export default function BillingSettingsSection() {
  const { t } = useTranslation();
  const { data } = useAdminQuery<SiteSettings>("/settings/site");
  const [form, setForm] = useState<SiteSettings>({});
  const [groups, setGroups] = useState<UserGroupEntry[]>([]);
  const [tiers, setTiers] = useState<DiscountTierEntry[]>([]);

  useEffect(() => {
    if (!data) return;
    const next: SiteSettings = {};
    for (const k of Object.keys(data)) {
      const v = data[k];
      next[k] = typeof v === "string" ? v : JSON.stringify(v);
    }
    setForm(next);
    try {
      const parsed = JSON.parse(data.user_groups ?? "[]") as unknown;
      setGroups(Array.isArray(parsed) ? (parsed as UserGroupEntry[]) : []);
    } catch {
      setGroups([]);
    }
    try {
      const parsed = JSON.parse(data.discount_tiers ?? "[]") as unknown;
      setTiers(Array.isArray(parsed) ? (parsed as DiscountTierEntry[]) : []);
    } catch {
      setTiers([]);
    }
  }, [data]);

  const save = useAdminMutation<{ ok: boolean }, SiteSettings>("put", "/settings/site", "/settings/site", {
    onSuccess: () => toast.success(t("common.saved")),
    onError: (e) => toast.error(e instanceof Error ? e.message : t("common.saveFailed")),
  });

  const set = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [key]: e.target.value }));

  const savePayment = () => {
    const patch: SiteSettings = {};
    for (const k of PAYMENT_KEYS) patch[k] = form[k] ?? "";
    save.mutate(patch);
  };

  const updateGroup = (index: number, patch: Partial<UserGroupEntry>) =>
    setGroups((prev) => prev.map((g, i) => (i === index ? { ...g, ...patch } : g)));

  const addGroup = () => setGroups((prev) => [...prev, { name: "", ratio: "1" }]);

  const removeGroup = (index: number) => setGroups((prev) => prev.filter((_, i) => i !== index));

  const saveGroups = () => {
    const seen = new Set<string>();
    for (const g of groups) {
      const name = g.name.trim();
      if (!name) {
        toast.error(t("settings.groupsInvalidName"));
        return;
      }
      const ratio = Number(g.ratio);
      if (!g.ratio.trim() || !Number.isFinite(ratio) || ratio <= 0) {
        toast.error(t("settings.groupsInvalidRatio", { name }));
        return;
      }
      if (seen.has(name)) {
        toast.error(t("settings.groupsDuplicate", { name }));
        return;
      }
      seen.add(name);
    }
    save.mutate({
      user_groups: JSON.stringify(
        groups.map((g) => ({ name: g.name.trim(), ratio: g.ratio.trim() })),
      ),
    });
  };

  const updateTier = (index: number, patch: Partial<DiscountTierEntry>) =>
    setTiers((prev) => prev.map((g, i) => (i === index ? { ...g, ...patch } : g)));

  const addTier = () => setTiers((prev) => [...prev, { min_tokens: "0", ratio: "1" }]);

  const removeTier = (index: number) => setTiers((prev) => prev.filter((_, i) => i !== index));

  const saveTiers = () => {
    const seen = new Set<string>();
    for (const tier of tiers) {
      const tokens = Number(tier.min_tokens);
      if (!String(tier.min_tokens ?? "").trim() || !Number.isInteger(tokens) || tokens < 0) {
        toast.error(t("settings.discountsInvalidTokens"));
        return;
      }
      const ratio = Number(tier.ratio);
      if (!String(tier.ratio ?? "").trim() || !Number.isFinite(ratio) || ratio <= 0) {
        toast.error(t("settings.discountsInvalidRatio"));
        return;
      }
      const key = String(tokens);
      if (seen.has(key)) {
        toast.error(t("settings.discountsDuplicate"));
        return;
      }
      seen.add(key);
    }
    save.mutate({
      discount_tiers: JSON.stringify(
        tiers.map((tier) => ({ min_tokens: Number(tier.min_tokens), ratio: tier.ratio.trim() })),
      ),
    });
  };

  const ordersQuery = useAdminQuery<{ orders: PaymentOrder[] }>("/payment/orders");
  const completeOrder = useAdminMutation<{ ok: boolean }, { id: string }>(
    "post",
    (v) => `/payment/orders/${v.id}/complete`,
    "/payment/orders",
    { onSuccess: () => toast.success(t("settings.orderCompleted")) },
  );

  const field = (key: string, label: string, hint?: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={key}>{label}</Label>
      <Input id={key} value={form[key] ?? ""} onChange={set(key)} />
      {hint && <p className="text-xs text-[#5C6472]">{hint}</p>}
    </div>
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("settings.billingTitle")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("settings.billingSubtitle")}</p>
      </div>
      <Tabs defaultValue="channel">
        <TabsList>
          <TabsTrigger value="channel">{t("settings.tabChannel")}</TabsTrigger>
          <TabsTrigger value="orders">{t("settings.tabOrders")}</TabsTrigger>
          <TabsTrigger value="redemption">{t("settings.tabRedemption")}</TabsTrigger>
          <TabsTrigger value="groups">{t("settings.tabGroups")}</TabsTrigger>
          <TabsTrigger value="discounts">{t("settings.tabDiscounts")}</TabsTrigger>
        </TabsList>

        <TabsContent value="channel">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.channelTitle")}</CardTitle>
              <CardDescription>{t("settings.channelDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-3">
                <input
                  id="payment_enabled"
                  type="checkbox"
                  checked={form.payment_enabled === "true"}
                  onChange={(e) => setForm((p) => ({ ...p, payment_enabled: e.target.checked ? "true" : "false" }))}
                  className="accent-[#F78B28]"
                />
                <Label htmlFor="payment_enabled">{t("settings.enableTopup")}</Label>
              </div>
              <div className="flex items-center gap-3">
                <input
                  id="payment_compliance"
                  type="checkbox"
                  checked={form.payment_compliance_confirmed === "true"}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, payment_compliance_confirmed: e.target.checked ? "true" : "false" }))
                  }
                  className="accent-[#F78B28]"
                />
                <Label htmlFor="payment_compliance">{t("settings.compliance")}</Label>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                {field("pay_address", t("settings.payAddress"))}
                {field("epay_id", t("settings.epayId"))}
                {field("epay_key", t("settings.epayKey"))}
                {field("callback_base_url", t("settings.callbackBase"))}
                {field("min_topup", t("settings.minTopup"))}
                {field("max_topup", t("settings.maxTopup"))}
                {field("amount_options", t("settings.amountOptions"))}
              </div>
              <Button onClick={savePayment} disabled={save.isPending}>{t("common.save")}</Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="orders">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.ordersTitle")}</CardTitle>
              <CardDescription>{t("settings.ordersDesc")}</CardDescription>
              <Button variant="outline" size="sm" className="mt-1" onClick={() => exportOrders(ordersQuery.data?.orders ?? [], t)}>
                {t("common.exportCsv")}
              </Button>
            </CardHeader>
            <CardContent>
              {ordersQuery.isLoading ? (
                <p className="text-sm text-[#5C6472]">{t("common.loading")}</p>
              ) : (ordersQuery.data?.orders ?? []).length === 0 ? (
                <p className="text-sm text-[#5C6472]">{t("settings.noOrders")}</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-xs text-[#5C6472]">
                        <th className="py-2 pr-3">{t("settings.thOrderNo")}</th>
                        <th className="py-2 pr-3">{t("settings.thAmount")}</th>
                        <th className="py-2 pr-3">{t("settings.thMethod")}</th>
                        <th className="py-2 pr-3">{t("settings.thStatus")}</th>
                        <th className="py-2 pr-3">{t("settings.thTime")}</th>
                        <th className="py-2" />
                      </tr>
                    </thead>
                    <tbody>
                      {(ordersQuery.data?.orders ?? []).map((o) => (
                        <tr key={o.id} className="border-b border-black/5">
                          <td className="py-2 pr-3 font-mono text-xs">{o.order_no}</td>
                          <td className="py-2 pr-3">{o.amount}</td>
                          <td className="py-2 pr-3">{o.pay_method === "wxpay" ? t("settings.methodWx") : t("settings.methodAlipay")}</td>
                          <td className="py-2 pr-3">
                            <Badge variant={o.status === "paid" ? "default" : "secondary"}>{o.status}</Badge>
                          </td>
                          <td className="py-2 pr-3 text-xs text-[#5C6472]">{o.created_at}</td>
                          <td className="py-2 text-right">
                            {o.status !== "paid" && (
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => completeOrder.mutate({ id: o.id })}
                                disabled={completeOrder.isPending}
                              >
                                {t("settings.completeOrder")}
                              </Button>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="redemption">
          <RedemptionPanel />
        </TabsContent>

        <TabsContent value="groups">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.groupsTitle")}</CardTitle>
              <CardDescription>{t("settings.groupsDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                {groups.map((g, i) => (
                  <div key={i} className="flex items-end gap-2">
                    <div className="space-y-1.5 flex-1">
                      <Label htmlFor={`group-name-${i}`}>{t("settings.groupsName")}</Label>
                      <Input
                        id={`group-name-${i}`}
                        value={g.name}
                        onChange={(e) => updateGroup(i, { name: e.target.value })}
                        placeholder="vip"
                        className="font-mono"
                      />
                    </div>
                    <div className="space-y-1.5 w-28">
                      <Label htmlFor={`group-ratio-${i}`}>{t("settings.groupsRatio")}</Label>
                      <Input
                        id={`group-ratio-${i}`}
                        type="number"
                        min="0"
                        step="0.01"
                        value={g.ratio}
                        onChange={(e) => updateGroup(i, { ratio: e.target.value })}
                        className="font-mono"
                      />
                    </div>
                    <Button variant="outline" size="sm" onClick={() => removeGroup(i)}>
                      {t("settings.groupsRemove")}
                    </Button>
                  </div>
                ))}
                {groups.length === 0 && (
                  <p className="text-sm text-[#8C93A1]">{t("settings.groupsEmpty")}</p>
                )}
              </div>
              <p className="text-xs text-[#5C6472]">{t("settings.groupsHint")}</p>
              <div className="flex gap-2">
                <Button variant="outline" onClick={addGroup}>
                  {t("settings.groupsAdd")}
                </Button>
                <Button onClick={saveGroups} disabled={save.isPending}>
                  {t("common.save")}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="discounts">
          <Card>
            <CardHeader>
              <CardTitle>{t("settings.discountsTitle")}</CardTitle>
              <CardDescription>{t("settings.discountsDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                {tiers.map((tier, i) => (
                  <div key={i} className="flex items-end gap-2">
                    <div className="space-y-1.5 flex-1">
                      <Label htmlFor={`tier-tokens-${i}`}>{t("settings.discountsMinTokens")}</Label>
                      <Input
                        id={`tier-tokens-${i}`}
                        type="number"
                        min="0"
                        step="1"
                        value={tier.min_tokens}
                        onChange={(e) => updateTier(i, { min_tokens: e.target.value })}
                        className="font-mono"
                      />
                    </div>
                    <div className="space-y-1.5 w-28">
                      <Label htmlFor={`tier-ratio-${i}`}>{t("settings.discountsRatio")}</Label>
                      <Input
                        id={`tier-ratio-${i}`}
                        type="number"
                        min="0"
                        step="0.01"
                        value={tier.ratio}
                        onChange={(e) => updateTier(i, { ratio: e.target.value })}
                        className="font-mono"
                      />
                    </div>
                    <Button variant="outline" size="sm" onClick={() => removeTier(i)}>
                      {t("settings.discountsRemove")}
                    </Button>
                  </div>
                ))}
                {tiers.length === 0 && (
                  <p className="text-sm text-[#8C93A1]">{t("settings.discountsEmpty")}</p>
                )}
              </div>
              <p className="text-xs text-[#5C6472]">{t("settings.discountsHint")}</p>
              <div className="flex gap-2">
                <Button variant="outline" onClick={addTier}>
                  {t("settings.discountsAdd")}
                </Button>
                <Button onClick={saveTiers} disabled={save.isPending}>
                  {t("common.save")}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function exportOrders(orders: PaymentOrder[], t: (key: string) => string) {
  const header = [
    t("settings.thOrderNo"),
    t("settings.thAmount"),
    t("settings.thMethod"),
    t("settings.thStatus"),
    t("settings.thTime"),
  ];
  const lines = orders.map((o) =>
    [o.order_no, o.amount, o.pay_method, o.status, o.created_at].map(escCsv).join(","),
  );
  const blob = new Blob(["\uFEFF" + [header.join(","), ...lines].join("\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "zhiyao-tokenhub-orders.csv";
  a.click();
  URL.revokeObjectURL(url);
}

function escCsv(v: unknown): string {
  const s = String(v ?? "");
  return /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
}

function RedemptionPanel() {
  const { t } = useTranslation();
  const [amount, setAmount] = useState("50");
  const [count, setCount] = useState("10");
  const [made, setMade] = useState<string[]>([]);
  const list = useAdminQuery<{ codes: { code: string; amount: string; status: string; created_at: string }[] }>("/redemption");
  const create = useAdminMutation<{ created: number; codes: string[] }, { amount: string; count: number }>(
    "post",
    "/redemption",
    "/redemption",
    { onSuccess: (r) => setMade(r.codes ?? []) },
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.redemptionTitle")}</CardTitle>
        <CardDescription>{t("settings.redemptionDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1.5">
            <Label>{t("common.amount")}</Label>
            <Input value={amount} onChange={(e) => setAmount(e.target.value)} className="w-32" />
          </div>
          <div className="space-y-1.5">
            <Label>{t("common.count")}</Label>
            <Input value={count} onChange={(e) => setCount(e.target.value)} className="w-24" />
          </div>
          <Button onClick={() => create.mutate({ amount, count: Number(count) || 0 })} disabled={create.isPending}>
            {create.isPending ? t("common.generating") : t("common.generate")}
          </Button>
        </div>
        {made.length > 0 && (
          <div className="glass-soft rounded-xl p-3">
            <p className="text-xs text-[#5C6472] mb-1">{t("settings.generated", { count: made.length })}</p>
            <div className="flex flex-wrap gap-1.5">
              {made.map((c) => (
                <code key={c} className="px-2 py-0.5 rounded-lg bg-white/70 text-xs font-mono">{c}</code>
              ))}
            </div>
          </div>
        )}
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-[#5C6472]">
                <th className="py-2 pr-3">{t("redemption.thCode")}</th>
                <th className="py-2 pr-3">{t("settings.thAmount")}</th>
                <th className="py-2 pr-3">{t("settings.thStatus")}</th>
                <th className="py-2 pr-3">{t("redemption.thCreated")}</th>
              </tr>
            </thead>
            <tbody>
              {(list.data?.codes ?? []).map((c) => (
                <tr key={c.code} className="border-b border-black/5">
                  <td className="py-2 pr-3 font-mono text-xs">{c.code}</td>
                  <td className="py-2 pr-3">{c.amount}</td>
                  <td className="py-2 pr-3"><Badge variant={c.status === "active" ? "success" : "secondary"}>{c.status}</Badge></td>
                  <td className="py-2 pr-3 text-xs text-[#5C6472]">{c.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  );
}
