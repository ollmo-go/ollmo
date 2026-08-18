"use client";

import { useState } from "react";
import { Copy, Loader2, Play } from "lucide-react";
import { toast } from "sonner";
import { AgentNode, NodeDebugResult, api } from "@/lib/api";
import { useTranslations } from "next-intl";

export function NodeDebugSection({
  kbId,
  node,
  query,
  onQueryChange,
}: {
  kbId: string;
  node: AgentNode;
  query: string;
  onQueryChange: (v: string) => void;
}) {
  const t = useTranslations();
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<NodeDebugResult | null>(null);

  async function run() {
    const q = query.trim();
    if (!q) {
      toast(t("agent.debug_no_query"));
      return;
    }
    setRunning(true);
    setResult(null);
    try {
      const res = await api.debugAgentNode(kbId, node, q);
      setResult(res);
    } catch (e) {
      toast((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="border-t pt-3 space-y-2">
      <div className="text-sm font-medium">{t("agent.test_node")}</div>
      <div className="flex gap-2 min-w-0">
        <input
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              run();
            }
          }}
          placeholder={t("agent.debug_query_placeholder")}
          className="flex-1 min-w-0 rounded-md border border-border bg-background px-2 py-1 text-sm"
        />
        <button
          onClick={run}
          disabled={running}
          className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs hover:bg-accent disabled:opacity-50 transition-colors"
        >
          {running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
          {t("agent.debug_run")}
        </button>
      </div>
      {result && (
        <div className="rounded-md border bg-muted/30 p-2 text-xs space-y-2 overflow-hidden">
          <div className="flex items-center gap-1.5 text-muted-foreground flex-wrap">
            <span className="inline-flex items-center rounded bg-background px-1.5 py-0.5 border">
              {result.duration_ms != null ? `${result.duration_ms}ms` : ""}
            </span>
            {result.type === "retrieval" && (
              <span className="inline-flex items-center rounded bg-background px-1.5 py-0.5 border">
                {t("agent.debug_hits", { count: result.hits ?? 0 })}
              </span>
            )}
            {result.type === "retrieval" && result.top_score ? (
              <span className="inline-flex items-center rounded bg-background px-1.5 py-0.5 border">
                {t("agent.var_top_score")} {result.top_score.toFixed(3)}
              </span>
            ) : null}
          </div>
          {result.variables && Object.keys(result.variables).length > 0 && (
            <div className="space-y-1">
              <div className="text-muted-foreground">{t("agent.debug_vars")}</div>
              <div className="rounded-md border divide-y overflow-hidden">
                {Object.entries(result.variables).map(([k, v]) => (
                  <button
                    key={k}
                    type="button"
                    title={t("agent.debug_copy_var")}
                    onClick={() => {
                      navigator.clipboard.writeText(k);
                      toast(t("agent.debug_var_copied"));
                    }}
                    className="w-full text-left px-2 py-1.5 hover:bg-accent/50 transition-colors group"
                  >
                    <div className="flex items-center gap-1 min-w-0">
                      <code className="min-w-0 truncate font-mono text-[11px] text-primary/90">{k}</code>
                      <Copy className="ml-auto shrink-0 h-3 w-3 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                    </div>
                    <div className="mt-0.5 break-words text-foreground/80">{v}</div>
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="max-h-48 overflow-y-auto space-y-1.5 min-w-0">
            {(result.type === "retrieval" ? result.context : result.text)
              ?.split(/\n\n+/)
              .map((seg) => seg.trim())
              .filter(Boolean)
              .map((seg, i) => (
                <p
                  key={i}
                  className="whitespace-pre-wrap break-words min-w-0 text-foreground/80 bg-background rounded border px-2 py-1.5"
                >
                  {seg}
                </p>
              ))}
          </div>
        </div>
      )}
    </div>
  );
}
