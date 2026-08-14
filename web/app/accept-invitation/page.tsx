"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { CheckCircle2 } from "lucide-react";
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
import { api, ApiError, InvitationView } from "@/lib/api";
import { useTranslations } from "next-intl";

function AcceptInvitationInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get("token") || "";
  const t = useTranslations();

  const [phase, setPhase] = useState<"loading" | "form" | "success" | "error">("loading");
  const [errorMsg, setErrorMsg] = useState("");
  const [invitation, setInvitation] = useState<InvitationView | null>(null);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // Peek the invitation by token on mount. If it is missing, expired, or
  // already accepted, surface an error state with a link back to login.
  useEffect(() => {
    if (!token) {
      setErrorMsg(t("invitation.invalid"));
      setPhase("error");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const inv = await api.peekInvitation(token);
        if (cancelled) return;
        // Treat a non-pending invitation as already accepted/invalid.
        if (inv.status && inv.status !== "pending") {
          setErrorMsg(t("invitation.invalid"));
          setPhase("error");
          return;
        }
        setInvitation(inv);
        // Pre-fill the name with the invited email.
        setName(inv.email || "");
        setPhase("form");
      } catch (e) {
        if (cancelled) return;
        const status = e instanceof ApiError ? e.status : 0;
        // 410 typically means expired; everything else is treated as invalid.
        setErrorMsg(status === 410 ? t("invitation.expired") : t("invitation.invalid"));
        setPhase("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, t]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    try {
      await api.acceptInvitation(token, name, password);
      setPhase("success");
      // Redirect to login after a short delay so the user sees the success msg.
      setTimeout(() => router.push("/login"), 2000);
    } catch (e) {
      const status = e instanceof ApiError ? e.status : 0;
      setErrorMsg(status === 410 ? t("invitation.expired") : t("invitation.invalid"));
      setPhase("error");
    } finally {
      setSubmitting(false);
    }
  }

  const roleLabel = invitation ? t(`team.${invitation.role}`) : "";

  // Error state: missing / expired / already accepted invitation.
  if (phase === "error") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="w-[400px]">
          <CardHeader>
            <CardTitle className="text-xl">{t("invitation.accept")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-sm text-destructive rounded-md bg-destructive/10 px-3 py-2">
              {errorMsg}
            </div>
          </CardContent>
          <CardFooter>
            <Link href="/login" className="w-full">
              <Button className="w-full">{t("invitation.go_login")}</Button>
            </Link>
          </CardFooter>
        </Card>
      </div>
    );
  }

  // Success state: account created, redirecting to login.
  if (phase === "success") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="w-[400px]">
          <CardContent className="pt-6 flex flex-col items-center text-center gap-3">
            <CheckCircle2 className="h-12 w-12 text-emerald-500" />
            <h2 className="text-xl font-semibold">{t("invitation.success")}</h2>
            <p className="text-sm text-muted-foreground">{t("invitation.success_msg")}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Loading state.
  if (phase === "loading" || !invitation) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Card className="w-[400px]">
          <CardContent className="pt-6 text-center text-sm text-muted-foreground">
            {t("invitation.loading")}
          </CardContent>
        </Card>
      </div>
    );
  }

  // Form state: collect name + password and accept the invitation.
  return (
    <div className="min-h-screen flex items-center justify-center">
      <Card className="w-[400px]">
        <CardHeader>
          <CardTitle className="text-xl">{t("invitation.accept")}</CardTitle>
          <CardDescription>
            {invitation.tenant_name
              ? `${t("invitation.title")} · ${invitation.tenant_name}`
              : t("invitation.title")}
          </CardDescription>
        </CardHeader>
        <form onSubmit={handleSubmit}>
          <CardContent className="space-y-4">
            <div className="space-y-1 text-sm">
              <p className="text-muted-foreground">
                <span className="font-medium text-foreground">{invitation.email}</span>
              </p>
              <p className="text-muted-foreground">
                {t("invitation.invited_as")}:{" "}
                <span className="font-medium text-foreground">{roleLabel}</span>
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="name">{t("invitation.name")}</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("invitation.password")}</Label>
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
            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? t("invitation.submitting") : t("invitation.submit")}
            </Button>
            <p className="text-sm text-muted-foreground">
              <Link href="/login" className="underline">
                {t("invitation.go_login")}
              </Link>
            </p>
          </CardFooter>
        </form>
      </Card>
    </div>
  );
}

export default function AcceptInvitationPage() {
  // useSearchParams must be wrapped in a Suspense boundary for static
  // rendering in Next.js 15.
  return (
    <Suspense
      fallback={
        <div className="min-h-screen flex items-center justify-center">
          <Card className="w-[400px]">
            <CardContent className="pt-6 text-center text-sm text-muted-foreground">
              ...
            </CardContent>
          </Card>
        </div>
      }
    >
      <AcceptInvitationInner />
    </Suspense>
  );
}
