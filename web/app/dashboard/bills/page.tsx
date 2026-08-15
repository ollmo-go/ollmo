"use client";

import useSWR from "swr";
import { Sigma, FileInput, FileOutput, Coins, PhoneCall, Users, Cpu, History, type LucideIcon } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, BillOverview, UserBillTotal, ModelBillTotal, BillRecord } from "@/lib/api";
import { useTranslations } from "next-intl";
import { formatCost, formatTime, formatTokens } from "@/lib/utils";

// Color tokens for icon badges. Tailwind requires static class names, so we
// enumerate the variants used on this page.
const BADGE: Record<string, string> = {
  blue: "bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400",
  green: "bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400",
  purple: "bg-purple-50 text-purple-600 dark:bg-purple-950/50 dark:text-purple-400",
  amber: "bg-amber-50 text-amber-600 dark:bg-amber-950/50 dark:text-amber-400",
  cyan: "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/50 dark:text-cyan-400",
};

function Badge({
  icon: Icon,
  color,
  className,
}: {
  icon: LucideIcon;
  color: string;
  className?: string;
}) {
  return (
    <span
      className={`inline-flex h-9 w-9 items-center justify-center rounded-lg ${BADGE[color]} ${className ?? ""}`}
    >
      <Icon className="h-5 w-5" />
    </span>
  );
}

// Source label lookup: each LLM call carries a source that identifies which
// pipeline step produced it. Colors match the agent node palette.
const SOURCE: Record<string, { key: string; cls: string }> = {
  chat: { key: "bill.source_chat", cls: "bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400" },
  classifier: { key: "bill.source_classifier", cls: "bg-violet-50 text-violet-600 dark:bg-violet-950/50 dark:text-violet-400" },
  intermediate: { key: "bill.source_intermediate", cls: "bg-amber-50 text-amber-600 dark:bg-amber-950/50 dark:text-amber-400" },
  followups: { key: "bill.source_followups", cls: "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/50 dark:text-cyan-400" },
};

export default function BillsPage() {
  const t = useTranslations();
  const { data: overview } = useSWR<BillOverview>("bill-overview", () => api.billOverview());
  const { data: users } = useSWR<{ items: UserBillTotal[] }>("bill-users", () => api.billUsers());
  const { data: models } = useSWR<{ items: ModelBillTotal[] }>("bill-models", () => api.billModels());
  const { data: records } = useSWR<{ items: BillRecord[] }>("bill-records", () => api.billRecords(100));

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold">{t("bill.title")}</h1>
        <p className="text-sm text-muted-foreground mt-1">{t("bill.subtitle")}</p>
      </div>

      {/* Overview cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
        <StatCard icon={Sigma} color="blue" label={t("bill.total_tokens")} value={overview ? formatTokens(overview.total_tokens) : undefined} loading={!overview} />
        <StatCard icon={FileInput} color="cyan" label={t("bill.prompt_tokens")} value={overview ? formatTokens(overview.prompt_tokens) : undefined} loading={!overview} />
        <StatCard icon={FileOutput} color="purple" label={t("bill.completion_tokens")} value={overview ? formatTokens(overview.completion_tokens) : undefined} loading={!overview} />
        <StatCard icon={Coins} color="amber" label={t("bill.amount")} value={overview ? formatCost(overview.amount) : undefined} loading={!overview} />
        <StatCard icon={PhoneCall} color="green" label={t("bill.call_count")} value={overview ? formatTokens(overview.call_count) : undefined} loading={!overview} />
      </div>

      <p className="text-xs text-muted-foreground">{t("bill.estimate_hint")}</p>

      {/* Per-user and per-model aggregates */}
      <div className="grid md:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Badge icon={Users} color="green" />
              {t("bill.per_user")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {!users ? (
              <Skeleton className="h-20 w-full" />
            ) : users.items?.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("bill.empty")}</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-xs text-muted-foreground border-b">
                      <th className="pb-2 pr-4 font-medium">{t("bill.user")}</th>
                      <th className="pb-2 px-4 font-medium text-right">{t("bill.total_tokens")}</th>
                      <th className="pb-2 px-4 font-medium text-right">{t("bill.amount")}</th>
                      <th className="pb-2 pl-4 font-medium text-right">{t("bill.call_count")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.items?.map((u) => (
                      <tr key={u.user_id} className="border-b last:border-0 hover:bg-muted/40">
                        <td className="py-2 pr-4 truncate max-w-[180px]">
                          {u.user_name || t("bill.system_user")}
                        </td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatTokens(u.total_tokens)}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatCost(u.amount)}</td>
                        <td className="py-2 pl-4 text-right tabular-nums">{u.call_count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Badge icon={Cpu} color="purple" />
              {t("bill.per_model")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {!models ? (
              <Skeleton className="h-20 w-full" />
            ) : models.items?.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("bill.empty")}</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-xs text-muted-foreground border-b">
                      <th className="pb-2 pr-4 font-medium">{t("bill.model")}</th>
                      <th className="pb-2 px-4 font-medium text-right">{t("bill.total_tokens")}</th>
                      <th className="pb-2 px-4 font-medium text-right">{t("bill.amount")}</th>
                      <th className="pb-2 pl-4 font-medium text-right">{t("bill.call_count")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {models.items?.map((m) => (
                      <tr key={m.model_id || m.model_name} className="border-b last:border-0 hover:bg-muted/40">
                        <td className="py-2 pr-4 truncate max-w-[180px]">{m.model_name}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatTokens(m.total_tokens)}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatCost(m.amount)}</td>
                        <td className="py-2 pl-4 text-right tabular-nums">{m.call_count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Recent raw calls */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Badge icon={History} color="amber" />
            {t("bill.recent_records")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {!records ? (
            <Skeleton className="h-20 w-full" />
          ) : records.items?.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("bill.empty")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground border-b">
                    <th className="pb-2 pr-4 font-medium">{t("bill.time")}</th>
                    <th className="pb-2 px-4 font-medium">{t("bill.source")}</th>
                    <th className="pb-2 px-4 font-medium">{t("bill.model")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("bill.prompt_tokens")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("bill.completion_tokens")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("bill.amount")}</th>
                  </tr>
                </thead>
                <tbody>
                  {records.items?.map((r) => {
                    const src = SOURCE[r.source] ?? SOURCE.chat;
                    return (
                      <tr key={r.id} className="border-b last:border-0 hover:bg-muted/40">
                        <td className="py-2 pr-4 whitespace-nowrap text-xs text-muted-foreground">{formatTime(r.created_at)}</td>
                        <td className="py-2 px-4">
                          <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${src.cls}`}>
                            {t(src.key)}
                          </span>
                        </td>
                        <td className="py-2 px-4 truncate max-w-[200px]">{r.model_name}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatTokens(r.prompt_tokens)}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatTokens(r.completion_tokens)}</td>
                        <td className="py-2 px-4 text-right tabular-nums">{formatCost(r.amount)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function StatCard({
  icon: Icon,
  color,
  label,
  value,
  loading,
}: {
  icon: LucideIcon;
  color: string;
  label: string;
  value?: string;
  loading?: boolean;
}) {
  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4 flex flex-col gap-2">
        <Badge icon={Icon} color={color} />
        {loading ? (
          <Skeleton className="h-6 w-12" />
        ) : (
          <p className="text-xl font-semibold tabular-nums">{value ?? "0"}</p>
        )}
        <p className="text-xs text-muted-foreground">{label}</p>
      </CardContent>
    </Card>
  );
}
