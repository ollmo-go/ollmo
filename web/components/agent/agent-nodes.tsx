"use client";

import { memo, useState } from "react";
import { createPortal } from "react-dom";
import {
  BaseEdge,
  EdgeLabelRenderer,
  Handle,
  Position,
  getBezierPath,
  useReactFlow,
  type EdgeProps,
} from "reactflow";
import { Search, Brain, MessageSquare, GitBranch, Split, Check, Plus, X, AlertCircle, StickyNote } from "lucide-react";
import { useTranslations } from "next-intl";
import { normalizeCategories } from "./agent-config-panel";
import { AGENT_NODE_ACCENTS } from "@/lib/ui-colors";

interface NodeData {
  [key: string]: unknown;
}

// Per-node-type accent colors: classifier (routing) violet, retrieval (data)
// emerald, condition (decision) amber, llm (AI) teal, message (output) cyan.
type NodeAccent = { color: string; icon: string };

// Node accent colors live in lib/ui-colors (hex for react-flow + icon class).
const accents = AGENT_NODE_ACCENTS;

function nodeShell(
  label: string,
  icon: React.ReactNode,
  subtitle: string,
  selected: boolean,
  accent: NodeAccent,
  data: NodeData,
  onDelete?: () => void,
  hasTarget = true,
  hasSource = true
) {
  // __runtime/__issues are injected at render time by the canvas page (trace
  // badges and validation problems); they never reach the saved definition.
  const runtime = data.__runtime as { status?: string; ms?: number } | undefined;
  const issues = (data.__issues as string[] | undefined) ?? [];
  return (
    <div
      className={`relative rounded-lg border border-l-4 bg-card px-4 py-3 shadow-sm w-52 transition-colors ${
        selected ? "border-primary ring-2 ring-primary/30" : "border-border"
      }`}
      style={{ borderLeftColor: accent.color }}
    >
      {issues.length > 0 && (
        <span
          title={issues.join("\n")}
          className="absolute -top-2 -left-2 flex h-4 w-4 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-sm"
        >
          <AlertCircle className="h-2.5 w-2.5" />
        </span>
      )}
      {hasTarget && (
        <Handle type="target" position={Position.Left} className="!h-2 !w-2 !bg-muted-foreground" />
      )}
      <div className="flex items-center gap-2 mb-1">
        <span className={accent.icon}>{icon}</span>
        <span className="font-medium text-sm">{label}</span>
        {runtime?.status === "ok" && (
          <span className="ml-auto inline-flex items-center gap-0.5 rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
            <Check className="h-2.5 w-2.5" />
            {runtime.ms ? `${(runtime.ms / 1000).toFixed(1)}s` : ""}
          </span>
        )}
      </div>
      <div className="text-xs text-muted-foreground truncate">{subtitle}</div>
      {hasSource && (
        <Handle type="source" position={Position.Right} className="!h-2 !w-2 !bg-muted-foreground" />
      )}
      {selected && onDelete && (
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(); }}
          className="absolute -top-2 -right-2 flex h-5 w-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 transition-colors nodrag"
          aria-label="delete"
        >
          <X className="h-3 w-3" />
        </button>
      )}
    </div>
  );
}

type NodeCompProps = { id: string; data: NodeData; selected?: boolean };

function useDeleteNode(id: string) {
  const { deleteElements } = useReactFlow();
  return () => deleteElements({ nodes: [{ id }] });
}

export const RetrievalNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const rerank = data.rerank ? t("agent.node_rerank_on") : t("agent.node_rerank_off");
  const graph = data.use_graph ? t("agent.node_graph_on") : t("agent.node_graph_off");
  return nodeShell(
    t("agent.node_retrieval"),
    <Search className="h-4 w-4" />,
    `top_k ${Number(data.top_k || 10)} · ${rerank} · ${graph}`,
    !!selected,
    accents.retrieval,
    data,
    onDelete
  );
});

export const LLMNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const temp = Number(data.temperature ?? 0.7);
  const maxTok = Number(data.max_tokens || 2048);
  return nodeShell(
    t("agent.node_llm"),
    <Brain className="h-4 w-4" />,
    `temp ${temp} · max_tokens ${maxTok}`,
    !!selected,
    accents.llm,
    data,
    onDelete
  );
});

export const MessageNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const text = String(data.text || "");
  return nodeShell(
    t("agent.node_message"),
    <MessageSquare className="h-4 w-4" />,
    text ? text.slice(0, 40) : t("agent.node_message_default"),
    !!selected,
    accents.message,
    data,
    onDelete
  );
});

export const ConditionNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const variable = String(data.variable || "hit_count");
  const op = String(data.operator || ">");
  const val = String(data.value ?? "");
  const varName =
    variable === "hit_count"
      ? t("agent.var_hit_count")
      : variable === "top_score"
        ? t("agent.var_top_score")
        : variable === "query"
          ? t("agent.var_query")
          : variable;
  return nodeShell(
    t("agent.node_condition"),
    <GitBranch className="h-4 w-4" />,
    `${varName} ${op === "contains" ? t("agent.op_contains") : op} ${val}`,
    !!selected,
    accents.condition,
    data,
    onDelete
  );
});

export const ClassifierNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const names = normalizeCategories(data.categories).map((c) => c.name.trim()).filter(Boolean);
  return nodeShell(
    t("agent.node_classifier"),
    <Split className="h-4 w-4" />,
    names.length > 0 ? names.join(" / ") : t("agent.node_classifier_default"),
    !!selected,
    accents.classifier,
    data,
    onDelete
  );
});

// NoteNode is a sticky note for documenting the canvas. It never executes:
// no handles (cannot be wired into the flow) and the executor skips it
// because isolated nodes are never enqueued.
export const NoteNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const text = String(data.text || "");
  return (
    <div
      className={`relative w-56 rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-950/40 px-3 py-2 shadow-sm ${
        selected ? "ring-2 ring-primary/40" : ""
      }`}
    >
      <div className="flex items-center gap-1.5 mb-1 text-amber-600 dark:text-amber-400">
        <StickyNote className="h-3.5 w-3.5" />
        <span className="text-xs font-medium">{t("agent.node_note")}</span>
      </div>
      <div className="text-xs whitespace-pre-wrap text-foreground/90 min-h-4 line-clamp-6">
        {text || t("agent.note_placeholder")}
      </div>
      {selected && (
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(); }}
          className="absolute -top-2 -right-2 flex h-5 w-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 transition-colors nodrag"
          aria-label="delete"
        >
          <X className="h-3 w-3" />
        </button>
      )}
    </div>
  );
});

export const agentNodeTypes = {
  retrieval: RetrievalNode,
  llm: LLMNode,
  message: MessageNode,
  condition: ConditionNode,
  classifier: ClassifierNode,
  note: NoteNode,
};

// Node type -> i18n label key. Shared by the canvas palette and the
// execution replay timeline so names stay in one place.
export const NODE_TYPE_LABEL_KEY: Record<string, string> = {
  retrieval: "agent.node_retrieval",
  llm: "agent.node_llm",
  message: "agent.node_message",
  condition: "agent.node_condition",
  classifier: "agent.node_classifier",
  note: "agent.node_note",
};

// LabeledEdge renders the branch label as an HTML badge via EdgeLabelRenderer
// so theme CSS variables resolve correctly (SVG label bg falls back to black).
// Hovering the edge reveals a "+" under the midpoint that inserts a new node
// between source and target without deleting and re-wiring by hand.
const INSERTABLE_NODES: { type: string; labelKey: string; icon: React.ReactNode; color: string }[] = [
  { type: "retrieval", labelKey: "agent.node_retrieval", icon: <Search className="h-3.5 w-3.5" />, color: AGENT_NODE_ACCENTS.retrieval.icon },
  { type: "llm", labelKey: "agent.node_llm", icon: <Brain className="h-3.5 w-3.5" />, color: AGENT_NODE_ACCENTS.llm.icon },
  { type: "message", labelKey: "agent.node_message", icon: <MessageSquare className="h-3.5 w-3.5" />, color: AGENT_NODE_ACCENTS.message.icon },
  { type: "condition", labelKey: "agent.node_condition", icon: <GitBranch className="h-3.5 w-3.5" />, color: AGENT_NODE_ACCENTS.condition.icon },
  { type: "classifier", labelKey: "agent.node_classifier", icon: <Split className="h-3.5 w-3.5" />, color: AGENT_NODE_ACCENTS.classifier.icon },
];

const LabeledEdge = memo(function LabeledEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  markerEnd,
  label,
  selected,
  data,
}: EdgeProps) {
  const { deleteElements } = useReactFlow();
  const t = useTranslations();
  const [hovered, setHovered] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  // Viewport-space position of the open menu, anchored under the + button.
  const [menuPos, setMenuPos] = useState<{ x: number; y: number } | null>(null);
  const onInsertNode = (data as { onInsertNode?: (type: string) => void } | undefined)?.onInsertNode;
  // Branch semantics: condition-true edges read as success (green check),
  // false as failure (red cross). Legacy spellings stay recognized.
  const isTrue = label === t("agent.branch_true") || label === "true" || label === "条件成立";
  const isFalse = label === t("agent.branch_false") || label === "false" || label === "条件不成立";
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  const onDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    deleteElements({ edges: [{ id }] });
  };
  const showInsert = (hovered || selected || menuOpen) && !!onInsertNode;
  return (
    <>
      <BaseEdge
        path={edgePath}
        markerEnd={markerEnd}
        style={{ strokeWidth: selected ? 2.5 : 1.5 }}
      />
      {/* Wide invisible stroke so hovering the edge (not just the 1.5px line)
          reveals the insert button. */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        className="react-flow__edge-interaction"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      />
      <EdgeLabelRenderer>
        <div
          style={{
            position: "absolute",
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            pointerEvents: "all",
          }}
          className="flex items-center gap-1 nodrag nopan"
        >
          {label ? (
            isTrue || isFalse ? (
              <span
                className={`flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-medium shadow-sm cursor-pointer transition-colors ${
                  isTrue
                    ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 hover:border-emerald-500"
                    : "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400 hover:border-red-500"
                }`}
              >
                {isTrue ? <Check className="h-3 w-3" /> : <X className="h-3 w-3" />}
                {label}
              </span>
            ) : (
              <span className="rounded border border-border bg-popover px-2 py-0.5 text-xs font-medium text-foreground shadow-sm cursor-pointer hover:border-primary transition-colors">
                {label}
              </span>
            )
          ) : null}
          {selected ? (
            <button
              onClick={onDelete}
              className="flex h-4 w-4 items-center justify-center rounded-full bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 transition-colors"
              aria-label="delete edge"
            >
              <X className="h-2.5 w-2.5" />
            </button>
          ) : null}
        </div>
        {showInsert && (
          <div
            style={{
              position: "absolute",
              transform: `translate(-50%, 0) translate(${labelX}px, ${labelY + 14}px)`,
              pointerEvents: "all",
            }}
            className="nodrag nopan"
          >
            <button
              onClick={(e) => {
                e.stopPropagation();
                if (menuOpen) {
                  setMenuOpen(false);
                  setMenuPos(null);
                  return;
                }
                // Anchor the portal menu to the + button's own screen rect —
                // fixed positioning needs viewport coords, not flow coords.
                const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
                setMenuPos({ x: r.left + r.width / 2, y: r.bottom + 4 });
                setMenuOpen(true);
              }}
              title={t("agent.insert_node")}
              aria-label={t("agent.insert_node")}
              className="flex h-5 w-5 items-center justify-center rounded-full border border-border bg-background text-foreground shadow-sm hover:border-primary hover:text-primary transition-colors"
            >
              <Plus className="h-3 w-3" />
            </button>
          </div>
        )}
        {menuOpen && menuPos &&
          createPortal(
            <>
              {/* Click-away backdrop closing the menu from anywhere. */}
              <div
                className="fixed inset-0 z-40"
                onClick={() => { setMenuOpen(false); setMenuPos(null); }}
              />
              {/* Portaled to body: the edge-label layer sits below the node
                  layer, so an in-place menu gets covered by nearby nodes. */}
              <div
                className="fixed z-50 rounded-md border border-border bg-popover shadow-md py-1 w-40"
                style={{ left: menuPos.x, top: menuPos.y, transform: "translateX(-50%)" }}
              >
                {INSERTABLE_NODES.map((item) => (
                  <button
                    key={item.type}
                    onClick={(e) => {
                      e.stopPropagation();
                      setMenuOpen(false);
                      setMenuPos(null);
                      onInsertNode!(item.type);
                    }}
                    className="flex w-full items-center gap-2 text-left px-3 py-1.5 text-sm hover:bg-accent transition-colors"
                  >
                    <span className={item.color}>{item.icon}</span>
                    {t(item.labelKey)}
                  </button>
                ))}
              </div>
            </>,
            document.body
          )}
      </EdgeLabelRenderer>
    </>
  );
});

export const agentEdgeTypes = {
  labeled: LabeledEdge,
};
