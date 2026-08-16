"use client";

import { useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import useSWR from "swr";
import ReactFlow, {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  type Edge,
  type Node,
  ReactFlowProvider,
} from "reactflow";
import "reactflow/dist/style.css";
import { History, X } from "lucide-react";
import { toast } from "sonner";

import { api, Execution, Paginated, TraceStep } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent } from "@/components/ui/card";
import { agentNodeTypes, agentEdgeTypes, NODE_TYPE_LABEL_KEY } from "@/components/agent/agent-nodes";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";

// toFlow converts a saved agent definition into ReactFlow shape, dropping the
// implicit start/end nodes. Mirrors the canvas page but without slug
// backfilling — replay only renders what was saved.
function toFlow(def: { nodes: { id: string; type: string; position: { x: number; y: number }; data?: Record<string, unknown> }[]; edges: { id: string; source: string; target: string; label?: string }[] }) {
  const nodes: Node[] = def.nodes
    .filter((n) => n.type !== "start" && n.type !== "end")
    .map((n) => ({ id: n.id, type: n.type, position: n.position, data: n.data }));
  const ids = new Set(nodes.map((n) => n.id));
  const edges: Edge[] = def.edges
    .filter((e) => ids.has(e.source) && ids.has(e.target))
    .map((e) => ({ id: e.id, source: e.source, target: e.target, label: e.label, type: "labeled" }));
  return { nodes, edges };
}

function statusBadgeClass(status: string) {
  if (status === "success") return "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-400";
  if (status === "error") return "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400";
  return "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-400";
}

export default function ExecutionsPage() {
  return (
    <ReactFlowProvider>
      <ExecutionsInner />
    </ReactFlowProvider>
  );
}

function ExecutionsInner() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const t = useTranslations();

  // Source filter: "" = all, "chat" = real conversations, "test" = drawer runs.
  const [source, setSource] = useState("");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Execution | null>(null);

  const { data: agent } = useSWR(`agent-${kbId}`, () => api.getAgent(kbId));
  const { data: execPage, isLoading } = useSWR<Paginated<Execution>>(
    ["executions", kbId, source, page],
    () => api.listExecutions(kbId, page, 20, source || undefined)
  );

  // Deep-link from the analytics feedback list: ?msg=<messageId> opens the
  // execution that produced that assistant message straight in the replay
  // drawer. Read once on mount. Messages answered before execution recording
  // shipped have no record — surface that instead of failing silently.
  useEffect(() => {
    const msgId = new URLSearchParams(window.location.search).get("msg");
    if (!msgId) return;
    api
      .getExecutionByMessage(kbId, msgId)
      .then((e) => setSelected(e))
      .catch(() => toast.error(t("exec.no_record")));
  }, [kbId, t]);

  const total = execPage?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / 20));

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("exec.title")}</h1>
          <p className="text-sm text-muted-foreground mt-1">{t("exec.desc")}</p>
        </div>
        <div className="flex gap-1 rounded-md border p-0.5">
          {[
            { v: "", key: "exec.source_all" },
            { v: "chat", key: "exec.source_chat" },
            { v: "test", key: "exec.source_test" },
          ].map((s) => (
            <button
              key={s.v}
              onClick={() => { setSource(s.v); setPage(1); }}
              className={cn(
                "rounded px-2.5 py-1 text-xs transition-colors",
                source === s.v ? "bg-primary text-primary-foreground" : "hover:bg-accent"
              )}
            >
              {t(s.key)}
            </button>
          ))}
        </div>
      </div>

      <Card>
        <CardContent className="p-4">
          {isLoading ? (
            <div className="space-y-2">
              {[...Array(5)].map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : !execPage?.items?.length ? (
            <p className="text-sm text-muted-foreground text-center py-8">{t("exec.empty")}</p>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-muted-foreground border-b">
                  <th className="py-2 pr-4 font-medium">{t("exec.col_time")}</th>
                  <th className="py-2 pr-4 font-medium">{t("exec.col_query")}</th>
                  <th className="py-2 pr-4 font-medium">{t("exec.col_status")}</th>
                  <th className="py-2 pr-4 font-medium">{t("exec.col_source")}</th>
                  <th className="py-2 pr-4 font-medium">{t("exec.col_duration")}</th>
                  <th className="py-2 font-medium" />
                </tr>
              </thead>
              <tbody>
                {execPage.items.map((e) => (
                  <tr key={e.id} className="border-b last:border-0 hover:bg-accent/50">
                    <td className="py-2 pr-4 text-muted-foreground whitespace-nowrap">
                      {new Date(e.created_at).toLocaleString()}
                    </td>
                    <td className="py-2 pr-4 max-w-[280px] truncate">{e.query}</td>
                    <td className="py-2 pr-4">
                      <span className={cn("rounded-full px-2 py-0.5 text-xs", statusBadgeClass(e.status))}>
                        {t(`exec.status_${e.status}`)}
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">
                      {e.source === "chat" ? t("exec.source_chat") : t("exec.source_test")}
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground whitespace-nowrap">{e.total_ms} ms</td>
                    <td className="py-2 text-right">
                      <Button size="sm" variant="outline" onClick={() => {
                        // Open with the list row first (fast), then fetch the
                        // full record: the list endpoint omits the heavy trace.
                        setSelected(e);
                        api.getExecution(e.id).then(setSelected).catch(() => {});
                      }}>
                        <History className="h-3.5 w-3.5 mr-1" />
                        {t("exec.replay")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {totalPages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <span className="text-sm text-muted-foreground">
                {Math.min((page - 1) * 20 + 1, total)}–{Math.min(page * 20, total)} / {total}
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
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  ›
                </button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {selected && (
        <ReplayDrawer
          kbId={kbId}
          execution={selected}
          definition={agent?.definition}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

// ReplayDrawer re-plays one execution: read-only canvas with the walked path
// highlighted (same classes as live tracing) plus a step timeline.
function ReplayDrawer({
  kbId,
  execution,
  definition,
  onClose,
}: {
  kbId: string;
  execution: Execution;
  definition?: { nodes: { id: string; type: string; position: { x: number; y: number }; data?: Record<string, unknown> }[]; edges: { id: string; source: string; target: string; label?: string }[] };
  onClose: () => void;
}) {
  const t = useTranslations();

  const { nodes, edges, steps, traceNodeIds, traceEdgeIds, nodeRuntime } = useMemo(() => {
    const { nodes: fn, edges: fe } = toFlow(definition ?? { nodes: [], edges: [] });
    let steps: TraceStep[] = [];
    try {
      steps = execution.trace ? JSON.parse(execution.trace) : [];
    } catch {
      steps = [];
    }
    const traceNodeIds = new Set<string>();
    const traceEdgeIds = new Set<string>();
    const nodeRuntime: Record<string, { status: string; ms: number }> = {};
    for (const s of steps) {
      if (s.node_id) {
        traceNodeIds.add(s.node_id);
        if (s.status) nodeRuntime[s.node_id] = { status: s.status, ms: s.duration_ms ?? 0 };
      }
      if (s.edge_id) traceEdgeIds.add(s.edge_id);
    }
    const nodes = fn.map((n) => ({
      ...n,
      className: traceNodeIds.has(n.id) ? "agent-trace-active" : undefined,
      data: { ...n.data, __runtime: nodeRuntime[n.id] },
      draggable: false,
      connectable: false,
      selectable: false,
    }));
    const edges = fe.map((e) => ({
      ...e,
      animated: traceEdgeIds.has(e.id),
      className: traceEdgeIds.has(e.id) ? "agent-trace-edge" : undefined,
    }));
    return { nodes, edges, steps, traceNodeIds, traceEdgeIds, nodeRuntime };
  }, [definition, execution.trace]);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative h-full w-full max-w-3xl bg-background shadow-xl flex flex-col animate-in slide-in-from-right">
        <div className="flex items-center justify-between border-b px-5 py-4">
          <div>
            <h3 className="font-semibold">{t("exec.replay_title")}</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              {new Date(execution.created_at).toLocaleString()} · {t(`exec.status_${execution.status}`)} · {execution.total_ms} ms
            </p>
          </div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label={t("common.close")}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto p-5 space-y-4">
          {/* Question / answer */}
          <div className="rounded-lg bg-muted px-4 py-3 text-sm">{execution.query}</div>
          {execution.answer && (
            <div className="rounded-lg border px-4 py-3 text-sm whitespace-pre-wrap">{execution.answer}</div>
          )}

          {/* Read-only canvas with the walked path highlighted */}
          {nodes.length > 0 ? (
            <div className="rounded-lg border" style={{ height: 360 }}>
              <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={agentNodeTypes}
                edgeTypes={agentEdgeTypes}
                nodesDraggable={false}
                nodesConnectable={false}
                edgesUpdatable={false}
                elementsSelectable={false}
                defaultEdgeOptions={{ type: "labeled", markerEnd: { type: MarkerType.ArrowClosed } }}
                fitView
                proOptions={{ hideAttribution: true }}
              >
                <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
                <Controls showInteractive={false} />
              </ReactFlow>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-4">{t("exec.no_graph")}</p>
          )}

          {/* Step timeline */}
          {steps.length > 0 && (
            <div>
              <h4 className="text-sm font-medium mb-2">{t("exec.steps")}</h4>
              <ol className="space-y-1.5">
                {steps.map((s, i) => {
                  const labelKey = s.node_type ? NODE_TYPE_LABEL_KEY[s.node_type] : undefined;
                  return (
                    <li key={i} className="flex items-center gap-2 text-xs rounded-md border px-3 py-2">
                      <span className="font-mono text-muted-foreground w-6">{i + 1}</span>
                      {s.node_id && (
                        <span className="font-medium">
                          {labelKey ? t(labelKey) : s.node_type}
                        </span>
                      )}
                      {s.edge_id && <span className="text-muted-foreground">{t("exec.edge_traversed")}</span>}
                      {s.detail && <span className="text-muted-foreground truncate">· {s.detail}</span>}
                      {!!s.duration_ms && (
                        <span className="ml-auto text-muted-foreground whitespace-nowrap">{s.duration_ms} ms</span>
                      )}
                    </li>
                  );
                })}
              </ol>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
