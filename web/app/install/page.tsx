"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api, ApiError, InstallDefaults } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";
import { useTranslations } from "next-intl";
import { Logo } from "@/components/brand/logo";

export default function InstallPage() {
  const router = useRouter();
  const t = useTranslations();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [tenantName, setTenantName] = useState("");
  const [providerKey, setProviderKey] = useState("");
  const [defaults, setDefaults] = useState<InstallDefaults | null>(null);
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [error, setError] = useState("");
  const [optionalExpanded, setOptionalExpanded] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const [status, def] = await Promise.all([
          api.installStatus(),
          api.installDefaults(),
        ]);
        if (status.installed) {
          router.replace("/login");
          return;
        }
        setDefaults(def);
      } catch {
        // API not reachable; let the user see the form anyway.
      } finally {
        setChecking(false);
      }
    })();
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const res = await api.install({
        email,
        password,
        name,
        tenant_name: tenantName,
        provider_key: providerKey,
        provider_name: "SiliconFlow",
      });
      useAuthStore.getState().setAuth(res.token, res.tenant_id, res.user_id);
      router.push("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("install.failed"));
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center py-8">
      <div className="w-[480px]">
        <div className="flex items-center justify-center gap-2 mb-6">
          <Logo size="h-8 w-8" nameClass="text-2xl font-bold leading-tight" />
        </div>
        <Card>
        <CardHeader>
          <CardTitle className="text-xl">{t("install.title")}</CardTitle>
          <CardDescription>{t("install.description")}</CardDescription>
        </CardHeader>
        <form onSubmit={handleSubmit}>
          <CardContent className="space-y-6">
            {error && (
              <div className="text-sm text-destructive rounded-md bg-destructive/10 px-3 py-2">
                {error}
              </div>
            )}

            {/* Admin account */}
            <div className="space-y-3">
              <h3 className="text-sm font-medium text-muted-foreground">
                {t("install.admin_section")}
              </h3>
              <div className="space-y-2">
                <Label htmlFor="email">{t("auth.email")}</Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">{t("auth.password")}</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                />
              </div>
              <button
                type="button"
                onClick={() => setOptionalExpanded(!optionalExpanded)}
                className="flex items-center gap-1 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
              >
                <ChevronDown
                  className={`h-4 w-4 transition-transform ${optionalExpanded ? "" : "-rotate-90"}`}
                />
                {t("install.optional_section")}
              </button>

              {optionalExpanded && (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-2">
                      <Label htmlFor="name">{t("auth.name")}</Label>
                      <Input
                        id="name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        placeholder={t("install.name_placeholder")}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="tenant">{t("auth.tenant_name")}</Label>
                      <Input
                        id="tenant"
                        value={tenantName}
                        onChange={(e) => setTenantName(e.target.value)}
                        placeholder={t("install.tenant_placeholder")}
                      />
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="apikey">{t("install.api_key")}</Label>
                    <Input
                      id="apikey"
                      type="password"
                      value={providerKey}
                      onChange={(e) => setProviderKey(e.target.value)}
                      placeholder="sk-..."
                    />
                    <p className="text-xs text-muted-foreground">
                      {t("install.api_key_hint")}
                    </p>
                    <a
                      href={t("install.api_key_link_url")}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-xs text-blue-600 hover:underline"
                    >
                      {t("install.api_key_link")}
                    </a>
                  </div>

                  {defaults && (
                    <div className="rounded-md border bg-muted/30 p-3 space-y-2 text-xs">
                      <div className="font-medium text-muted-foreground">
                        {t("install.defaults_preview")}
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">LLM</span>
                        <span className="font-mono">{defaults.llm.model}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Embedding</span>
                        <span className="font-mono">{defaults.embedding.model}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Rerank</span>
                        <span className="font-mono">{defaults.rerank.model}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-muted-foreground">Endpoint</span>
                        <span className="font-mono">{defaults.llm.endpoint}</span>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </CardContent>

          <div className="px-6 pb-6">
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? t("install.installing") : t("install.submit")}
            </Button>
          </div>
        </form>
      </Card>
      </div>
    </div>
  );
}
