"use client";

import { useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { toast } from "sonner";
import { ArrowLeft, Loader2, Pencil, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { api, type AdminTenant } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";

function fmt(limit: number): string {
  return limit === -1 ? "∞" : String(limit);
}

// Default quotas per plan, mirroring the backend config defaults.
// Changing the plan pre-fills the quota fields with these values.
const PLAN_QUOTAS: Record<string, { doc_quota: number; vector_quota: number; message_quota: number; user_message_quota: number }> = {
  free: { doc_quota: 100, vector_quota: 10000, message_quota: 100, user_message_quota: 20 },
  pro: { doc_quota: 1000, vector_quota: 100000, message_quota: 1000, user_message_quota: 100 },
  enterprise: { doc_quota: 10000, vector_quota: 1000000, message_quota: -1, user_message_quota: -1 },
};

export default function TenantsPage() {
  const t = useTranslations();
  const { data, mutate } = useSWR("admin-tenants", () => api.listAdminTenants());

  const [editing, setEditing] = useState<AdminTenant | null>(null);
  const [form, setForm] = useState({ doc_quota: 0, vector_quota: 0, message_quota: 0, user_message_quota: 0 });
  const [plan, setPlan] = useState("free");
  const [saving, setSaving] = useState(false);

  function openEdit(tn: AdminTenant) {
    setEditing(tn);
    setPlan(tn.plan);
    setForm({
      doc_quota: tn.doc_quota,
      vector_quota: tn.vector_quota,
      message_quota: tn.message_quota,
      user_message_quota: tn.user_message_quota,
    });
  }

  function closeEdit() {
    setEditing(null);
  }

  function changePlan(p: string) {
    setPlan(p);
    const q = PLAN_QUOTAS[p];
    if (q) setForm(q);
  }

  async function saveQuota() {
    if (!editing) return;
    setSaving(true);
    try {
      if (plan !== editing.plan) {
        await api.updateTenantPlan(editing.id, plan);
      }
      await api.updateTenantQuota(editing.id, form);
      toast.success(t("tenants.saved"));
      closeEdit();
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link
          href="/dashboard"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4 mr-1" /> {t("common.back")}
        </Link>
        <h1 className="text-2xl font-semibold tracking-tight">{t("tenants.title")}</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("tenants.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground border-b">
                  <th className="py-2 pr-4">{t("tenants.name")}</th>
                  <th className="py-2 pr-4">{t("team.plan")}</th>
                  <th className="py-2 pr-4">{t("tenants.members")}</th>
                  <th className="py-2 pr-4">{t("tenants.doc_usage")}</th>
                  <th className="py-2 pr-4">{t("tenants.msg_usage")}</th>
                  <th className="py-2 pr-4">{t("tenants.user_msg_quota")}</th>
                  <th className="py-2 pr-4">{t("tenants.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {!data &&
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b last:border-0">
                      {Array.from({ length: 7 }).map((_, j) => (
                        <td key={j} className="py-2 pr-4">
                          <Skeleton className="h-4 w-20" />
                        </td>
                      ))}
                    </tr>
                  ))}
                {data?.map((tn) => (
                  <tr key={tn.id} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-medium">{tn.name}</td>
                    <td className="py-2 pr-4">
                      <span className="inline-flex items-center rounded-md bg-muted px-2 py-0.5 text-xs font-medium">
                        {tn.plan === "pro"
                          ? t("team.plan_pro")
                          : tn.plan === "enterprise"
                            ? t("team.plan_enterprise")
                            : t("team.plan_free")}
                      </span>
                    </td>
                    <td className="py-2 pr-4">{tn.member_count}</td>
                    <td className="py-2 pr-4">
                      {tn.doc_used}/{fmt(tn.doc_quota)}
                    </td>
                    <td className="py-2 pr-4">
                      {tn.msg_used}/{fmt(tn.message_quota)}
                    </td>
                    <td className="py-2 pr-4">{fmt(tn.user_message_quota)}</td>
                    <td className="py-2 pr-4">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(tn)}>
                        <Pencil className="h-3.5 w-3.5 mr-1" />
                        {t("tenants.edit_quota")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Edit quota dialog */}
      {editing && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={closeEdit} aria-hidden="true" />
          <div className="relative w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
            <button
              onClick={closeEdit}
              className="absolute right-3 top-3 text-muted-foreground hover:text-foreground"
              aria-label={t("common.close")}
            >
              <X className="h-4 w-4" />
            </button>
            <h3 className="text-lg font-semibold mb-1">{t("tenants.edit_quota_title")}</h3>
            <p className="text-sm text-muted-foreground mb-4">{editing.name}</p>
            <div className="space-y-1 mb-4">
              <Label>{t("tenants.plan")}</Label>
              <select
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                value={plan}
                onChange={(e) => changePlan(e.target.value)}
              >
                <option value="free">{t("team.plan_free")}</option>
                <option value="pro">{t("team.plan_pro")}</option>
                <option value="enterprise">{t("team.plan_enterprise")}</option>
              </select>
              <p className="text-xs text-muted-foreground">{t("tenants.plan_hint")}</p>
            </div>
            <p className="text-xs text-muted-foreground mb-4">{t("tenants.unlimited_hint")}</p>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1">
                <Label>{t("tenants.doc_quota")}</Label>
                <Input
                  type="number"
                  min={-1}
                  value={form.doc_quota}
                  onChange={(e) => setForm((f) => ({ ...f, doc_quota: Number(e.target.value) }))}
                />
              </div>
              <div className="space-y-1">
                <Label>{t("tenants.vector_quota")}</Label>
                <Input
                  type="number"
                  min={-1}
                  value={form.vector_quota}
                  onChange={(e) => setForm((f) => ({ ...f, vector_quota: Number(e.target.value) }))}
                />
              </div>
              <div className="space-y-1">
                <Label>{t("tenants.team_msg_quota")}</Label>
                <Input
                  type="number"
                  min={-1}
                  value={form.message_quota}
                  onChange={(e) => setForm((f) => ({ ...f, message_quota: Number(e.target.value) }))}
                />
              </div>
              <div className="space-y-1">
                <Label>{t("tenants.user_msg_quota")}</Label>
                <Input
                  type="number"
                  min={-1}
                  value={form.user_message_quota}
                  onChange={(e) => setForm((f) => ({ ...f, user_message_quota: Number(e.target.value) }))}
                />
              </div>
            </div>
            <div className="mt-6 flex justify-end gap-2">
              <Button variant="outline" onClick={closeEdit}>
                {t("common.cancel")}
              </Button>
              <Button onClick={saveQuota} disabled={saving}>
                {saving && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                {t("profile.save")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
