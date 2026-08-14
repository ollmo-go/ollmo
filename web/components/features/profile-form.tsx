"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { ArrowLeft, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { useLocaleState, type Locale } from "@/lib/i18n";

// ProfileEditor is the core form body (tabs + info/password cards) shared
// by the page wrapper and the dialog wrapper. Width is unconstrained so
// the wrapper controls layout.
export function ProfileEditor({ onSaved }: { onSaved?: () => void }) {
  const t = useTranslations();
  const [tab, setTab] = useState<"info" | "password">("info");
  const { data: profile, mutate } = useSWR("profile", () => api.getProfile());
  const [name, setName] = useState("");
  // Language preference is persisted to the backend per user. The select
  // only stages the choice; it takes effect after Save.
  const [language, setLanguage] = useState<Locale | "">("");
  const { setLocale } = useLocaleState();
  const [savingName, setSavingName] = useState(false);

  const [oldPwd, setOldPwd] = useState("");
  const [newPwd, setNewPwd] = useState("");
  const [confirmPwd, setConfirmPwd] = useState("");
  const [savingPwd, setSavingPwd] = useState(false);

  useEffect(() => {
    if (profile) {
      setName(profile.name);
      setLanguage(profile.language === "en" || profile.language === "zh" ? profile.language : "");
    }
  }, [profile]);

  const savedLanguage = profile?.language === "en" || profile?.language === "zh" ? profile.language : "";
  const infoDirty = name !== (profile?.name ?? "") || language !== savedLanguage;

  async function handleSaveName() {
    if (!name.trim()) {
      toast.error(t("profile.name_required"));
      return;
    }
    setSavingName(true);
    try {
      await api.updateProfile(name.trim(), language);
      // Apply the saved language immediately after a successful save so
      // the UI reflects the stored preference.
      if (language === "en" || language === "zh") {
        setLocale(language);
      }
      await mutate();
      toast.success(t("profile.name_updated"));
      onSaved?.();
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setSavingName(false);
    }
  }

  async function handleChangePassword() {
    if (!oldPwd || !newPwd || !confirmPwd) {
      toast.error(t("profile.all_fields_required"));
      return;
    }
    if (newPwd.length < 6) {
      toast.error(t("profile.password_too_short"));
      return;
    }
    if (newPwd !== confirmPwd) {
      toast.error(t("profile.password_mismatch"));
      return;
    }
    setSavingPwd(true);
    try {
      await api.changePassword(oldPwd, newPwd);
      setOldPwd("");
      setNewPwd("");
      setConfirmPwd("");
      toast.success(t("profile.password_updated"));
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setSavingPwd(false);
    }
  }

  const tabs = [
    { key: "info" as const, label: t("profile.tab_info") },
    { key: "password" as const, label: t("profile.tab_password") },
  ];

  return (
    <div>
      <div className="flex gap-1 border-b mb-6">
        {tabs.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setTab(tb.key)}
            className={cn(
              "px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors",
              tab === tb.key
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {tb.label}
          </button>
        ))}
      </div>

      {tab === "info" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">{t("profile.section_info")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t("profile.email")}</Label>
              {/* Email is identity and cannot be changed. */}
              <Input value={profile?.email ?? ""} disabled autoComplete="email" />
            </div>
            <div className="space-y-2">
              <Label>{t("profile.name")}</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("profile.name")} autoComplete="name" />
            </div>
            <div className="space-y-2">
              <Label>{t("profile.language")}</Label>
              <select
                value={language}
                onChange={(e) => setLanguage(e.target.value as Locale | "")}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                <option value="">{t("profile.language_default")}</option>
                <option value="zh">中文</option>
                <option value="en">English</option>
              </select>
            </div>
            <Button onClick={handleSaveName} disabled={savingName || !infoDirty}>
              {savingName && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
              {t("profile.save")}
            </Button>
          </CardContent>
        </Card>
      )}

      {tab === "password" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">{t("profile.section_password")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label>{t("profile.old_password")}</Label>
              <Input type="password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} autoComplete="current-password" />
            </div>
            <div className="space-y-2">
              <Label>{t("profile.new_password")}</Label>
              <Input type="password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} autoComplete="new-password" />
            </div>
            <div className="space-y-2">
              <Label>{t("profile.confirm_password")}</Label>
              <Input type="password" value={confirmPwd} onChange={(e) => setConfirmPwd(e.target.value)} autoComplete="new-password" />
            </div>
            <Button onClick={handleChangePassword} disabled={savingPwd || !oldPwd || !newPwd || !confirmPwd}>
              {savingPwd && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
              {t("profile.change_password")}
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// ProfileForm is the page wrapper used by /dashboard/profile. It renders a
// header with a back arrow (backHref) plus the editor body.
export function ProfileForm({ backHref }: { backHref: string }) {
  const t = useTranslations();
  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link
          href={backHref}
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground shrink-0"
        >
          <ArrowLeft className="h-4 w-4 mr-1" /> {t("common.back")}
        </Link>
        <h1 className="text-2xl font-semibold tracking-tight truncate">{t("profile.title")}</h1>
      </div>
      <div className="max-w-lg">
        <ProfileEditor />
      </div>
    </div>
  );
}
