"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { ArrowLeft, BarChart3, KeyRound, Loader2, User } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { useLocaleState, type Locale } from "@/lib/i18n";
import { MyUsage } from "@/components/features/my-usage";

type SectionKey = "info" | "password" | "usage";

// ProfilePanel is the shared personal-center body: a vertical section nav
// (basic info / password / usage) with the matching content on the right.
// Used by both the chat profile dialog and /dashboard/profile.
export function ProfilePanel({ onSaved }: { onSaved?: () => void }) {
  const t = useTranslations();
  const [section, setSection] = useState<SectionKey>("info");
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

  const sections: { key: SectionKey; label: string; icon: typeof User }[] = [
    { key: "info", label: t("profile.tab_info"), icon: User },
    { key: "password", label: t("profile.tab_password"), icon: KeyRound },
    { key: "usage", label: t("profile.tab_usage"), icon: BarChart3 },
  ];

  return (
    <div className="flex flex-1 min-h-0">
      <nav className="w-36 shrink-0 border-r p-2 space-y-0.5">
        {sections.map((s) => {
          const Icon = s.icon;
          const active = section === s.key;
          return (
            <button
              key={s.key}
              onClick={() => setSection(s.key)}
              className={cn(
                "flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors",
                active
                  ? "bg-accent text-primary font-medium"
                  : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {s.label}
            </button>
          );
        })}
      </nav>
      <div className="flex-1 min-w-0 p-6 pt-4 overflow-y-auto">
        {section === "info" && (
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

        {section === "password" && (
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

        {section === "usage" && <MyUsage />}
      </div>
    </div>
  );
}

// ProfileForm is the page wrapper used by /dashboard/profile. It renders a
// header with a back arrow (backHref) plus the shared ProfilePanel body.
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
      <div className="max-w-2xl rounded-lg border bg-background">
        <ProfilePanel />
      </div>
    </div>
  );
}
