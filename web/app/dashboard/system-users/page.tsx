"use client";

import Link from "next/link";
import useSWR from "swr";
import { toast } from "sonner";
import { ArrowLeft } from "lucide-react";
import { useTranslations } from "next-intl";
import { api, type SystemUser } from "@/lib/api";
import { useAuthStore } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useConfirm } from "@/components/ui/confirm";
import { formatTime } from "@/lib/utils";

export default function SystemUsersPage() {
  const t = useTranslations();
  const confirm = useConfirm();
  const userId = useAuthStore((s) => s.userId);
  const { data, mutate } = useSWR<{ items: SystemUser[] }>("system-users", () =>
    api.listSystemUsers()
  );

  async function toggleSuperAdmin(u: SystemUser) {
    const next = !u.is_super_admin;
    const ok = await confirm({
      title: next ? t("users.grant_super") : t("users.revoke_super"),
      description: next
        ? t("users.grant_super_desc", { name: u.name || u.email })
        : t("users.revoke_super_desc", { name: u.name || u.email }),
      destructive: !next,
      confirmText: next ? t("users.grant_super") : t("users.revoke_super"),
    });
    if (!ok) return;
    try {
      await api.updateUserSuperAdmin(u.id, next);
      mutate(
        (page) =>
          page
            ? {
                ...page,
                items: page.items.map((x) =>
                  x.id === u.id ? { ...x, is_super_admin: next } : x
                ),
              }
            : page,
        false
      );
      toast.success(t("users.super_updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    }
  }

  const users = data?.items ?? [];

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link
          href="/dashboard"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4 mr-1" /> {t("common.back")}
        </Link>
        <h1 className="text-2xl font-semibold tracking-tight">{t("users.title")}</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("users.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground border-b">
                  <th className="py-2 pr-4">{t("team.name")}</th>
                  <th className="py-2 pr-4">{t("team.email")}</th>
                  <th className="py-2 pr-4">{t("users.tenant")}</th>
                  <th className="py-2 pr-4">{t("team.role")}</th>
                  <th className="py-2 pr-4">{t("team.status")}</th>
                  <th className="py-2 pr-4">{t("users.super_admin")}</th>
                  <th className="py-2 pr-4">{t("kb.created")}</th>
                  <th className="py-2 pr-4">{t("team.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {!data &&
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b last:border-0">
                      {Array.from({ length: 8 }).map((_, j) => (
                        <td key={j} className="py-2 pr-4">
                          <Skeleton className="h-4 w-20" />
                        </td>
                      ))}
                    </tr>
                  ))}
                {users.map((u) => {
                  const isSelf = u.id === userId;
                  return (
                    <tr key={u.id} className="border-b last:border-0">
                      <td className="py-2 pr-4 font-medium">{u.name || "—"}</td>
                      <td className="py-2 pr-4 text-muted-foreground">{u.email}</td>
                      <td className="py-2 pr-4 text-muted-foreground">{u.tenant_name || "—"}</td>
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
                        {u.is_super_admin ? (
                          <span className="px-2 py-0.5 rounded text-xs bg-amber-100 text-amber-700">
                            {t("users.super_admin")}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="py-2 pr-4 text-muted-foreground">{formatTime(u.created_at)}</td>
                      <td className="py-2 pr-4">
                        <Button
                          size="sm"
                          variant={u.is_super_admin ? "outline" : "default"}
                          disabled={isSelf}
                          onClick={() => toggleSuperAdmin(u)}
                          title={isSelf ? t("team.cannot_change_self") : undefined}
                        >
                          {u.is_super_admin ? t("users.revoke_super") : t("users.grant_super")}
                        </Button>
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
    </div>
  );
}
