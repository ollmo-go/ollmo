"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { toast } from "sonner";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { useTranslations } from "next-intl";

export default function SystemSettingsPage() {
  const t = useTranslations();
  const { data, mutate } = useSWR("site-settings-all", () => api.getAllSiteSettings());

  const [form, setForm] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (data) setForm(data);
  }, [data]);

  function set(key: string, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSave() {
    setSaving(true);
    try {
      await api.updateSiteSettings(form);
      await mutate();
      toast.success(t("system.saved"));
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    } finally {
      setSaving(false);
    }
  }

  if (!data) return null;

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link
          href="/dashboard"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4 mr-1" /> {t("common.back")}
        </Link>
        <h1 className="text-2xl font-semibold tracking-tight">{t("system.title")}</h1>
      </div>

      <div className="max-w-2xl space-y-6">
        {/* General settings */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">{t("system.title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t("system.site_name")}</Label>
              <Input
                value={form.site_name ?? ""}
                onChange={(e) => set("site_name", e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("system.site_description")}</Label>
              <Input
                value={form.site_description ?? ""}
                onChange={(e) => set("site_description", e.target.value)}
                placeholder={t("system.site_description_placeholder")}
              />
              <p className="text-xs text-muted-foreground">
                {t("system.site_description_hint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label>{t("system.default_language")}</Label>
              <select
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus:outline-none focus:ring-1 focus:ring-ring"
                value={form.default_language ?? "zh"}
                onChange={(e) => set("default_language", e.target.value)}
              >
                <option value="zh">{t("system.lang_zh")}</option>
                <option value="en">{t("system.lang_en")}</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label>{t("system.timezone")}</Label>
              <Input
                value={form.timezone ?? ""}
                onChange={(e) => set("timezone", e.target.value)}
                placeholder="Asia/Shanghai"
              />
            </div>
            <div className="flex items-center justify-between">
              <div>
                <Label>{t("system.allow_registration")}</Label>
                <p className="text-xs text-muted-foreground mt-1">
                  {t("system.allow_registration_desc")}
                </p>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={form.allow_registration === "true"}
                onClick={() =>
                  set("allow_registration", form.allow_registration === "true" ? "false" : "true")
                }
                className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${
                  form.allow_registration === "true"
                    ? "bg-primary"
                    : "bg-muted-foreground/30"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-background shadow transition-transform ${
                    form.allow_registration === "true" ? "translate-x-4" : "translate-x-0.5"
                  }`}
                />
              </button>
            </div>
            <div className="flex items-center justify-between">
              <div>
                <Label>{t("system.auto_memory")}</Label>
                <p className="text-xs text-muted-foreground mt-1">
                  {t("system.auto_memory_desc")}
                </p>
              </div>
              {/* Missing setting row means enabled (backend default is on). */}
              <button
                type="button"
                role="switch"
                aria-checked={form.auto_memory !== "false"}
                onClick={() =>
                  set("auto_memory", form.auto_memory !== "false" ? "false" : "true")
                }
                className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${
                  form.auto_memory !== "false"
                    ? "bg-primary"
                    : "bg-muted-foreground/30"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-background shadow transition-transform ${
                    form.auto_memory !== "false" ? "translate-x-4" : "translate-x-0.5"
                  }`}
                />
              </button>
            </div>
          </CardContent>
        </Card>

        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
            {t("profile.save")}
          </Button>
        </div>
      </div>
    </div>
  );
}
