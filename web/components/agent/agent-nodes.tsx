"use client";

import { memo } from "react";
import {
  BaseEdge,
  EdgeLabelRenderer,
  Handle,
  Position,
  getBezierPath,
  useReactFlow,
  type EdgeProps,
} from "reactflow";
import { Search, Brain, MessageSquare, GitBranch, Split, X } from "lucide-react";
import { useTranslations } from "next-intl";

interface NodeData {
  [key: string]: unknown;
}

// Per-node-type accent colors: classifier (routing) violet, retrieval (data)
// emerald, condition (decision) amber, llm (AI) blue, message (output) cyan.
type NodeAccent = { color: string; icon: string };

const accents: Record<string, NodeAccent> = {
  classifier: { color: "#8b5cf6", icon: "text-violet-500" },
  retrieval: { color: "#10b981", icon: "text-emerald-500" },
  condition: { color: "#f59e0b", icon: "text-amber-500" },
  llm: { color: "#3b82f6", icon: "text-blue-500" },
  message: { color: "#06b6d4", icon: "text-cyan-500" },
};

function nodeShell(
  label: string,
  icon: React.ReactNode,
  subtitle: string,
  selected: boolean,
  accent: NodeAccent,
  onDelete?: () => void,
  hasTarget = true,
  hasSource = true
) {
  return (
    <div
      className={`relative rounded-lg border border-l-4 bg-card px-4 py-3 shadow-sm w-52 transition-colors ${
        selected ? "border-primary ring-2 ring-primary/30" : "border-border"
      }`}
      style={{ borderLeftColor: accent.color }}
    >
      {hasTarget && (
        <Handle type="target" position={Position.Left} className="!h-2 !w-2 !bg-muted-foreground" />
      )}
      <div className="flex items-center gap-2 mb-1">
        <span className={accent.icon}>{icon}</span>
        <span className="font-medium text-sm">{label}</span>
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
    onDelete
  );
});

export const ConditionNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const variable = String(data.variable || "hit_count");
  const op = String(data.operator || ">");
  const val = String(data.value || "0");
  return nodeShell(
    t("agent.node_condition"),
    <GitBranch className="h-4 w-4" />,
    `${variable} ${op} ${val}`,
    !!selected,
    accents.condition,
    onDelete
  );
});

export const ClassifierNode = memo(({ id, data, selected }: NodeCompProps) => {
  const t = useTranslations();
  const onDelete = useDeleteNode(id);
  const categories = Array.isArray(data.categories) ? data.categories as string[] : [];
  return nodeShell(
    t("agent.node_classifier"),
    <Split className="h-4 w-4" />,
    categories.length > 0 ? categories.join(" / ") : t("agent.node_classifier_default"),
    !!selected,
    accents.classifier,
    onDelete
  );
});

export const agentNodeTypes = {
  retrieval: RetrievalNode,
  llm: LLMNode,
  message: MessageNode,
  condition: ConditionNode,
  classifier: ClassifierNode,
};

// LabeledEdge renders the branch label as an HTML badge via EdgeLabelRenderer
// so theme CSS variables resolve correctly (SVG label bg falls back to black).
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
}: EdgeProps) {
  const { deleteElements } = useReactFlow();
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
  return (
    <>
      <BaseEdge
        path={edgePath}
        markerEnd={markerEnd}
        style={{ strokeWidth: selected ? 2.5 : 1.5 }}
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
            <span className="rounded border border-border bg-popover px-2 py-0.5 text-xs font-medium text-foreground shadow-sm cursor-pointer hover:border-primary transition-colors">
              {label}
            </span>
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
      </EdgeLabelRenderer>
    </>
  );
});

export const agentEdgeTypes = {
  labeled: LabeledEdge,
};
