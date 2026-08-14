"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api, ApiError } from "@/lib/api";
import { isAuthenticated, useAuthStore } from "@/lib/auth";
import { useSiteSettings } from "@/lib/use-site-settings";
import { useTranslations } from "next-intl";
import { Logo } from "@/components/brand/logo";
import { LanguageToggle } from "@/components/features/language-toggle";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const t = useTranslations();
  const { settings } = useSiteSettings();
  // Registration is allowed only when the setting is explicitly "true",
  // matching the backend check. Missing/other values hide the entry.
  const allowRegistration = settings?.allow_registration === "true";

  // Already logged in? Go straight to the app.
  useEffect(() => {
    if (isAuthenticated()) {
      router.replace("/");
    }
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const res = await api.login({ email, password });
      useAuthStore.getState().setAuth(res.token, res.tenant_id, res.user_id);
      router.push("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("auth.login_failed"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center">
      <LanguageToggle className="fixed top-4 right-4" />
      <div className="w-[400px]">
        <div className="flex items-center justify-center mb-6">
          <Link href="/">
            <Logo />
          </Link>
        </div>
        <Card>
        <CardHeader>
          <CardTitle className="text-xl">{t("auth.login")}</CardTitle>
          <CardDescription>{t("auth.login_desc")}</CardDescription>
        </CardHeader>
        <form onSubmit={handleSubmit}>
          <CardContent className="space-y-4">
            {error && (
              <div className="text-sm text-destructive rounded-md bg-destructive/10 px-3 py-2">
                {error}
              </div>
            )}
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
              />
            </div>
          </CardContent>
          <CardFooter className="flex flex-col gap-3">
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? t("auth.signing_in") : t("auth.sign_in")}
            </Button>
            {allowRegistration && (
              <p className="text-sm text-muted-foreground">
                {t("auth.no_account_short")}{" "}
                <Link href="/register" className="underline">
                  {t("auth.register")}
                </Link>
              </p>
            )}
          </CardFooter>
        </form>
      </Card>
      </div>
    </div>
  );
}
