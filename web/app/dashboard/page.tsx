"use client";

import Link from "next/link";
import useSWR from "swr";
import {
  BookOpen,
  CheckCircle2,
  Circle,
  Cpu,
  MessageSquare,
  Plus,
  Settings,
  Sparkles,
  Users,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  api,
  KnowledgeBase,
  LLMModel,
  Paginated,
  type QuotaItem,
} from "@/lib/api";
import { useTranslations } from "next-intl";

const BADGE: Record<string, string> = {
  blue: "bg-teal-50 text-teal-600 dark:bg-teal-950/50 dark:text-teal-400",
  purple: "bg-purple-50 text-purple-600 dark:bg-purple-950/50 dark:text-purple-400",
  orange: "bg-orange-50 text-orange-600 dark:bg-orange-950/50 dark:text-orange-400",
  indigo: "bg-indigo-50 text-indigo-600 dark:bg-indigo-950/50 dark:text-indigo-400",
  green: "bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400",
};

function IconBadge({ icon: Icon, color }: { icon: LucideIcon; color: string }) {
  return (
    <span className={`inline-flex h-10 w-10 items-center justify-center rounded-lg ${BADGE[color]}`}>
      <Icon className="h-5 w-5" />
    </span>
  );
}

function quotaBarColor(pct: number) {
  if (pct >= 90) return "bg-red-500 dark:bg-red-400";
  if (pct >= 60) return "bg-amber-500 dark:bg-amber-400";
  return "bg-green-500 dark:bg-green-400";
}

export default function DashboardHome() {
  const t = useTranslations();
  // All list keys are shared with their real pages (same fetchers) so SWR
  // caches one copy: navigating between dashboard and those pages reuses the
  // data instead of re-fetching under a second "-dashboard" key.
  const { data: kbData, isLoading: kbLoading } = useSWR<Paginated<KnowledgeBase>>(
    "kb-list",
    () => api.listKBs(1, 50)
  );
  const { data: llmData, isLoading: llmLoading } = useSWR<Paginated<LLMModel>>(
    "llm-list",
    () => api.listLLMs(1, 50)
  );
  const { data: embedData } = useSWR("embedding-list", () => api.listEmbeddings(1, 50));
  const { data: rerankData } = useSWR("rerank-list", () => api.listReranks(1, 50));
  const { data: quota } = useSWR("tenant-quota", () => api.getQuota());

  const hasKB = (kbData?.total ?? 0) > 0;
  const hasLLM = (llmData?.total ?? 0) > 0;
  const loading = kbLoading || llmLoading;
  const setupComplete = hasKB && hasLLM;

  const steps = [
    {
      done: hasKB,
      title: t("onboarding.step1_title"),
      desc: t("onboarding.step1_desc"),
      href: "/dashboard/knowledge-bases",
      cta: t("kb.create"),
      icon: BookOpen,
    },
    {
      done: hasLLM,
      title: t("onboarding.step2_title"),
      desc: t("onboarding.step2_desc"),
      href: "/dashboard/settings/models",
      cta: t("llm.create"),
      icon: Settings,
    },
    {
      done: false,
      title: t("onboarding.step3_title"),
      desc: t("onboarding.step3_desc"),
      href: "/",
      cta: t("chat.new_conversation"),
      icon: MessageSquare,
    },
  ];

  const firstUndone = steps.findIndex((s) => !s.done);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("dashboard.title")}
        </h1>
        <p className="text-muted-foreground mt-1">{t("dashboard.welcome")}</p>
      </div>

      {quota && (
        <Card className="overflow-hidden">
          <CardHeader className="pb-3">
            <div className="flex items-center gap-3">
              <IconBadge icon={Users} color="indigo" />
              <div>
                <CardDescription>{t("team.team_info")}</CardDescription>
                <CardTitle className="text-xl">{quota.name}</CardTitle>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
              <span className="text-muted-foreground">
                {t("team.member_count")}:{" "}
                <span className="text-foreground font-medium">
                  {quota.member_count}
                </span>
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="text-muted-foreground">{t("team.plan")}</span>
                <span className="inline-flex items-center rounded-md bg-indigo-50 dark:bg-indigo-950/50 px-2 py-1 text-xs font-medium text-indigo-700 dark:text-indigo-300">
                  {quota.plan === "pro"
                    ? t("team.plan_pro")
                    : quota.plan === "enterprise"
                      ? t("team.plan_enterprise")
                      : t("team.plan_free")}
                </span>
              </span>
            </div>
          </CardContent>
        </Card>
      )}

      {!setupComplete && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-xl">
              <Sparkles className="h-5 w-5 text-primary" />
              {t("onboarding.title")}
            </CardTitle>
            <CardDescription>{t("onboarding.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading
              ? Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="flex items-center gap-3">
                    <Skeleton className="h-6 w-6 rounded-full" />
                    <div className="flex-1 space-y-1">
                      <Skeleton className="h-4 w-40" />
                      <Skeleton className="h-3 w-64" />
                    </div>
                  </div>
                ))
              : steps.map((step, i) => {
                  const Icon = step.icon;
                  const isNext = i === firstUndone;
                  return (
                    <div
                      key={i}
                      className={`flex items-center gap-3 rounded-lg border p-3 ${
                        step.done ? "bg-muted/40 opacity-70" : ""
                      }`}
                    >
                      {step.done ? (
                        <CheckCircle2 className="h-5 w-5 text-green-500 shrink-0" />
                      ) : (
                        <Circle className="h-5 w-5 text-muted-foreground shrink-0" />
                      )}
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium flex items-center gap-1.5">
                          <Icon className="h-3.5 w-3.5" />
                          {step.title}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {step.desc}
                        </p>
                      </div>
                      {!step.done && (
                        <Link href={step.href}>
                          <Button
                            size="sm"
                            variant={isNext ? "default" : "outline"}
                          >
                            {step.cta}
                          </Button>
                        </Link>
                      )}
                    </div>
                  );
                })}
          </CardContent>
        </Card>
      )}

      {setupComplete && !loading && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <QuickCard
            icon={BookOpen}
            color="blue"
            label={t("nav.knowledge_bases")}
            value={kbData?.total ?? 0}
            href="/dashboard/knowledge-bases"
            cta={t("kb.new")}
            ctaIcon={Plus}
          />
          <QuickCard
            icon={Cpu}
            color="purple"
            label={t("llm.title")}
            value={(llmData?.total ?? 0) + (embedData?.total ?? 0) + (rerankData?.total ?? 0)}
            href="/dashboard/settings/models"
            cta={t("llm.create")}
            ctaIcon={Settings}
          />
          <QuickCard
            icon={MessageSquare}
            color="orange"
            label={t("chat.title")}
            value="→"
            href="/"
            cta={t("chat.new_conversation")}
            ctaIcon={MessageSquare}
          />
        </div>
      )}

      {quota && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold tracking-tight">
            {t("settings.usage")}
          </h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[
              { item: quota.doc, label: t("settings.quota_documents") },
              { item: quota.vector, label: t("settings.quota_vectors") },
              { item: quota.message, label: t("settings.quota_messages") },
            ].map(({ item, label }, i) => {
              if (!item) return null;
              const pct = item.limit > 0 ? (item.used / item.limit) * 100 : 0;
              return (
                <Card key={i}>
                  <CardHeader className="pb-2">
                    <CardDescription>{label}</CardDescription>
                    <CardTitle className="text-2xl tabular-nums">
                      {item.used}
                      <span className="text-base text-muted-foreground">
                        {" / "}
                        {item.limit > 0 ? item.limit : "∞"}
                      </span>
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="h-2 rounded-full bg-muted overflow-hidden">
                      <div
                        className={`h-full transition-all ${quotaBarColor(pct)}`}
                        style={{ width: `${item.limit > 0 ? pct : 0}%` }}
                      />
                    </div>
                    {item.warning && (
                      <p className="text-xs text-destructive mt-2">
                        {t("settings.quota_warning")}
                      </p>
                    )}
                  </CardContent>
                </Card>
              );
            })}
          </div>
          {quota.user_message_limit !== undefined && (
            <p className="text-sm text-muted-foreground">
              {t("settings.user_message_limit")}:{" "}
              {quota.user_message_limit > 0
                ? quota.user_message_limit
                : "∞"}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function QuickCard({
  icon: Icon,
  color,
  label,
  value,
  href,
  cta,
  ctaIcon: CtaIcon,
}: {
  icon: LucideIcon;
  color: string;
  label: string;
  value: string | number;
  href: string;
  cta: string;
  ctaIcon: LucideIcon;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-3">
          <IconBadge icon={Icon} color={color} />
          <div>
            <CardDescription>{label}</CardDescription>
            <CardTitle className="text-3xl tabular-nums">{value}</CardTitle>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <Link href={href}>
          <Button variant="outline" size="sm" className="w-full">
            <CtaIcon className="h-3.5 w-3.5 mr-1" />
            {cta}
          </Button>
        </Link>
      </CardContent>
    </Card>
  );
}
