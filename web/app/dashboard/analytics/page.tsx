"use client";

import useSWR from "swr";
import {
  FileText,
  MessageSquare,
  BookOpen,
  Database,
  HardDrive,
  Layers,
  Activity,
  History,
  ThumbsDown,
  ThumbsUp,
  type LucideIcon,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { api, AnalyticsOverview, AnalyticsDocStats, KBUsage, ActivityItem, FeedbackItem } from "@/lib/api";
import { useTranslations } from "next-intl";
import { formatSize, formatTime } from "@/lib/utils";
import { useRouter } from "next/navigation";

// Color tokens for icon badges. Tailwind requires static class names, so we
// enumerate the variants used across the dashboard.
const BADGE: Record<string, string> = {
  blue: "bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-400",
  green: "bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400",
  purple: "bg-purple-50 text-purple-600 dark:bg-purple-950/50 dark:text-purple-400",
  orange: "bg-orange-50 text-orange-600 dark:bg-orange-950/50 dark:text-orange-400",
  rose: "bg-rose-50 text-rose-600 dark:bg-rose-950/50 dark:text-rose-400",
  cyan: "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/50 dark:text-cyan-400",
  amber: "bg-amber-50 text-amber-600 dark:bg-amber-950/50 dark:text-amber-400",
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

const STATUS_BADGE: Record<string, string> = {
  completed: "bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400",
  ready: "bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400",
  success: "bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400",
  active: "bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400",
  processing: "bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400",
  parsing: "bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400",
  pending: "bg-gray-100 text-gray-600 dark:bg-gray-800/60 dark:text-gray-400",
  queued: "bg-gray-100 text-gray-600 dark:bg-gray-800/60 dark:text-gray-400",
  failed: "bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-400",
  error: "bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-400",
};

function StatusPill({ status }: { status: string }) {
  const cls = STATUS_BADGE[status?.toLowerCase()] ?? STATUS_BADGE.pending;
  return (
    <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium capitalize ${cls}`}>
      {status}
    </span>
  );
}

export default function AnalyticsPage() {
  const t = useTranslations();
  const router = useRouter();
  const { data: overview } = useSWR<AnalyticsOverview>("analytics-overview", () => api.analyticsOverview());
  const { data: docStats } = useSWR<AnalyticsDocStats>("analytics-docs", () => api.analyticsDocStats());
  const { data: usage } = useSWR<{ items: KBUsage[] }>("analytics-usage", () => api.analyticsUsage());
  const { data: activity } = useSWR<{ items: ActivityItem[] }>("analytics-activity", () => api.analyticsActivity(20));
  const { data: feedback } = useSWR<{ items: FeedbackItem[] }>("analytics-feedback", () => api.analyticsFeedback(50));

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold">{t("analytics.title")}</h1>
        <p className="text-sm text-muted-foreground mt-1">{t("analytics.overview")}</p>
      </div>

      {/* Overview cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
        <StatCard icon={BookOpen} color="blue" label={t("analytics.knowledge_bases")} value={overview?.knowledge_bases} loading={!overview} />
        <StatCard icon={FileText} color="green" label={t("analytics.documents")} value={overview?.documents} loading={!overview} />
        <StatCard icon={Layers} color="purple" label={t("analytics.chunks")} value={overview?.chunks} loading={!overview} />
        <StatCard icon={MessageSquare} color="orange" label={t("analytics.conversations")} value={overview?.conversations} loading={!overview} />
        <StatCard icon={Activity} color="rose" label={t("analytics.messages")} value={overview?.messages} loading={!overview} />
        <StatCard icon={HardDrive} color="cyan" label={t("analytics.storage")} value={overview ? formatSize(overview.storage_bytes) : undefined} loading={!overview} />
      </div>

      {/* Document processing stats */}
      <div className="grid md:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Badge icon={FileText} color="green" />
              {t("analytics.doc_stats")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {!docStats ? (
              <Skeleton className="h-20 w-full" />
            ) : (
              <>
                <div className="grid grid-cols-3 gap-3 text-center">
                  <div className="rounded-md bg-muted/40 py-2">
                    <p className="text-2xl font-semibold">{docStats.total}</p>
                    <p className="text-xs text-muted-foreground">{t("analytics.total_docs")}</p>
                  </div>
                  <div className="rounded-md bg-green-50 dark:bg-green-950/30 py-2">
                    <p className="text-2xl font-semibold text-green-600 dark:text-green-400">{(docStats.success_rate * 100).toFixed(0)}%</p>
                    <p className="text-xs text-muted-foreground">{t("analytics.success_rate")}</p>
                  </div>
                  <div className="rounded-md bg-muted/40 py-2">
                    <p className="text-2xl font-semibold">{docStats.total_chunks}</p>
                    <p className="text-xs text-muted-foreground">{t("analytics.total_chunks")}</p>
                  </div>
                </div>
                <div className="space-y-1.5 pt-2">
                  <p className="text-xs font-medium text-muted-foreground">{t("analytics.by_status")}</p>
                  {docStats.by_status?.map((s) => (
                    <div key={s.status} className="flex items-center justify-between text-sm">
                      <StatusPill status={s.status} />
                      <span className="font-medium">{s.count}</span>
                    </div>
                  ))}
                </div>
              </>
            )}
          </CardContent>
        </Card>

        {/* Recent activity */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Badge icon={Activity} color="rose" />
              {t("analytics.recent_activity")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {!activity ? (
              <Skeleton className="h-20 w-full" />
            ) : activity.items?.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("analytics.no_activity")}</p>
            ) : (
              <div className="space-y-1 max-h-64 overflow-auto pr-1">
                {activity.items?.slice(0, 15).map((a, i) => {
                  const color = a.kind === "document" ? "green" : "orange";
                  const Icon = a.kind === "document" ? FileText : MessageSquare;
                  return (
                    <div key={i} className="flex items-center gap-2 text-sm rounded-md px-1.5 py-1 hover:bg-muted/40">
                      <span className={`inline-flex h-6 w-6 items-center justify-center rounded ${BADGE[color]}`}>
                        <Icon className="h-3 w-3" />
                      </span>
                      <span className="truncate flex-1">{a.name}</span>
                      {a.status && <StatusPill status={a.status} />}
                      <span className="text-xs text-muted-foreground shrink-0">{formatTime(a.created_at)}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Usage by KB */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Badge icon={Database} color="purple" />
            {t("analytics.usage_by_kb")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {!usage ? (
            <Skeleton className="h-20 w-full" />
          ) : usage.items?.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("analytics.no_activity")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground border-b">
                    <th className="pb-2 pr-4 font-medium">{t("analytics.kb_name")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("analytics.doc_count")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("analytics.chunk_count")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("analytics.convs")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("analytics.storage")}</th>
                  </tr>
                </thead>
                <tbody>
                  {usage.items?.map((u) => (
                    <tr key={u.kb_id} className="border-b last:border-0 hover:bg-muted/40">
                      <td className="py-2 pr-4 truncate max-w-[200px]">{u.kb_name}</td>
                      <td className="py-2 px-4 text-right tabular-nums">{u.doc_count}</td>
                      <td className="py-2 px-4 text-right tabular-nums">{u.chunk_count}</td>
                      <td className="py-2 px-4 text-right tabular-nums">{u.conversations}</td>
                      <td className="py-2 px-4 text-right tabular-nums">{formatSize(u.storage_bytes)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      {/* Voted messages: downvotes are the bad-case review list */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Badge icon={ThumbsDown} color="rose" />
            {t("analytics.feedback")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {!feedback ? (
            <Skeleton className="h-20 w-full" />
          ) : feedback.items?.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("analytics.no_feedback")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground border-b">
                    <th className="pb-2 pr-4 font-medium">{t("analytics.feedback_vote")}</th>
                    <th className="pb-2 px-4 font-medium">{t("analytics.feedback_question")}</th>
                    <th className="pb-2 px-4 font-medium">{t("analytics.feedback_answer")}</th>
                    <th className="pb-2 px-4 font-medium">{t("analytics.kb_name")}</th>
                    <th className="pb-2 px-4 font-medium">{t("analytics.feedback_user")}</th>
                    <th className="pb-2 px-4 font-medium text-right">{t("analytics.feedback_time")}</th>
                    <th className="pb-2 pl-4 font-medium" />
                  </tr>
                </thead>
                <tbody>
                  {feedback.items?.map((f) => (
                    <tr key={f.id} className="border-b last:border-0 hover:bg-muted/40">
                      <td className="py-2 pr-4">
                        {f.vote === "down" ? (
                          <ThumbsDown className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />
                        ) : (
                          <ThumbsUp className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />
                        )}
                      </td>
                      <td className="py-2 px-4 truncate max-w-[220px]">{f.question}</td>
                      <td className="py-2 px-4 truncate max-w-[280px] text-muted-foreground">{f.answer}</td>
                      <td className="py-2 px-4 truncate max-w-[140px]">{f.kb_name}</td>
                      <td className="py-2 px-4 truncate max-w-[120px]">{f.user_name}</td>
                      <td className="py-2 pl-4 text-right text-xs text-muted-foreground whitespace-nowrap">{formatTime(f.created_at)}</td>
                      <td className="py-2 pl-4 text-right">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            router.push(`/dashboard/knowledge-bases/${f.kb_id}/executions?msg=${f.id}`)
                          }
                        >
                          <History className="h-3.5 w-3.5 mr-1" />
                          {t("analytics.view_execution")}
                        </Button>
                      </td>
                    </tr>
                  ))}
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
  value?: string | number;
  loading?: boolean;
}) {
  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4 flex flex-col gap-2">
        <Badge icon={Icon} color={color} />
        {loading ? (
          <Skeleton className="h-6 w-12" />
        ) : (
          <p className="text-xl font-semibold tabular-nums">{value ?? 0}</p>
        )}
        <p className="text-xs text-muted-foreground">{label}</p>
      </CardContent>
    </Card>
  );
}
