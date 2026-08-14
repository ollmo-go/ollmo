"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { toast } from "sonner";
import { ArrowLeft, Ban, Check, Copy, Loader2, UserPlus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { api, Invitation, TeamUser } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";
import { formatTime } from "@/lib/utils";

// Shared SWR fetcher for the team page. Dispatches by key so both the users
// list and the pending invitations list can reuse the same function.
async function fetcher<T>(key: string): Promise<T> {
  if (key === "team-users") return (await api.listTeamUsers()) as unknown as T;
  if (key === "team-invitations") return (await api.listInvitations()) as unknown as T;
  throw new Error(`unknown fetcher key: ${key}`);
}

export default function TeamPage() {
  const t = useTranslations();
  const confirm = useConfirm();
  const userId = useAuthStore((s) => s.userId);

  const { data, mutate } = useSWR<{ items: TeamUser[] }>("team-users", fetcher);
  const { data: invData, mutate: invMutate } = useSWR<{ items: Invitation[] }>(
    "team-invitations",
    fetcher
  );
  const { data: quota, mutate: mutateQuota } = useSWR("tenant-quota", () => api.getQuota());

  // Team name editing
  const [teamName, setTeamName] = useState("");
  const [savingName, setSavingName] = useState(false);
  useEffect(() => {
    if (quota) setTeamName(quota.name);
  }, [quota]);
  async function handleSaveTeamName() {
    if (!teamName.trim()) {
      toast.error(t("team.name_required"));
      return;
    }
    setSavingName(true);
    try {
      await api.updateTenantName(teamName.trim());
      await mutateQuota();
      toast.success(t("team.name_updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutateQuota();
    } finally {
      setSavingName(false);
    }
  }

  // Invite dialog state. Two phases: "form" collects email + role, "link"
  // shows the generated invitation link with a copy button after creation.
  const [dialogOpen, setDialogOpen] = useState(false);
  const [phase, setPhase] = useState<"form" | "link">("form");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<"admin" | "member">("member");
  const [creating, setCreating] = useState(false);
  const [inviteToken, setInviteToken] = useState("");
  const [copied, setCopied] = useState(false);

  // Reset password dialog state
  const [resetTarget, setResetTarget] = useState<TeamUser | null>(null);
  const [resetPwd, setResetPwd] = useState("");
  const [resetting, setResetting] = useState(false);

  function openReset(u: TeamUser) {
    setResetTarget(u);
    setResetPwd("");
    setResetting(false);
  }
  function closeReset() {
    setResetTarget(null);
    setResetPwd("");
  }
  async function doReset() {
    if (!resetTarget) return;
    if (resetPwd.length < 6) {
      toast.error(t("profile.password_too_short"));
      return;
    }
    setResetting(true);
    try {
      await api.resetUserPassword(resetTarget.id, resetPwd);
      toast.success(t("team.reset_password_done"));
      closeReset();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setResetting(false);
    }
  }

  function openInvite() {
    setPhase("form");
    setEmail("");
    setRole("member");
    setInviteToken("");
    setCopied(false);
    setDialogOpen(true);
  }

  function closeInvite() {
    setDialogOpen(false);
    setInviteToken("");
  }

  async function sendInvite() {
    if (!email.trim()) {
      toast.error(t("team.invite_email"));
      return;
    }
    setCreating(true);
    try {
      const res = await api.createInvitation(email.trim(), role);
      setInviteToken(res.token);
      setPhase("link");
      invMutate();
      toast.success(t("team.invite_created"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setCreating(false);
    }
  }

  function inviteLink(token: string): string {
    // Build the public acceptance URL from the current origin so it works in
    // any deployment (defaults to http://localhost:3001 in dev).
    return `${window.location.origin}/accept-invitation?token=${token}`;
  }

  function copyLink() {
    if (!inviteToken) return;
    navigator.clipboard.writeText(inviteLink(inviteToken));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  async function changeRole(u: TeamUser, newRole: "admin" | "member") {
    if (newRole === u.role) return;
    try {
      await api.updateUserRole(u.id, newRole);
      mutate(
        (page) =>
          page
            ? {
                ...page,
                items: page.items.map((x) => (x.id === u.id ? { ...x, role: newRole } : x)),
              }
            : page,
        false
      );
      toast.success(t("toast.role_updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    }
  }

  async function toggleStatus(u: TeamUser) {
    const next = u.status === "active" ? "disabled" : "active";
    try {
      await api.updateUserStatus(u.id, next);
      mutate(
        (page) =>
          page
            ? {
                ...page,
                items: page.items.map((x) => (x.id === u.id ? { ...x, status: next } : x)),
              }
            : page,
        false
      );
      toast.success(t("team.status_updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    }
  }

  async function cancelInvite(inv: Invitation) {
    const ok = await confirm({
      title: t("team.cancel_invite") + "?",
      description: inv.email,
      destructive: true,
      confirmText: t("team.cancel_invite"),
    });
    if (!ok) return;
    try {
      await api.cancelInvitation(inv.id);
      invMutate();
      toast.success(t("team.invite_cancelled"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  const users = data?.items ?? [];
  const pending = (invData?.items ?? []).filter((i) => i.status === "pending");

  return (
    <div>
      {/* Return button on the left, title, and invite button on the right */}
      <div className="flex items-center justify-between mb-6 gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <Link
            href="/dashboard"
            className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground shrink-0"
          >
            <ArrowLeft className="h-4 w-4 mr-1" /> {t("common.back")}
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight truncate">{t("nav.my_tenant")}</h1>
        </div>
        <Button onClick={openInvite} className="shrink-0">
          <UserPlus className="h-4 w-4 mr-1" /> {t("team.invite_user")}
        </Button>
      </div>

      {/* Team info card */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-lg">{t("team.team_info")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-end gap-3">
            <div className="space-y-2 flex-1 max-w-xs">
              <Label>{t("team.team_name")}</Label>
              <Input value={teamName} onChange={(e) => setTeamName(e.target.value)} />
            </div>
            <Button
              onClick={handleSaveTeamName}
              disabled={savingName || teamName === quota?.name}
            >
              {savingName && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
              {t("profile.save")}
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Label className="text-sm text-muted-foreground shrink-0">{t("team.plan")}</Label>
            <span className="inline-flex items-center rounded-md bg-muted px-2 py-1 text-xs font-medium">
              {quota?.plan === "pro"
                ? t("team.plan_pro")
                : quota?.plan === "enterprise"
                  ? t("team.plan_enterprise")
                  : t("team.plan_free")}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-1 text-sm text-muted-foreground">
            <span className="shrink-0">{t("team.quota_summary")}</span>
            <span>
              {t("settings.quota_documents")}: {quota?.doc.used ?? "-"}/
              {quota?.doc.limit === -1 ? "∞" : quota?.doc.limit ?? "-"} {t("settings.quota_unit_docs")}
            </span>
            <span>
              {t("team.quota_team_messages")}: {quota?.message.used ?? "-"}/
              {quota?.message.limit === -1 ? "∞" : quota?.message.limit ?? "-"} {t("settings.quota_unit_messages")}
            </span>
            <span>
              {t("team.quota_user_messages")}:{" "}
              {quota?.user_message_limit === -1 ? "∞" : quota?.user_message_limit ?? "-"} {t("settings.quota_unit_messages")}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-lg">{t("team.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground border-b">
                  <th className="py-2 pr-4">{t("team.name")}</th>
                  <th className="py-2 pr-4">{t("team.email")}</th>
                  <th className="py-2 pr-4">{t("team.role")}</th>
                  <th className="py-2 pr-4">{t("team.status")}</th>
                  <th className="py-2 pr-4">{t("team.usage")}</th>
                  <th className="py-2 pr-4">{t("kb.created")}</th>
                  <th className="py-2 pr-4">{t("team.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {!data &&
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b last:border-0">
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-40" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-28" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                    </tr>
                  ))}
                {users.map((u) => {
                  const isSelf = u.id === userId;
                  return (
                    <tr key={u.id} className="border-b last:border-0">
                      <td className="py-2 pr-4 font-medium">{u.name || "—"}</td>
                      <td className="py-2 pr-4 text-muted-foreground">{u.email}</td>
                      <td className="py-2 pr-4">
                        <span
                          className={`px-2 py-0.5 rounded text-xs ${
                            u.role === "admin"
                              ? "bg-blue-100 text-blue-700"
                              : "bg-muted text-muted-foreground"
                          }`}
                        >
                          {t(`team.${u.role}`)}
                        </span>
                      </td>
                      <td className="py-2 pr-4">
                        <span
                          className={`px-2 py-0.5 rounded text-xs ${
                            u.status === "active"
                              ? "bg-emerald-100 text-emerald-700"
                              : "bg-red-100 text-red-700"
                          }`}
                        >
                          {t(`team.${u.status}`)}
                        </span>
                      </td>
                      <td className="py-2 pr-4">
                        <span className="text-muted-foreground">
                          {u.message_used}/
                          {quota?.user_message_limit === -1 ? "∞" : quota?.user_message_limit ?? "-"}
                        </span>
                      </td>
                      <td className="py-2 pr-4 text-muted-foreground">{formatTime(u.created_at)}</td>
                      <td className="py-2 pr-4">
                        <div className="flex items-center gap-2">
                          {/* Role selector — disabled for the current user */}
                          <select
                            value={u.role}
                            disabled={isSelf}
                            onChange={(e) => changeRole(u, e.target.value as "admin" | "member")}
                            title={isSelf ? t("team.cannot_change_self") : t("team.change_role")}
                            className="h-8 rounded-md border border-input bg-background px-2 text-xs disabled:opacity-50 disabled:cursor-not-allowed"
                          >
                            <option value="admin">{t("team.admin")}</option>
                            <option value="member">{t("team.member")}</option>
                          </select>
                          {/* Activate / disable toggle — disabled for the current user */}
                          <Button
                            size="sm"
                            variant={u.status === "active" ? "outline" : "default"}
                            disabled={isSelf}
                            onClick={() => toggleStatus(u)}
                            title={isSelf ? t("team.cannot_change_self") : undefined}
                          >
                            {u.status === "active" ? t("team.disable") : t("team.activate")}
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={isSelf}
                            onClick={() => openReset(u)}
                            title={isSelf ? t("team.cannot_change_self") : undefined}
                          >
                            {t("team.reset_password")}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {data && users.length === 0 && (
            <p className="text-sm text-muted-foreground py-4">{t("team.empty")}</p>
          )}
        </CardContent>
      </Card>

      {/* Pending invitations */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("team.pending_invitations")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground border-b">
                  <th className="py-2 pr-4">{t("team.email")}</th>
                  <th className="py-2 pr-4">{t("team.role")}</th>
                  <th className="py-2 pr-4">{t("kb.created")}</th>
                  <th className="py-2 pr-4">{t("team.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {pending.map((inv) => (
                  <tr key={inv.id} className="border-b last:border-0">
                    <td className="py-2 pr-4 text-muted-foreground">{inv.email}</td>
                    <td className="py-2 pr-4">
                      <span
                        className={`px-2 py-0.5 rounded text-xs ${
                          inv.role === "admin"
                            ? "bg-blue-100 text-blue-700"
                            : "bg-muted text-muted-foreground"
                        }`}
                      >
                        {t(`team.${inv.role}`)}
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">{formatTime(inv.created_at)}</td>
                    <td className="py-2 pr-4">
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive hover:text-destructive"
                        onClick={() => cancelInvite(inv)}
                      >
                        <Ban className="h-3.5 w-3.5 mr-1" /> {t("team.cancel_invite")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {pending.length === 0 && (
            <p className="text-sm text-muted-foreground py-4">{t("team.no_pending")}</p>
          )}
        </CardContent>
      </Card>

      {/* Invite dialog */}
      {dialogOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/40"
            onClick={closeInvite}
            aria-hidden="true"
          />
          <div className="relative w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
            <button
              onClick={closeInvite}
              className="absolute right-3 top-3 text-muted-foreground hover:text-foreground"
              aria-label={t("common.close")}
            >
              <X className="h-4 w-4" />
            </button>
            {phase === "form" ? (
              <>
                <h3 className="text-lg font-semibold mb-4">{t("team.invite_user")}</h3>
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label>{t("team.invite_email")}</Label>
                    <Input
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="name@example.com"
                      autoFocus
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>{t("team.invite_role")}</Label>
                    <select
                      value={role}
                      onChange={(e) => setRole(e.target.value as "admin" | "member")}
                      className="w-full h-10 rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="member">{t("team.member")}</option>
                      <option value="admin">{t("team.admin")}</option>
                    </select>
                  </div>
                </div>
                <div className="mt-6 flex justify-end gap-2">
                  <Button variant="outline" onClick={closeInvite}>
                    {t("common.cancel")}
                  </Button>
                  <Button onClick={sendInvite} disabled={creating}>
                    {creating ? "..." : t("team.invite_send")}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <h3 className="text-lg font-semibold mb-2">{t("team.invite_link")}</h3>
                <p className="text-sm text-muted-foreground mb-4">
                  {t("team.invite_created")}
                </p>
                <div className="space-y-1">
                  <Label>{t("team.invite_link")}</Label>
                  <div className="flex gap-2">
                    <Input
                      readOnly
                      value={inviteToken ? inviteLink(inviteToken) : ""}
                      className="font-mono text-xs"
                      onFocus={(e) => e.target.select()}
                    />
                    <Button variant="outline" onClick={copyLink} className="shrink-0">
                      {copied ? (
                        <><Check className="h-4 w-4 mr-1" /> {t("team.invite_link_copied")}</>
                      ) : (
                        <><Copy className="h-4 w-4 mr-1" /> {t("settings.apiKeys.copy")}</>
                      )}
                    </Button>
                  </div>
                </div>
                <div className="mt-6 flex justify-end">
                  <Button onClick={closeInvite}>{t("common.close")}</Button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {/* Reset password dialog */}
      {resetTarget && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={closeReset} aria-hidden="true" />
          <div className="relative w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
            <button
              onClick={closeReset}
              className="absolute right-3 top-3 text-muted-foreground hover:text-foreground"
              aria-label={t("common.close")}
            >
              <X className="h-4 w-4" />
            </button>
            <h3 className="text-lg font-semibold mb-2">{t("team.reset_password")}</h3>
            <p className="text-sm text-muted-foreground mb-4">
              {t("team.reset_password_hint", { name: resetTarget.name || resetTarget.email })}
            </p>
            <div className="space-y-1">
              <Label>{t("team.new_password")}</Label>
              <Input
                type="password"
                value={resetPwd}
                onChange={(e) => setResetPwd(e.target.value)}
                autoFocus
              />
            </div>
            <div className="mt-6 flex justify-end gap-2">
              <Button variant="outline" onClick={closeReset}>{t("common.cancel")}</Button>
              <Button onClick={doReset} disabled={resetting || resetPwd.length < 6}>
                {resetting ? "..." : t("team.reset_password_confirm")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

