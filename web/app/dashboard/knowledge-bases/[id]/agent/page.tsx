"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Play, Save, Send, Square, X, Plus, Search, Brain, MessageSquare, GitBranch, Split, LayoutGrid, Redo2, StickyNote, Undo2 } from "lucide-react";
import ReactFlow, {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  type Edge,
  type Node,
  type Connection,
  ReactFlowProvider,
  addEdge,
  updateEdge,
  useEdgesState,
  useNodesState,
  useReactFlow,
  useStore,
} from "reactflow";
import useSWR from "swr";
import dagre from "dagre";
import "reactflow/dist/style.css";

import { api, AgentDefinition, AgentEdge, AgentNode, Citation, Message, ReplyStats, TraceStep } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { agentNodeTypes, agentEdgeTypes } from "@/components/agent/agent-nodes";
import { AgentConfigPanel, normalizeCategories } from "@/components/agent/agent-config-panel";
import { MessageBubble } from "@/components/chat/message-bubble";
import { useTranslations } from "next-intl";
import { consumeChatStream } from "@/lib/stream";
import { toast } from "sonner";

// nextSlug returns a language-independent, unique variable name for a new
// node, e.g. retrieval_1, llm_2. Prompts reference these as {slug.output}.
function nextSlug(nodes: { data?: Record<string, unknown> }[], type: string): string {
  const taken = new Set(nodes.map((n) => String(n.data?.slug ?? "")));
  let i = 1;
  while (taken.has(`${type}_${i}`)) i++;
  return `${type}_${i}`;
}

function toFlow(def: AgentDefinition): { nodes: Node[]; edges: Edge[] } {
  // Filter out implicit start/end nodes from legacy definitions. Nodes from
  // graphs predating the variable system get a slug backfilled so their
  // outputs become referenceable.
  const raw = def.nodes.filter((n: AgentNode) => n.type !== "start" && n.type !== "end");
  const slugged = raw.map((n) =>
    n.data?.slug || n.type === "note" ? n : { ...n, data: { ...n.data, slug: nextSlug(raw, n.type) } }
  );
  const nodes: Node[] = slugged.map((n: AgentNode) => ({
    id: n.id,
    type: n.type,
    position: n.position,
    data: n.data,
  }));
  const edges: Edge[] = def.edges
    .filter((e: AgentEdge) =>
      nodes.some((n) => n.id === e.source) && nodes.some((n) => n.id === e.target)
    )
    .map((e: AgentEdge) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      label: e.label,
      type: "labeled",
    }));
  return { nodes, edges };
}

function fromFlow(nodes: Node[], edges: Edge[], openingMessage: string, suggestedQuestions: string[]): AgentDefinition {
  return {
    nodes: nodes.map((n) => ({
      id: n.id,
      type: n.type || "retrieval",
      position: n.position,
      data: n.data as Record<string, unknown>,
    })),
    edges: edges.map((e) => ({ id: e.id, source: e.source, target: e.target, label: typeof e.label === "string" ? e.label : undefined })),
    opening_message: openingMessage,
    // Clean up on save: trim, drop empty lines, cap at 4.
    suggested_questions: suggestedQuestions.map((s) => s.trim()).filter(Boolean).slice(0, 4),
  };
}

export default function AgentPage() {
  return (
    <ReactFlowProvider>
      <AgentCanvas />
    </ReactFlowProvider>
  );
}

// AgentChatDrawer is a right-side slide-out panel for quick agent testing.
// It streams replies via the test-chat endpoint without persisting anything.
function AgentChatDrawer({ kbId, onClose, onTrace, onTraceStep, onSendStart }: { kbId: string; onClose: () => void; onTrace?: (trace: TraceStep[]) => void; onTraceStep?: (step: TraceStep) => void; onSendStart?: () => void }) {
  const t = useTranslations();
  const [messages, setMessages] = useState<Message[]>([]);
  const [draft, setDraft] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [streamedText, setStreamedText] = useState("");
  const [streamedThinking, setStreamedThinking] = useState("");
  const [pendingCitations, setPendingCitations] = useState<Citation[]>([]);
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Daily message quota (shared with the main chat page). Test runs also
  // consume quota, so show the remaining count and block when exhausted.
  const { data: msgQuota, mutate: mutateQuota } = useSWR("msg-quota", () => api.getMessageQuota());
  const quotaExceeded = msgQuota !== undefined && msgQuota.quota >= 0 && msgQuota.remaining <= 0;

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, streamedText, streamedThinking]);

  // Focus the input when the drawer opens and after each reply completes,
  // so the user can immediately type the next test question.
  useEffect(() => {
    if (!streaming) inputRef.current?.focus();
  }, [streaming]);

  // Fetch the agent's opening message on mount without creating a
  // conversation. Uses the same SWR key as the canvas ("agent-${kbId}") so
  // the drawer reuses the cached agent instead of re-fetching it. The
  // conversation is created lazily on first send() to avoid creating +
  // deleting a conversation every time the drawer is opened (and twice in
  // React Strict Mode).
  const { data: agentData } = useSWR(`agent-${kbId}`, () => api.getAgent(kbId));
  useEffect(() => {
    const agent = agentData;
    if (!agent) return;
    const msg = agent.definition?.opening_message;
    if (typeof msg === "string" && msg) {
      setMessages([{
        id: "greeting",
        tenant_id: "",
        conversation_id: "",
        role: "assistant",
        content: msg,
        created_at: new Date().toISOString(),
      }]);
    }
    const qs = agent.definition?.suggested_questions;
    setSuggestedQuestions(Array.isArray(qs) ? qs : []);
  }, [agentData]);

  // Abort any in-flight stream on unmount.
  useEffect(() => {
    return () => { abortRef.current?.abort(); };
  }, []);

  async function send(overrideText?: string) {
    const text = (overrideText ?? draft).trim();
    if (!text || streaming) return;
    if (quotaExceeded) {
      toast.error(t("chat.quota_exceeded"));
      return;
    }
    if (!overrideText) setDraft("");
    onSendStart?.();

    const userMsg: Message = {
      id: crypto.randomUUID(),
      tenant_id: "",
      conversation_id: "",
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    };
    setMessages((m) => [...m, userMsg]);
    setStreamedText("");
    setStreamedThinking("");
    setPendingCitations([]);
    setStreaming(true);

    const controller = new AbortController();
    abortRef.current = controller;

    let acc = "";
    let thinkAcc = "";
    let cits: Citation[] = [];
    let doneStats: ReplyStats | undefined;
    let streamError = "";
    let finalized = false;

    // finalize ends the streaming UI at the done EVENT, not at stream
    // close: the server keeps the test stream open after done to generate
    // follow-up chips (an extra LLM call), which must not keep the
    // blinking cursor alive. The error/abort path reuses it.
    const finalize = (errMsg?: string) => {
      if (finalized) return;
      finalized = true;
      // Quota errors are shown as a toast only, not appended to the bubble.
      const isQuotaErr = !!errMsg && errMsg.includes("quota exceeded");
      const content = errMsg && !isQuotaErr
        ? (acc ? `${acc}\n\n> ${errMsg}` : `> ${errMsg}`)
        : acc;
      if (content) {
        setMessages((m) => [
          ...m,
          {
            id: crypto.randomUUID(),
            tenant_id: "",
            conversation_id: "",
            role: "assistant",
            content,
            reasoning: thinkAcc || undefined,
            citations: cits.length ? JSON.stringify(cits) : "",
            retrieve_ms: doneStats?.retrieve_ms,
            generate_ms: doneStats?.generate_ms,
            total_ms: doneStats?.total_ms,
            prompt_tokens: doneStats?.prompt_tokens,
            completion_tokens: doneStats?.completion_tokens,
            total_tokens: doneStats?.total_tokens,
            created_at: new Date().toISOString(),
          },
        ]);
      }
      if (errMsg) {
        // Map raw quota errors to a friendly localized message.
        if (isQuotaErr) {
          toast.error(t("chat.quota_exceeded"));
        } else {
          toast.error(errMsg);
        }
      }
      setStreamedText("");
      setStreamedThinking("");
      setPendingCitations([]);
      setStreaming(false);
    };

    try {
      const stream = api.testChat(kbId, { message: text }, controller.signal);
      const { error } = await consumeChatStream(stream, {
        onCitations: (c) => { cits = c; setPendingCitations(c); },
        onToken: (t) => { acc += t; setStreamedText(acc); },
        onThinking: (t) => { thinkAcc += t; setStreamedThinking(thinkAcc); },
        onDone: (_id, stats, trace) => { doneStats = stats; if (trace) onTrace?.(trace); finalize(); },
        onWarning: (w) => { acc += `> ${w}\n\n`; setStreamedText(acc); },
        onTraceStep: (step) => { onTraceStep?.(step); },
      });
      streamError = error ?? "";
    } finally {
      finalize(streamError);
      mutateQuota();
      abortRef.current = null;
    }
  }

  function stop() {
    abortRef.current?.abort();
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative h-full w-full max-w-md bg-background shadow-xl flex flex-col animate-in slide-in-from-right">
        {/* Header */}
        <div className="flex items-center justify-between border-b px-5 py-4">
          <h3 className="font-semibold">{t("agent.test_run")}</h3>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label={t("common.close")}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Messages */}
        <div ref={scrollRef} className="flex-1 overflow-y-auto p-5 space-y-4">
          {messages.length === 0 && !streaming && (
            <p className="text-sm text-muted-foreground text-center mt-8">
              {t("agent.test_placeholder")}
            </p>
          )}
          {messages.map((m) => (
            <MessageBubble key={m.id} message={m} kbId={kbId} />
          ))}

          {/* Suggested questions: shown until the user sends the first message. */}
          {suggestedQuestions.length > 0 &&
            !messages.some((m) => m.role === "user") &&
            !streaming && (
              <div className="flex flex-wrap gap-2 mt-2">
                {suggestedQuestions.map((q) => (
                  <button
                    key={q}
                    onClick={() => send(q)}
                    disabled={streaming || quotaExceeded}
                    className="rounded-full border border-border bg-background px-3 py-1.5 text-xs text-foreground hover:bg-accent hover:border-primary/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {q}
                  </button>
                ))}
              </div>
            )}

          {/* Streaming reply */}
          {streaming && (
            <MessageBubble
              message={{
                id: "streaming",
                tenant_id: "",
                conversation_id: "",
                role: "assistant",
                content: streamedText,
                reasoning: streamedThinking || undefined,
                citations: pendingCitations.length ? JSON.stringify(pendingCitations) : "",
                created_at: new Date().toISOString(),
              }}
              streaming
              isThinking={!!streamedThinking && !streamedText}
              kbId={kbId}
            />
          )}
        </div>

        {/* Input */}
        <div className="border-t p-4">
          {msgQuota && msgQuota.quota >= 0 && (
            <p className={`text-xs mb-2 ${quotaExceeded ? "text-destructive" : "text-muted-foreground"}`}>
              {quotaExceeded
                ? t("chat.quota_exceeded")
                : t("chat.quota_remaining", { used: Math.max(0, msgQuota.quota - msgQuota.remaining), total: msgQuota.quota })}
            </p>
          )}
          <div className="flex gap-2">
            <Input
              ref={inputRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  send();
                }
              }}
              placeholder={t("agent.test_input_placeholder")}
              disabled={streaming || quotaExceeded}
            />
            {streaming ? (
              <Button size="icon" variant="destructive" onClick={stop}>
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button size="icon" onClick={() => send()} disabled={!draft.trim() || quotaExceeded}>
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

const NODE_PALETTE: { type: string; icon: React.ReactNode; labelKey: string; color: string }[] = [
  { type: "retrieval", icon: <Search className="h-3.5 w-3.5" />, labelKey: "agent.node_retrieval", color: "text-emerald-500" },
  { type: "llm", icon: <Brain className="h-3.5 w-3.5" />, labelKey: "agent.node_llm", color: "text-blue-500" },
  { type: "message", icon: <MessageSquare className="h-3.5 w-3.5" />, labelKey: "agent.node_message", color: "text-cyan-500" },
  { type: "condition", icon: <GitBranch className="h-3.5 w-3.5" />, labelKey: "agent.node_condition", color: "text-amber-500" },
  { type: "classifier", icon: <Split className="h-3.5 w-3.5" />, labelKey: "agent.node_classifier", color: "text-violet-500" },
];

const DEFAULT_NODE_DATA: Record<string, Record<string, unknown>> = {
  retrieval: { top_k: 10, rerank: true, use_graph: true },
  llm: { system_prompt: "你是一个友好的助手，请根据上下文回答用户问题。如果上下文没有相关信息，可以与用户自由对话。", temperature: 0.7, max_tokens: 2048, top_p: 0.9 },
  message: { text: "" },
  condition: { variable: "query", operator: "contains", value: "" },
  classifier: { categories: [], llm_model_id: "" },
  note: { text: "" },
};

// Node card metrics for dagre: width w-52 = 208px, content height ~76px.
const NODE_W = 208;
const NODE_H = 76;

// layoutNodes re-positions nodes with dagre (left-to-right layered layout)
// so hand-built graphs and freshly added nodes stay tidy.
function layoutNodes(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "LR", nodesep: 50, ranksep: 110, marginx: 40, marginy: 40 });
  g.setDefaultEdgeLabel(() => ({}));
  nodes.forEach((n) => g.setNode(n.id, { width: NODE_W, height: NODE_H }));
  edges.forEach((e) => g.setEdge(e.source, e.target));
  dagre.layout(g);
  return nodes.map((n) => {
    const pos = g.node(n.id) as { x: number; y: number };
    return { ...n, position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 } };
  });
}

// --- Drag alignment guides (FastGPT-style) ---
// While dragging, snap the node's left/center/right (and top/center/bottom)
// to any other node's within a few pixels, and report the guide line that
// spans both nodes so the canvas can render it.
const SNAP_PX = 6;

type GuideLine =
  | { hLine: { y: number; x1: number; x2: number }; vLine?: undefined }
  | { vLine: { x: number; y1: number; y2: number }; hLine?: undefined }
  | { hLine: { y: number; x1: number; x2: number }; vLine: { x: number; y1: number; y2: number } }
  | { hLine?: undefined; vLine?: undefined };

function snapToGuides(dragged: Node, others: Node[]): {
  position: { x: number; y: number };
  guides: GuideLine;
} {
  const dw = dragged.width ?? NODE_W;
  const dh = dragged.height ?? NODE_H;
  let x = dragged.position.x;
  let y = dragged.position.y;
  let bestDX = SNAP_PX + 1;
  let bestDY = SNAP_PX + 1;
  let vLine: { x: number; y1: number; y2: number } | undefined;
  let hLine: { y: number; x1: number; x2: number } | undefined;

  for (const o of others) {
    if (o.id === dragged.id) continue;
    const ow = o.width ?? NODE_W;
    const oh = o.height ?? NODE_H;
    // Vertical guide: align dragged's left / center / right edge to o's.
    for (const ox of [o.position.x, o.position.x + ow / 2, o.position.x + ow]) {
      for (let i = 0; i < 3; i++) {
        const anchor = x + (i === 0 ? 0 : i === 1 ? dw / 2 : dw);
        const d = Math.abs(ox - anchor);
        if (d <= SNAP_PX && d < bestDX) {
          bestDX = d;
          x = ox - (i === 0 ? 0 : i === 1 ? dw / 2 : dw);
          vLine = {
            x: ox,
            y1: Math.min(y, o.position.y),
            y2: Math.max(y + dh, o.position.y + oh),
          };
        }
      }
    }
    // Horizontal guide: align dragged's top / center / bottom edge to o's.
    for (const oy of [o.position.y, o.position.y + oh / 2, o.position.y + oh]) {
      for (let i = 0; i < 3; i++) {
        const anchor = y + (i === 0 ? 0 : i === 1 ? dh / 2 : dh);
        const d = Math.abs(oy - anchor);
        if (d <= SNAP_PX && d < bestDY) {
          bestDY = d;
          y = oy - (i === 0 ? 0 : i === 1 ? dh / 2 : dh);
          hLine = {
            y: oy,
            x1: Math.min(x, o.position.x),
            x2: Math.max(x + dw, o.position.x + ow),
          };
        }
      }
    }
  }
  return { position: { x, y }, guides: { hLine, vLine } as GuideLine };
}

// GuideLineOverlay draws the active snap guides as 1px lines. Flow
// coordinates are converted with the live viewport transform so the lines
// track pan/zoom. Rendered as a sibling overlay (not a ReactFlow child) to
// avoid depending on where ReactFlow mounts arbitrary children.
function GuideLineOverlay({ guides }: { guides: GuideLine }) {
  const transform = useStore((s) => s.transform);
  const { hLine, vLine } = guides;
  if (!hLine && !vLine) return null;
  const [tx, ty, zoom] = transform;
  return (
    <div className="pointer-events-none absolute inset-0 z-10">
      {vLine && (
        <div
          className="absolute bg-primary/70"
          style={{
            left: vLine.x * zoom + tx,
            top: vLine.y1 * zoom + ty,
            width: 1,
            height: (vLine.y2 - vLine.y1) * zoom,
          }}
        />
      )}
      {hLine && (
        <div
          className="absolute bg-primary/70"
          style={{
            left: hLine.x1 * zoom + tx,
            top: hLine.y * zoom + ty,
            height: 1,
            width: (hLine.x2 - hLine.x1) * zoom,
          }}
        />
      )}
    </div>
  );
}

// validateGraph returns per-node problems shown as red badges on the canvas.
// Save still succeeds (with a warning); running the agent is blocked instead.
function validateGraph(
  nodes: Node[],
  edges: Edge[],
  t: (key: string) => string
): Record<string, string[]> {
  const issues: Record<string, string[]> = {};
  const push = (id: string, msg: string) => {
    (issues[id] ||= []).push(msg);
  };
  const connected = new Set<string>();
  edges.forEach((e) => {
    connected.add(e.source);
    connected.add(e.target);
  });
  for (const n of nodes) {
    // Notes are documentation, not flow steps: an unconnected note is fine.
    if (n.type === "note") continue;
    if (!connected.has(n.id)) push(n.id, t("agent.issue_disconnected"));
    const d = n.data as Record<string, unknown>;
    if (n.type === "llm" && !String(d.system_prompt ?? "").trim()) push(n.id, t("agent.issue_no_prompt"));
    if (n.type === "message" && !String(d.text ?? "").trim()) push(n.id, t("agent.issue_no_text"));
    if (n.type === "condition" && !String(d.value ?? "").trim()) push(n.id, t("agent.issue_no_value"));
    if (n.type === "classifier" && normalizeCategories(d.categories).length === 0) {
      push(n.id, t("agent.issue_no_categories"));
    }
  }
  return issues;
}

function AgentCanvas() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const t = useTranslations();

  const { data: agent, mutate } = useSWR(`agent-${kbId}`, () => api.getAgent(kbId));

  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const { fitView } = useReactFlow();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [runOpen, setRunOpen] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [openingMessage, setOpeningMessage] = useState("");
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [traceNodes, setTraceNodes] = useState<Set<string>>(new Set());
  const [traceEdges, setTraceEdges] = useState<Set<string>>(new Set());
  // node_id -> { status, ms } runtime badge filled in from trace steps.
  const [nodeRuntime, setNodeRuntime] = useState<Record<string, { status: string; ms: number }>>({});

  // Undo/redo: snapshots of {nodes, edges} taken before structural changes
  // (add/delete node, connect/reconnect/delete edge, template, auto-layout,
  // node drag). Panel field edits are intentionally not snapshotted — they
  // are per-keystroke and would flood the stack.
  const pastRef = useRef<{ nodes: Node[]; edges: Edge[] }[]>([]);
  const futureRef = useRef<{ nodes: Node[]; edges: Edge[] }[]>([]);
  // Bumped on every stack change so toolbar buttons re-render.
  const [histVer, setHistVer] = useState(0);

  const deepCopy = (v: unknown) => JSON.parse(JSON.stringify(v));

  const takeSnapshot = useCallback(() => {
    pastRef.current.push({ nodes: deepCopy(nodes) as Node[], edges: deepCopy(edges) as Edge[] });
    if (pastRef.current.length > 50) pastRef.current.shift();
    futureRef.current = [];
    setHistVer((v) => v + 1);
  }, [nodes, edges]);

  const undo = useCallback(() => {
    const prev = pastRef.current.pop();
    if (!prev) return;
    futureRef.current.push({ nodes: deepCopy(nodes) as Node[], edges: deepCopy(edges) as Edge[] });
    setNodes(prev.nodes);
    setEdges(prev.edges);
    setDirty(true);
    setHistVer((v) => v + 1);
  }, [nodes, edges, setNodes, setEdges]);

  const redo = useCallback(() => {
    const next = futureRef.current.pop();
    if (!next) return;
    pastRef.current.push({ nodes: deepCopy(nodes) as Node[], edges: deepCopy(edges) as Edge[] });
    setNodes(next.nodes);
    setEdges(next.edges);
    setDirty(true);
    setHistVer((v) => v + 1);
  }, [nodes, edges, setNodes, setEdges]);

  // Stack lengths are read during render; histVer's changes trigger the
  // re-render that picks them up.
  const canUndo = histVer >= 0 && pastRef.current.length > 0;
  const canRedo = histVer >= 0 && futureRef.current.length > 0;

  // Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y, ignored while typing in form controls.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT" || target.isContentEditable)) return;
      const mod = e.ctrlKey || e.metaKey;
      if (!mod) return;
      if (e.key.toLowerCase() === "z" && !e.shiftKey) { e.preventDefault(); undo(); }
      else if ((e.key.toLowerCase() === "z" && e.shiftKey) || e.key.toLowerCase() === "y") { e.preventDefault(); redo(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [undo, redo]);

  useEffect(() => {
    if (!agent || loaded) return;
    const { nodes: fn, edges: fe } = toFlow(agent.definition);
    setNodes(fn);
    setEdges(fe);
    setOpeningMessage(agent.definition?.opening_message || "");
    setSuggestedQuestions(agent.definition?.suggested_questions || []);
    setLoaded(true);
  }, [agent, loaded, setNodes, setEdges]);

  const selectedNode = nodes.find((n) => n.id === selectedId) || null;
  const selectedEdge = edges.find((e) => e.id === selectedEdgeId) || null;
  const selectedEdgeSource = selectedEdge
    ? nodes.find((n) => n.id === selectedEdge.source)
    : undefined;
  const selectedEdgeSourceType = selectedEdgeSource?.type;
  const selectedEdgeSourceCategories =
    selectedEdgeSource?.type === "classifier"
      ? normalizeCategories(selectedEdgeSource.data.categories)
      : undefined;

  const onNodeDataChange = useCallback(
    (id: string, data: Record<string, unknown>) => {
      setNodes((nds) =>
        nds.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...data } } : n))
      );

      // When a classifier's categories change, auto-sync outgoing edge
      // labels so renaming a category updates the corresponding branch.
      // Logic: old labels that disappeared from the new category list are
      // matched to new categories that weren't in the old list (a rename).
      if (data.categories !== undefined) {
        const oldNode = nodes.find((n) => n.id === id);
        if (oldNode?.type === "classifier") {
          const oldCats = normalizeCategories(oldNode.data.categories).map((c) => c.name);
          const newCats = normalizeCategories(data.categories).map((c) => c.name);
          const removed = oldCats.filter((c) => !newCats.includes(c));
          const added = newCats.filter((c) => !oldCats.includes(c));
          if (removed.length > 0 && added.length > 0) {
            const addedCopy = [...added];
            setEdges((eds) =>
              eds.map((e) => {
                if (e.source !== id) return e;
                const idx = removed.indexOf(typeof e.label === "string" ? e.label : "");
                if (idx >= 0 && addedCopy.length > 0) {
                  const newLabel = addedCopy.shift()!;
                  return { ...e, label: newLabel };
                }
                return e;
              })
            );
          }
        }
      }

      setDirty(true);
    },
    [setNodes, setEdges, nodes]
  );

  const onConnect = useCallback(
    (conn: Connection) => {
      takeSnapshot();
      const srcNode = nodes.find((n) => n.id === conn.source);
      setEdges((eds) => {
        let label: string | undefined;
        if (srcNode?.type === "condition") {
          const cnt = eds.filter((e) => e.source === conn.source).length;
          label = cnt === 0 ? t("agent.branch_true") : t("agent.branch_false");
        } else if (srcNode?.type === "classifier") {
          // Auto-assign the first category not yet used by a sibling edge so
          // new branches are routable without manual label editing.
          const cats = normalizeCategories(srcNode.data.categories)
            .map((c) => c.name.trim())
            .filter(Boolean);
          const used = new Set(
            eds.filter((e) => e.source === conn.source).map((e) => String(e.label ?? ""))
          );
          label = cats.find((c) => !used.has(c)) ?? `${t("agent.branch_category")} ${eds.filter((e) => e.source === conn.source).length + 1}`;
        }
        return addEdge({ ...conn, id: `e-${conn.source}-${conn.target}`, label }, eds);
      });
      setDirty(true);
    },
    [setEdges, nodes, t, takeSnapshot]
  );

  // Reconnect an existing edge by dragging its endpoint to another node.
  const onEdgeUpdate = useCallback(
    (oldEdge: Edge, newConnection: Connection) => {
      takeSnapshot();
      setEdges((eds) => updateEdge(oldEdge, newConnection, eds));
      setDirty(true);
    },
    [setEdges, takeSnapshot]
  );

  const onEdgeLabelChange = useCallback(
    (id: string, label: string) => {
      takeSnapshot();
      setEdges((eds) => eds.map((e) => (e.id === id ? { ...e, label } : e)));
      setDirty(true);
    },
    [setEdges, takeSnapshot]
  );

  // Insert a node of `type` in the middle of an edge (edge "+" button): the
  // original edge is replaced by source→new + new→target, keeping the source
  // branch label so condition/classifier routing stays intact.
  const insertOnEdge = useCallback(
    (edgeId: string, type: string) => {
      const edge = edges.find((e) => e.id === edgeId);
      if (!edge) return;
      const src = nodes.find((n) => n.id === edge.source);
      const dst = nodes.find((n) => n.id === edge.target);
      takeSnapshot();
      const id = `n${Date.now()}`;
      setNodes((nds) => [
        ...nds,
        {
          id,
          type,
          position: {
            x: src && dst ? (src.position.x + dst.position.x) / 2 : 200,
            y: src && dst ? (src.position.y + dst.position.y) / 2 : 150,
          },
          data: { ...DEFAULT_NODE_DATA[type], slug: nextSlug(nds, type) },
        },
      ]);
      setEdges((eds) => [
        ...eds.filter((e) => e.id !== edgeId),
        { id: `e-${edge.source}-${id}`, source: edge.source, target: id, type: "labeled", label: edge.label },
        { id: `e-${id}-${edge.target}`, source: id, target: edge.target, type: "labeled" },
      ]);
      setSelectedId(id);
      setSelectedEdgeId(null);
      setDirty(true);
    },
    [edges, nodes, setNodes, setEdges, takeSnapshot]
  );

  // Drag alignment: snap the dragged node to guides from other nodes and
  // show the matching 1px reference lines; cleared when the drag ends.
  const [guides, setGuides] = useState<GuideLine>({});
  const onNodeDrag = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const { position, guides: g } = snapToGuides(
        node,
        nodes.filter((n) => n.id !== node.id)
      );
      setGuides(g);
      if (position.x !== node.position.x || position.y !== node.position.y) {
        setNodes((nds) => nds.map((n) => (n.id === node.id ? { ...n, position } : n)));
      }
    },
    [nodes, setNodes]
  );

  function addNode(type: string) {
    takeSnapshot();
    const id = `n${Date.now()}`;
    const newNode: Node = {
      id,
      type,
      position: { x: 200 + Math.random() * 100, y: 150 + Math.random() * 100 },
      // Notes are documentation: no slug, no execution, no variables.
      data: type === "note" ? { ...DEFAULT_NODE_DATA.note } : { ...DEFAULT_NODE_DATA[type], slug: nextSlug(nodes, type) },
    };
    setNodes((nds) => [...nds, newNode]);
    setSelectedId(id);
    setDirty(true);
    setPaletteOpen(false);
  }

  // applyTemplate replaces the canvas with a predefined graph; positions are
  // assigned by dagre so both templates render already tidied up.
  // minimal: retrieval → llm (2 nodes).
  // standard: classifier → retrieval → condition → llm/message (5 nodes).
  function applyTemplate(template: "minimal" | "standard") {
    takeSnapshot();
    const now = Date.now();
    let templateNodes: Node[];
    let templateEdges: Edge[];
    if (template === "minimal") {
      const rId = `n${now}`;
      const lId = `n${now + 1}`;
      templateNodes = [
        { id: rId, type: "retrieval", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.retrieval, slug: "retrieval_1" } },
        { id: lId, type: "llm", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.llm, slug: "llm_1" } },
      ];
      templateEdges = [{ id: `e-${rId}-${lId}`, source: rId, target: lId }];
    } else {
      // Teaching template: every node type plays to its strength.
      //   classifier → intent routing (doc Q&A vs chitchat — judgeable from
      //                the query itself, no guessing about KB contents)
      //   retrieval  → recall
      //   condition  → relevance gate on the best score (data-driven, unlike
      //                hit_count which is almost always > 0)
      //   llm ×2     → grounded answer / free chat
      //   message    → fixed fallback reply
      const cId = `n${now}`;
      const rId = `n${now + 1}`;
      const cdId = `n${now + 2}`;
      const lId = `n${now + 3}`;
      const mId = `n${now + 4}`;
      const l2Id = `n${now + 5}`;
      templateNodes = [
        { id: cId, type: "classifier", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.classifier, slug: "classifier_1", categories: [
          { name: t("agent.cat_kb"), description: t("agent.cat_kb_desc") },
          { name: t("agent.cat_chat"), description: t("agent.cat_chat_desc") },
        ] } },
        { id: rId, type: "retrieval", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.retrieval, slug: "retrieval_1" } },
        { id: cdId, type: "condition", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.condition, variable: "retrieval_1.top_score", operator: ">", value: "0.35" } },
        { id: lId, type: "llm", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.llm, slug: "llm_1", system_prompt: t("agent.template_rag_prompt") } },
        { id: mId, type: "message", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.message, slug: "message_1", text: t("agent.template_default_fallback") } },
        { id: l2Id, type: "llm", position: { x: 0, y: 0 }, data: { ...DEFAULT_NODE_DATA.llm, slug: "llm_2" } },
      ];
      templateEdges = [
        { id: `e-${cId}-${rId}`, source: cId, target: rId, label: t("agent.cat_kb") },
        { id: `e-${cId}-${l2Id}`, source: cId, target: l2Id, label: t("agent.cat_chat") },
        { id: `e-${rId}-${cdId}`, source: rId, target: cdId },
        { id: `e-${cdId}-${lId}`, source: cdId, target: lId, label: t("agent.branch_true") },
        { id: `e-${cdId}-${mId}`, source: cdId, target: mId, label: t("agent.branch_false") },
      ];
    }
    setNodes(layoutNodes(templateNodes, templateEdges));
    setEdges(templateEdges);
    setSelectedId(null);
    setSelectedEdgeId(null);
    setTraceNodes(new Set());
    setTraceEdges(new Set());
    setNodeRuntime({});
    setOpeningMessage(t("agent.template_opening_message"));
    setDirty(true);
    setTimeout(() => fitView({ padding: 0.2 }), 60);
  }

  async function save(): Promise<boolean> {
    setSaving(true);
    const issueCount = Object.keys(issues).length;
    try {
      await api.saveAgent(kbId, fromFlow(nodes, edges, openingMessage, suggestedQuestions));
      mutate();
      setDirty(false);
      if (issueCount > 0) toast.warning(t("agent.validate_warn", { count: issueCount }));
      else toast.success(t("toast.saved"));
      return true;
    } catch (e) {
      toast.error((e as Error).message);
      return false;
    } finally {
      setSaving(false);
    }
  }

  // Run opens the chat drawer for quick testing. Persist first only when the
  // canvas has unsaved config changes so an untouched agent doesn't toast.
  // Config problems block the run so test results are meaningful.
  async function run() {
    const issueCount = Object.keys(issues).length;
    if (issueCount > 0) {
      toast.error(t("agent.validate_block", { count: issueCount }));
      return;
    }
    if (dirty) {
      const ok = await save();
      if (!ok) return;
    }
    setRunOpen(true);
  }

  // Validation problems recomputed live as nodes/edges change.
  const issues = useMemo(() => validateGraph(nodes, edges, t), [nodes, edges, t]);

  // Derive display nodes/edges with trace highlight, runtime badges, and
  // validation problems applied. Extras live only on the rendered copies so
  // the underlying nodes state stays save-clean.
  const displayNodes = useMemo(() => {
    const hasExtras =
      traceNodes.size > 0 || Object.keys(nodeRuntime).length > 0 || Object.keys(issues).length > 0;
    if (!hasExtras) return nodes;
    return nodes.map((n) => ({
      ...n,
      className: traceNodes.has(n.id) ? "agent-trace-active" : undefined,
      data: {
        ...n.data,
        __runtime: nodeRuntime[n.id],
        __issues: issues[n.id],
      },
    }));
  }, [nodes, traceNodes, nodeRuntime, issues]);

  // autoLayout re-arranges the canvas with dagre and frames the result.
  const autoLayout = useCallback(() => {
    takeSnapshot();
    setNodes((nds) => layoutNodes(nds, edges));
    setDirty(true);
    setTimeout(() => fitView({ padding: 0.2 }), 60);
  }, [edges, setNodes, fitView, takeSnapshot]);

  const displayEdges = useMemo(() => {
    // onInsertNode is render-time only (edge "+" menu); fromFlow saves the
    // raw edges state, so callbacks never reach the persisted definition.
    return edges.map((e) => ({
      ...e,
      animated: traceEdges.has(e.id),
      className: traceEdges.has(e.id) ? "agent-trace-edge" : undefined,
      data: { onInsertNode: (type: string) => insertOnEdge(e.id, type) },
    }));
  }, [edges, traceEdges, insertOnEdge]);

  return (
    <div>
      <Link
        href={`/dashboard/knowledge-bases/${kbId}`}
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4"
      >
        <ArrowLeft className="h-4 w-4 mr-1" /> {t("kb.back_kb")}
      </Link>

      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("agent.title")}</h1>
          {agent && (
            <p className="text-sm text-muted-foreground mt-1">
              {t("agent.version")} {agent.version} · {t("agent.saved_label")} {new Date(agent.updated_at).toLocaleString()}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Button onClick={save} disabled={saving || !loaded || !dirty}>
            <Save className="h-4 w-4 mr-1" />
            {saving ? t("agent.saving") : t("agent.save_button")}
          </Button>
          <Button variant="outline" onClick={run} disabled={!loaded || saving}>
            <Play className="h-4 w-4 mr-1" />
            {t("agent.run_button")}
          </Button>
        </div>
      </div>

      <div className="flex gap-4">
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card>
            <CardContent className="p-0" style={{ overflow: "hidden" }}>
              {/* Node palette toolbar; wraps on narrow viewports instead of
                  overflowing the card. */}
              <div className="relative border-b px-3 py-2 flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setPaletteOpen((v) => !v)}
                >
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  {t("agent.add_node")}
                </Button>
                {/* Sticky note: canvas documentation, deliberately kept out of
                    the node palette — it is not an executable node. */}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => addNode("note")}
                >
                  <StickyNote className="h-3.5 w-3.5 mr-1" />
                  {t("agent.node_note")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={autoLayout}
                  disabled={nodes.length === 0}
                >
                  <LayoutGrid className="h-3.5 w-3.5 mr-1" />
                  {t("agent.auto_layout")}
                </Button>
                <Button
                  size="icon"
                  variant="outline"
                  className="h-7 w-7"
                  onClick={undo}
                  disabled={!canUndo}
                  aria-label={t("agent.undo")}
                  title={t("agent.undo")}
                >
                  <Undo2 className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="icon"
                  variant="outline"
                  className="h-7 w-7"
                  onClick={redo}
                  disabled={!canRedo}
                  aria-label={t("agent.redo")}
                  title={t("agent.redo")}
                >
                  <Redo2 className="h-3.5 w-3.5" />
                </Button>
                {paletteOpen && (
                  <div className="absolute top-full left-3 mt-1 z-10 rounded-md border border-border bg-popover shadow-md py-1 w-40">
                    {NODE_PALETTE.map((item) => (
                      <button
                        key={item.type}
                        onClick={() => addNode(item.type)}
                        className="flex w-full items-center gap-2 text-left px-3 py-1.5 text-sm hover:bg-accent transition-colors"
                      >
                        <span className={item.color}>{item.icon}</span>
                        {t(item.labelKey)}
                      </button>
                    ))}
                  </div>
                )}

                {/* Canvas templates */}
                <div className="ml-auto flex items-center gap-1.5">
                  <span className="text-xs text-muted-foreground mr-1">{t("agent.template")}:</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => applyTemplate("minimal")}
                  >
                    {t("agent.template_minimal")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => applyTemplate("standard")}
                  >
                    {t("agent.template_standard")}
                  </Button>
                </div>
              </div>
              <div style={{ width: "100%", height: 520, position: "relative" }}>
                <ReactFlow
                  nodes={displayNodes}
                  edges={displayEdges}
                  onNodesChange={onNodesChange}
                  onEdgesChange={onEdgesChange}
                  onNodeClick={(_, n) => { setSelectedId(n.id); setSelectedEdgeId(null); }}
                  onEdgeClick={(_, e) => { setSelectedEdgeId(e.id); setSelectedId(null); }}
                  onPaneClick={() => { setSelectedId(null); setSelectedEdgeId(null); setPaletteOpen(false); }}
                  onConnect={onConnect}
                  onEdgeUpdate={onEdgeUpdate}
                  onNodeDragStart={() => takeSnapshot()}
                  onNodeDrag={onNodeDrag}
                  onNodeDragStop={() => setGuides({})}
                  onNodesDelete={() => { takeSnapshot(); setDirty(true); }}
                  onEdgesDelete={() => { takeSnapshot(); setDirty(true); }}
                  nodeTypes={agentNodeTypes}
                  edgeTypes={agentEdgeTypes}
                  nodesDraggable
                  nodesConnectable
                  edgesUpdatable
                  defaultEdgeOptions={{
                    type: "labeled",
                    markerEnd: { type: MarkerType.ArrowClosed },
                    updatable: true,
                    deletable: true,
                  }}
                  deleteKeyCode={["Backspace", "Delete"]}
                  fitView
                  proOptions={{ hideAttribution: true }}
                >
                  <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
                  <Controls showInteractive={false} />
                </ReactFlow>
                <GuideLineOverlay guides={guides} />
              </div>
            </CardContent>
          </Card>
        </div>

        <div style={{ width: 280, flexShrink: 0 }}>
          <Card className="flex flex-col" style={{ height: 560 }}>
            <CardHeader className="shrink-0">
              <CardTitle className="text-sm">{t("agent.config")}</CardTitle>
            </CardHeader>
            {/* overflow-x-hidden: the 280px panel wraps all content; any
                unexpected intrinsic width must never surface a scrollbar. */}
            <CardContent className="flex-1 overflow-y-auto overflow-x-hidden">
              <AgentConfigPanel
                node={selectedNode as AgentNode | null}
                edge={selectedEdge as AgentEdge | null}
                sourceNodeType={selectedEdgeSourceType}
                sourceNodeCategories={selectedEdgeSourceCategories}
                onChange={onNodeDataChange}
                onEdgeLabelChange={onEdgeLabelChange}
                openingMessage={openingMessage}
                onOpeningMessageChange={(v) => { setOpeningMessage(v); setDirty(true); }}
                suggestedQuestions={suggestedQuestions}
                onSuggestedQuestionsChange={(v) => { setSuggestedQuestions(v); setDirty(true); }}
                allNodes={nodes as unknown as AgentNode[]}
                allEdges={edges as unknown as AgentEdge[]}
                kbId={kbId}
              />
            </CardContent>
          </Card>
        </div>
      </div>

      {runOpen && <AgentChatDrawer kbId={kbId} onClose={() => {
        setRunOpen(false);
        setTraceNodes(new Set());
        setTraceEdges(new Set());
        setNodeRuntime({});
      }} onSendStart={() => {
        setTraceNodes(new Set());
        setTraceEdges(new Set());
        setNodeRuntime({});
      }} onTraceStep={(step) => {
        if (step.node_id) {
          setTraceNodes((prev) => new Set(prev).add(step.node_id!));
          if (step.status) {
            setNodeRuntime((prev) => ({
              ...prev,
              [step.node_id!]: { status: step.status!, ms: step.duration_ms ?? 0 },
            }));
          }
        }
        if (step.edge_id) setTraceEdges((prev) => new Set(prev).add(step.edge_id!));
      }} />}
    </div>
  );
}
