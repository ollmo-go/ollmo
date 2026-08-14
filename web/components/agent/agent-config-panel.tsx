"use client";

import { useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { AgentEdge, AgentNode, LLMModel, Paginated, RerankModel, api } from "@/lib/api";
import { useTranslations } from "next-intl";
import { ChevronDown, Plus, Trash2, Variable } from "lucide-react";

const NODE_TYPE_KEY: Record<string, string> = {
  retrieval: "node_retrieval",
  llm: "node_llm",
  message: "node_message",
  condition: "node_condition",
  classifier: "node_classifier",
};

// Outputs each node type publishes to the variable table. Keys mirror the
// executor's registrations (see internal/agent/executor.go).
const NODE_OUTPUTS: Record<string, { key: string; labelKey: string }[]> = {
  retrieval: [
    { key: "context", labelKey: "agent.var_context" },
    { key: "graph_context", labelKey: "agent.var_graph_context" },
    { key: "hit_count", labelKey: "agent.var_hit_count" },
    { key: "top_score", labelKey: "agent.var_top_score" },
  ],
  classifier: [{ key: "label", labelKey: "agent.var_label" }],
  llm: [{ key: "output", labelKey: "agent.var_output" }],
};

export function AgentConfigPanel({
  node,
  edge,
  sourceNodeType,
  sourceNodeCategories,
  onChange,
  onEdgeLabelChange,
  openingMessage,
  onOpeningMessageChange,
  suggestedQuestions,
  onSuggestedQuestionsChange,
  allNodes,
  allEdges,
}: {
  node: AgentNode | null;
  edge: AgentEdge | null;
  sourceNodeType?: string;
  sourceNodeCategories?: (string | { name?: string })[];
  onChange: (id: string, data: Record<string, unknown>) => void;
  onEdgeLabelChange: (id: string, label: string) => void;
  openingMessage: string;
  onOpeningMessageChange: (v: string) => void;
  suggestedQuestions: string[];
  onSuggestedQuestionsChange: (v: string[]) => void;
  allNodes: AgentNode[];
  allEdges: AgentEdge[];
}) {
  const t = useTranslations();
  const { data: llmsData } = useSWR<Paginated<LLMModel>>("llm-list", () =>
    api.listLLMs(1, 50)
  );
  const llms = llmsData?.items ?? [];
  const { data: reranksData } = useSWR<Paginated<RerankModel>>("rerank-list", () =>
    api.listReranks(1, 50)
  );
  const reranks = reranksData?.items ?? [];

  // Node config takes precedence, then edge config, then agent-level settings.
  if (node) {
    const nodeTypeLabel = NODE_TYPE_KEY[node.type]
      ? t(`agent.${NODE_TYPE_KEY[node.type]}`)
      : node.type;
    return (
      <div className="p-4 space-y-3">
        <div className="text-sm font-medium">
          {nodeTypeLabel} {t("agent.node_suffix")}
          {node.data.slug ? (
            <span className="ml-2 font-mono text-xs text-muted-foreground">{String(node.data.slug)}</span>
          ) : null}
        </div>
        {renderEditor(node, (patch) => onChange(node.id, { ...node.data, ...patch }), t, llms, reranks, allNodes, allEdges)}
      </div>
    );
  }

  if (edge) {
    // "true"/"false" and legacy Chinese spellings match the same branches.
    const isTrue = edge.label === t("agent.branch_true") || edge.label === "true" || edge.label === "条件成立";
    const isFalse = edge.label === t("agent.branch_false") || edge.label === "false" || edge.label === "条件不成立";
    const branchBtn = (active: boolean, label: string, onClick: () => void) => (
      <button
        onClick={onClick}
        className={`flex-1 rounded-md border px-2 py-1.5 text-xs transition-colors ${
          active
            ? "border-primary bg-primary text-primary-foreground"
            : "border-border bg-transparent text-muted-foreground hover:bg-accent"
        }`}
      >
        {label}
      </button>
    );
    const isClassifier = sourceNodeType === "classifier";
    const isCondition = sourceNodeType === "condition";
    // Categories may be {name, description} objects; only the name is a
    // routable label.
    const categories = (sourceNodeCategories ?? [])
      .map((c) => (typeof c === "string" ? c : String((c as { name?: string })?.name ?? "")))
      .filter(Boolean);
    return (
      <div className="p-4 space-y-3">
        <div className="text-sm font-medium">{t("agent.edge_config")}</div>
        {isClassifier ? (
          // Classifier branches must match a category exactly, so the label
          // is picked from the source node's list instead of free typing.
          categories.length > 0 ? (
            <Field label={t("agent.field_edge_label")}>
              <div className="flex flex-wrap gap-1.5">
                {categories.map((cat) => (
                  <button
                    key={cat}
                    onClick={() => onEdgeLabelChange(edge.id, cat)}
                    className={`rounded-full border px-2.5 py-1 text-xs transition-colors ${
                      edge.label === cat
                        ? "border-primary bg-primary text-primary-foreground"
                        : "border-border bg-transparent text-muted-foreground hover:bg-accent"
                    }`}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </Field>
          ) : (
            <p className="text-xs text-muted-foreground">{t("agent.edge_classifier_empty")}</p>
          )
        ) : isCondition ? (
          // Condition edges route on true/false only — same pick-don't-type
          // rule as classifier edges.
          <Field label={t("agent.field_edge_label")}>
            <div className="flex gap-2">
              {branchBtn(isTrue, t("agent.branch_true"), () => onEdgeLabelChange(edge.id, t("agent.branch_true")))}
              {branchBtn(isFalse, t("agent.branch_false"), () => onEdgeLabelChange(edge.id, t("agent.branch_false")))}
            </div>
          </Field>
        ) : (
          <Field label={t("agent.field_edge_label")}>
            <TextInput value={edge.label || ""} onChange={(v) => onEdgeLabelChange(edge.id, v)} />
          </Field>
        )}
        {isClassifier && (
          <p className="text-xs text-muted-foreground">{t("agent.edge_classifier_hint")}</p>
        )}
      </div>
    );
  }

  // Nothing selected: show agent-level settings (opening message + suggested questions).
  return (
    <div className="p-4 space-y-3">
      <div className="text-sm font-medium">{t("agent.general_settings")}</div>
      <Field label={t("agent.field_opening_message")}>
        <TextArea
          value={openingMessage}
          onChange={onOpeningMessageChange}
        />
      </Field>
      <p className="text-xs text-muted-foreground">
        {t("agent.opening_message_hint")}
      </p>
      <Field label={t("agent.field_suggested_questions")}>
        <TextArea
          value={suggestedQuestions.join("\n")}
          onChange={(v) => onSuggestedQuestionsChange(v.split("\n"))}
          rows={4}
        />
      </Field>
      <p className="text-xs text-muted-foreground">
        {t("agent.suggested_questions_hint")}
      </p>
    </div>
  );
}

function renderEditor(
  node: AgentNode,
  patch: (p: Record<string, unknown>) => void,
  t: ReturnType<typeof useTranslations>,
  llms: LLMModel[],
  reranks: RerankModel[],
  allNodes: AgentNode[],
  allEdges: AgentEdge[],
) {
  switch (node.type) {
    case "retrieval":
      return (
        <>
          <Field label={t("agent.field_top_k")}>
            <NumberInput value={Number(node.data.top_k || 10)} onChange={(v) => patch({ top_k: v })} />
          </Field>
          <Toggle label={t("agent.field_rerank")} checked={!!node.data.rerank} onChange={(v) => patch({ rerank: v })} />
          {!!node.data.rerank && (
            <Field label={t("agent.field_rerank_model")}>
              <select
                value={String(node.data.rerank_model_id || "")}
                onChange={(e) => patch({ rerank_model_id: e.target.value })}
                className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
              >
                <option value="">{t("agent.rerank_default")}</option>
                {reranks.filter((p) => p.status === "active").map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <Toggle
            label={t("agent.field_graph_context")}
            checked={!!node.data.use_graph}
            onChange={(v) => patch({ use_graph: v })}
          />
        </>
      );
    case "llm":
      return (
        <>
          <Field label={t("agent.field_llm_model")}>
            <select
              value={String(node.data.llm_model_id || "")}
              onChange={(e) => patch({ llm_model_id: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              <option value="">{t("agent.llm_default")}</option>
              {llms.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </Field>
          <PromptField
            label={t("agent.field_system_prompt")}
            hint={t("agent.system_prompt_hint")}
            value={String(node.data.system_prompt || "")}
            onChange={(v) => patch({ system_prompt: v })}
            nodes={allNodes}
            edges={allEdges}
            nodeId={node.id}
            t={t}
          />
          <Field label={t("agent.field_temperature")}>
            <NumberInput
              value={Number(node.data.temperature ?? 0.7)}
              step={0.1}
              onChange={(v) => patch({ temperature: v })}
            />
          </Field>
          <Field label={t("agent.field_max_tokens")}>
            <NumberInput
              value={Number(node.data.max_tokens || 2048)}
              onChange={(v) => patch({ max_tokens: v })}
            />
          </Field>
          <Field label={t("agent.field_top_p")}>
            <NumberInput
              value={Number(node.data.top_p ?? 0.9)}
              step={0.05}
              onChange={(v) => patch({ top_p: v })}
            />
          </Field>
        </>
      );
    case "message":
      return (
        <PromptField
          label={t("agent.field_message_text")}
          value={String(node.data.text || "")}
          onChange={(v) => patch({ text: v })}
          nodes={allNodes}
          edges={allEdges}
          nodeId={node.id}
          t={t}
          rows={6}
        />
      );
    case "condition": {
      // Variables come from the shared table: the query plus any upstream
      // node output ({slug.key}). The legacy shared hit_count/top_score
      // values stay selectable only when already set on old graphs.
      const upstream = upstreamOf(allNodes, allEdges, node.id);
      const variable = String(node.data.variable || "query");
      const legacy =
        variable === "hit_count" || variable === "top_score"
          ? [{ key: variable, labelKey: variable === "hit_count" ? "agent.var_hit_count" : "agent.var_top_score" }]
          : [];
      return (
        <>
          <Field label={t("agent.field_condition_variable")}>
            <select
              value={variable}
              onChange={(e) => patch({ variable: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              <option value="query">{t("agent.var_query")}</option>
              {legacy.map((o) => (
                <option key={o.key} value={o.key}>
                  {t(o.labelKey)}
                </option>
              ))}
              {upstream.flatMap((n) => {
                const slug = String(n.data?.slug ?? "");
                if (!slug) return [];
                const typeName = t(`agent.${NODE_TYPE_KEY[n.type] ?? ""}`);
                return (NODE_OUTPUTS[n.type] ?? []).map((o) => (
                  <option key={`${slug}.${o.key}`} value={`${slug}.${o.key}`}>
                    {typeName} · {t(o.labelKey)}
                  </option>
                ));
              })}
            </select>
          </Field>
          <Field label={t("agent.field_condition_operator")}>
            <select
              value={String(node.data.operator || ">")}
              onChange={(e) => patch({ operator: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              {[">", ">=", "==", "!=", "<", "<=", "contains"].map((o) => (
                <option key={o} value={o}>
                  {o === "contains" ? t("agent.op_contains") : o}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t("agent.field_condition_value")}>
            <input
              value={String(node.data.value ?? "")}
              onChange={(e) => patch({ value: e.target.value })}
              placeholder={t("agent.condition_value_placeholder")}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            />
          </Field>
          <p className="text-xs text-muted-foreground">
            {t("agent.condition_hint")}
          </p>
        </>
      );
    }
    case "classifier": {
      // Categories carry an optional description the LLM uses as its judging
      // basis; the legacy plain-string form is normalized on read.
      const cats = normalizeCategories(node.data.categories);
      const setCat = (i: number, p: Partial<{ name: string; description: string }>) =>
        patch({ categories: cats.map((c, idx) => (idx === i ? { ...c, ...p } : c)) });
      const delCat = (i: number) => patch({ categories: cats.filter((_, idx) => idx !== i) });
      return (
        <>
          <Field label={t("agent.field_llm_model")}>
            <select
              value={String(node.data.llm_model_id || "")}
              onChange={(e) => patch({ llm_model_id: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              <option value="">{t("agent.llm_default")}</option>
              {llms.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </Field>
          <div className="block space-y-1">
            <span className="text-xs text-muted-foreground">{t("agent.field_classifier_categories")}</span>
            {cats.map((c, i) => (
              <div key={i} className="flex items-start gap-1.5">
                <div className="flex-1 space-y-1">
                  <input
                    type="text"
                    value={c.name}
                    onChange={(e) => setCat(i, { name: e.target.value })}
                    placeholder={t("agent.category_name_placeholder")}
                    className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
                  />
                  <input
                    type="text"
                    value={c.description}
                    onChange={(e) => setCat(i, { description: e.target.value })}
                    placeholder={t("agent.category_desc_placeholder")}
                    className="w-full rounded-md border border-border bg-background px-2 py-1 text-xs text-muted-foreground"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => delCat(i)}
                  className="mt-1.5 text-muted-foreground hover:text-destructive"
                  aria-label={t("common.delete")}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => patch({ categories: [...cats, { name: "", description: "" }] })}
              className="flex items-center gap-1 text-xs text-primary hover:underline"
            >
              <Plus className="h-3 w-3" />
              {t("agent.add_category")}
            </button>
          </div>
          <p className="text-xs text-muted-foreground">
            {t("agent.classifier_hint")}
          </p>
        </>
      );
    }
    default:
      return null;
  }
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

// Classifier categories: {name, description} objects. Reads the legacy
// plain-string array too so existing graphs keep working.
export function normalizeCategories(raw: unknown): { name: string; description: string }[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((c) =>
    typeof c === "string"
      ? { name: c, description: "" }
      : { name: String((c as { name?: string })?.name ?? ""), description: String((c as { description?: string })?.description ?? "") }
  );
}

// upstreamOf returns every node reachable from nodeId by walking edges
// backwards (its transitive ancestors on the canvas).
function upstreamOf(nodes: AgentNode[], edges: AgentEdge[], nodeId: string): AgentNode[] {
  const rev = new Map<string, string[]>();
  for (const e of edges) {
    rev.set(e.target, [...(rev.get(e.target) ?? []), e.source]);
  }
  const seen = new Set<string>([nodeId]);
  const out: AgentNode[] = [];
  const stack = [nodeId];
  while (stack.length) {
    const cur = stack.pop()!;
    for (const src of rev.get(cur) ?? []) {
      if (seen.has(src)) continue;
      seen.add(src);
      stack.push(src);
      const n = nodes.find((x) => x.id === src);
      if (n) out.push(n);
    }
  }
  return out;
}

// PromptField is a TextArea for prompt-like text with an "insert variable"
// picker. Only upstream nodes' outputs are listed; picking one inserts the
// {slug.output} reference at the cursor.
function PromptField({
  label,
  hint,
  value,
  onChange,
  nodes,
  edges,
  nodeId,
  t,
  rows = 10,
}: {
  label: string;
  hint?: string;
  value: string;
  onChange: (v: string) => void;
  nodes: AgentNode[];
  edges: AgentEdge[];
  nodeId: string;
  t: ReturnType<typeof useTranslations>;
  rows?: number;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const btnRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [menuPos, setMenuPos] = useState<{ top: number; left: number } | null>(null);
  const upstream = useMemo(() => upstreamOf(nodes, edges, nodeId), [nodes, edges, nodeId]);

  // The panel body is a scroll container that would clip an absolute popup,
  // so the menu is fixed-positioned from the button's viewport coordinates.
  function toggleMenu() {
    if (open) {
      setOpen(false);
      return;
    }
    const r = btnRef.current?.getBoundingClientRect();
    if (r) {
      const W = 240; // w-60
      const H = 288; // max-h-72
      const left = Math.max(8, Math.min(r.right - W, window.innerWidth - W - 8));
      const top = r.bottom + 6 + H > window.innerHeight
        ? Math.max(8, r.top - H - 6)
        : r.bottom + 6;
      setMenuPos({ top, left });
    }
    setOpen(true);
  }

  function insert(refText: string) {
    const el = ref.current;
    setOpen(false);
    if (!el) {
      onChange(value + refText);
      return;
    }
    const start = el.selectionStart ?? value.length;
    const end = el.selectionEnd ?? value.length;
    const next = value.slice(0, start) + refText + value.slice(end);
    onChange(next);
    requestAnimationFrame(() => {
      el.focus();
      const pos = start + refText.length;
      el.setSelectionRange(pos, pos);
    });
  }

  const varButton = (
    <div className="relative">
      <button
        type="button"
        ref={btnRef}
        onClick={toggleMenu}
        className="flex items-center gap-1 text-xs text-primary hover:underline"
      >
        <Variable className="h-3 w-3" />
        {t("agent.insert_var")}
        <ChevronDown className="h-3 w-3" />
      </button>
      {open && menuPos && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div
            className="fixed z-50 w-60 max-h-72 overflow-auto rounded-md border border-border bg-background p-1 shadow-lg"
            style={{ top: menuPos.top, left: menuPos.left }}
          >
            <button
              type="button"
              onClick={() => insert("{query}")}
              className="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-xs hover:bg-accent"
            >
              <span>{t("agent.var_query")}</span>
              <code className="text-[10px] text-muted-foreground">{"{query}"}</code>
            </button>
            {upstream.map((n) => {
              const slug = String(n.data?.slug ?? "");
              const outputs = NODE_OUTPUTS[n.type] ?? [];
              if (!slug || outputs.length === 0) return null;
              return (
                <div key={n.id} className="mt-1 border-t pt-1 first:mt-0 first:border-t-0 first:pt-0">
                  <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                    {t(`agent.${NODE_TYPE_KEY[n.type] ?? ""}`)} · {slug}
                  </div>
                  {outputs.map((o) => (
                    <button
                      key={o.key}
                      type="button"
                      onClick={() => insert(`{${slug}.${o.key}}`)}
                      className="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-xs hover:bg-accent"
                    >
                      <span>{t(o.labelKey)}</span>
                      <code className="text-[10px] text-muted-foreground">{`{${slug}.${o.key}}`}</code>
                    </button>
                  ))}
                </div>
              );
            })}
            {upstream.length === 0 && (
              <p className="px-2 py-2 text-xs text-muted-foreground">{t("agent.var_no_upstream")}</p>
            )}
          </div>
        </>
      )}
    </div>
  );

  return (
    <div className="block space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        {varButton}
      </div>
      <TextArea value={value} onChange={onChange} rows={rows} textareaRef={ref} />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

function TextArea({
  value,
  onChange,
  rows = 4,
  textareaRef,
}: {
  value: string;
  onChange: (v: string) => void;
  rows?: number;
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>;
}) {
  return (
    <textarea
      ref={textareaRef}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      rows={rows}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm resize-y"
    />
  );
}

function NumberInput({
  value,
  onChange,
  step,
}: {
  value: number;
  onChange: (v: number) => void;
  step?: number;
}) {
  return (
    <input
      type="number"
      value={value}
      step={step || 1}
      onChange={(e) => onChange(Number(e.target.value) || 0)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between cursor-pointer">
      <span className="text-xs text-muted-foreground">{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4"
      />
    </label>
  );
}
