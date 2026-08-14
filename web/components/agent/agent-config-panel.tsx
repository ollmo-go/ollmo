"use client";

import useSWR from "swr";
import { AgentEdge, AgentNode, LLMModel, Paginated, RerankModel, api } from "@/lib/api";
import { useTranslations } from "next-intl";

const NODE_TYPE_KEY: Record<string, string> = {
  retrieval: "node_retrieval",
  llm: "node_llm",
  message: "node_message",
  condition: "node_condition",
  classifier: "node_classifier",
};

export function AgentConfigPanel({
  node,
  edge,
  sourceNodeType,
  onChange,
  onEdgeLabelChange,
  openingMessage,
  onOpeningMessageChange,
  suggestedQuestions,
  onSuggestedQuestionsChange,
}: {
  node: AgentNode | null;
  edge: AgentEdge | null;
  sourceNodeType?: string;
  onChange: (id: string, data: Record<string, unknown>) => void;
  onEdgeLabelChange: (id: string, label: string) => void;
  openingMessage: string;
  onOpeningMessageChange: (v: string) => void;
  suggestedQuestions: string[];
  onSuggestedQuestionsChange: (v: string[]) => void;
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
        </div>
        {renderEditor(node, (patch) => onChange(node.id, { ...node.data, ...patch }), t, llms, reranks)}
      </div>
    );
  }

  if (edge) {
    const isTrue = edge.label === t("agent.branch_true");
    const isFalse = edge.label === t("agent.branch_false");
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
    return (
      <div className="p-4 space-y-3">
        <div className="text-sm font-medium">{t("agent.edge_config")}</div>
        <Field label={t("agent.field_edge_label")}>
          <TextInput value={edge.label || ""} onChange={(v) => onEdgeLabelChange(edge.id, v)} />
        </Field>
        {sourceNodeType === "condition" && (
          <div className="flex gap-2">
            {branchBtn(isTrue, t("agent.branch_true"), () => onEdgeLabelChange(edge.id, t("agent.branch_true")))}
            {branchBtn(isFalse, t("agent.branch_false"), () => onEdgeLabelChange(edge.id, t("agent.branch_false")))}
          </div>
        )}
        {sourceNodeType === "classifier" && (
          <p className="text-xs text-muted-foreground">
            {t("agent.edge_classifier_hint")}
          </p>
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
          <Field label={t("agent.field_system_prompt")}>
            <TextArea
              value={String(node.data.system_prompt || "")}
              onChange={(v) => patch({ system_prompt: v })}
              rows={10}
            />
            <p className="text-xs text-muted-foreground">
              {t("agent.system_prompt_hint")}
            </p>
          </Field>
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
        <Field label={t("agent.field_message_text")}>
          <TextArea
            value={String(node.data.text || "")}
            onChange={(v) => patch({ text: v })}
            rows={6}
          />
        </Field>
      );
    case "condition":
      return (
        <>
          <Field label={t("agent.field_condition_variable")}>
            <select
              value={String(node.data.variable || "hit_count")}
              onChange={(e) => patch({ variable: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              <option value="hit_count">{t("agent.var_hit_count")}</option>
            </select>
          </Field>
          <Field label={t("agent.field_condition_operator")}>
            <select
              value={String(node.data.operator || ">")}
              onChange={(e) => patch({ operator: e.target.value })}
              className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
            >
              <option value=">">{">"}</option>
              <option value=">=">{">="}</option>
              <option value="==">{"=="}</option>
              <option value="<">{"<"}</option>
              <option value="<=">{"<="}</option>
            </select>
          </Field>
          <Field label={t("agent.field_condition_value")}>
            <NumberInput
              value={Number(node.data.value || 0)}
              onChange={(v) => patch({ value: v })}
            />
          </Field>
          <p className="text-xs text-muted-foreground">
            {t("agent.condition_hint")}
          </p>
        </>
      );
    case "classifier":
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
          <Field label={t("agent.field_classifier_categories")}>
            <TextArea
              value={Array.isArray(node.data.categories)
                ? (node.data.categories as string[]).join("\n")
                : ""}
              onChange={(v) => patch({ categories: v.split("\n").filter(Boolean) })}
              rows={6}
            />
          </Field>
          <p className="text-xs text-muted-foreground">
            {t("agent.classifier_hint")}
          </p>
        </>
      );
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

function TextArea({ value, onChange, rows = 4 }: { value: string; onChange: (v: string) => void; rows?: number }) {
  return (
    <textarea
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
