"use client";

import useSWR from "swr";
import { Coins, History, Sigma } from "lucide-react";
import { useTranslations } from "next-intl";
import { api, BillMine } from "@/lib/api";
import { formatCost, formatTime, formatTokens } from "@/lib/utils";
import { Skeleton } from "@/components/ui/skeleton";
import { BILL_SOURCE } from "@/lib/ui-colors";

const SOURCE = BILL_SOURCE;

function Stat({
  icon: Icon,
  label,
  value,
  loading,
}: {
  icon: typeof Sigma;
  label: string;
  value?: string;
  loading?: boolean;
}) {
  return (
    <div className="rounded-md border p-3 space-y-1 min-w-0">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Icon className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{label}</span>
      </div>
      {loading ? (
        <Skeleton className="h-5 w-16" />
      ) : (
        <p className="text-base font-semibold tabular-nums truncate">{value ?? "0"}</p>
      )}
    </div>
  );
}

// MyUsage shows the current user's own consumption: today's message quota,
// personal token/cost totals, and the most recent LLM calls. Lives in the
// chat profile dialog so regular members never need the backend dashboard.
export function MyUsage() {
  const t = useTranslations();
  // Same SWR key as the chat input bar, so the quota value is shared.
  const { data: msgQuota } = useSWR("msg-quota", () => api.getMessageQuota());
  const { data: mine, isLoading } = useSWR<BillMine>("bill-mine", () => api.billMine());

  return (
    <div className="space-y-4">
      {msgQuota && msgQuota.quota >= 0 && (
        <p className="text-sm text-muted-foreground">
          {t("chat.quota_remaining", {
            used: Math.max(0, msgQuota.quota - msgQuota.remaining),
            total: msgQuota.quota,
          })}
        </p>
      )}

      <div className="grid grid-cols-3 gap-2">
        <Stat
          icon={Sigma}
          label={t("bill.total_tokens")}
          value={mine ? formatTokens(mine.overview.total_tokens) : undefined}
          loading={isLoading}
        />
        <Stat
          icon={Coins}
          label={t("bill.amount")}
          value={mine ? formatCost(mine.overview.amount) : undefined}
          loading={isLoading}
        />
        <Stat
          icon={History}
          label={t("bill.call_count")}
          value={mine ? String(mine.overview.call_count) : undefined}
          loading={isLoading}
        />
      </div>

      <div>
        <p className="text-sm font-medium mb-2">{t("bill.recent_records")}</p>
        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : !mine || mine.records.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("bill.empty")}</p>
        ) : (
          <div className="space-y-1">
            {mine.records.map((r) => {
              const src = SOURCE[r.source] ?? SOURCE.chat;
              return (
                <div
                  key={r.id}
                  className="flex items-center gap-2 rounded-md border px-2 py-1.5 text-sm"
                >
                  <span className="text-xs text-muted-foreground whitespace-nowrap shrink-0">
                    {formatTime(r.created_at)}
                  </span>
                  <span
                    className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium shrink-0 ${src.cls}`}
                  >
                    {t(src.labelKey)}
                  </span>
                  <span className="truncate min-w-0">{r.model_name}</span>
                  <span className="ml-auto text-xs text-muted-foreground tabular-nums shrink-0">
                    {formatTokens(r.total_tokens)} · {formatCost(r.amount)}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">{t("bill.estimate_hint")}</p>
    </div>
  );
}
