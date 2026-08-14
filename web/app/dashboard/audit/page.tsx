"use client";

import { useState } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api, AuditLog, TeamUser, Paginated } from "@/lib/api";
import { useTranslations } from "next-intl";
import { formatTime } from "@/lib/utils";

export default function AuditPage() {
  const t = useTranslations();
  const [page, setPage] = useState(1);
  const [userId, setUserId] = useState("");
  const [action, setAction] = useState("");

  const { data: users } = useSWR<{ items: TeamUser[] }>("team-users", () => api.listTeamUsers());
  const { data, isLoading } = useSWR<Paginated<AuditLog>>(
    ["audit-logs", page, userId, action],
    () => api.listAuditLogs(page, 50, { user_id: userId || undefined, action: action || undefined })
  );

  const actions = Array.from(new Set(data?.items?.map((l) => l?.action?.split(".")[0]).filter(Boolean) || []));

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">{t("audit.title")}</h1>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <select
          className="h-9 rounded-md border border-input bg-background px-2 text-sm"
          value={userId}
          onChange={(e) => { setUserId(e.target.value); setPage(1); }}
        >
          <option value="">{t("audit.all_users")}</option>
          {users?.items?.map((u) => (
            <option key={u.id} value={u.id}>{u.name || u.email}</option>
          ))}
        </select>
        <select
          className="h-9 rounded-md border border-input bg-background px-2 text-sm"
          value={action}
          onChange={(e) => { setAction(e.target.value); setPage(1); }}
        >
          <option value="">{t("audit.all_actions")}</option>
          {actions.map((a) => (
            <option key={a} value={a}>{a}</option>
          ))}
        </select>
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-4 space-y-2">
              {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-8 w-full" />)}
            </div>
          ) : data?.items?.length === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">{t("audit.empty")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground border-b">
                    <th className="px-4 py-2 font-medium">{t("audit.user")}</th>
                    <th className="px-4 py-2 font-medium">{t("audit.action")}</th>
                    <th className="px-4 py-2 font-medium">{t("audit.resource")}</th>
                    <th className="px-4 py-2 font-medium">{t("audit.detail")}</th>
                    <th className="px-4 py-2 font-medium">{t("audit.ip")}</th>
                    <th className="px-4 py-2 font-medium">{t("audit.time")}</th>
                  </tr>
                </thead>
                <tbody>
                  {data?.items?.map((log) => {
                    const user = users?.items?.find((u) => u.id === log.user_id);
                    return (
                      <tr key={log.id} className="border-b last:border-0">
                        <td className="px-4 py-2 whitespace-nowrap">{user?.name || log.user_name || log.user_id?.slice(0, 8) || "-"}</td>
                        <td className="px-4 py-2">
                          <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{log.action}</code>
                        </td>
                        <td className="px-4 py-2 font-mono text-xs text-muted-foreground">{log.resource?.slice(0, 8) || "-"}</td>
                        <td className="px-4 py-2 text-xs text-muted-foreground truncate max-w-[200px]">{log.detail}</td>
                        <td className="px-4 py-2 text-xs text-muted-foreground">{log.ip}</td>
                        <td className="px-4 py-2 text-xs text-muted-foreground whitespace-nowrap">{formatTime(log.created_at)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Pagination */}
      {data && data.total > 50 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            {Math.min((page - 1) * 50 + 1, data.total)}–{Math.min(page * 50, data.total)} / {data.total}
          </span>
          <div className="flex gap-2">
            <button
              className="px-3 py-1 text-sm rounded-md border disabled:opacity-50"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              ‹
            </button>
            <button
              className="px-3 py-1 text-sm rounded-md border disabled:opacity-50"
              disabled={page * 50 >= data.total}
              onClick={() => setPage((p) => p + 1)}
            >
              ›
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
