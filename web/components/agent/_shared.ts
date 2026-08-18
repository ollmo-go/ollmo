"use client";

import type { AgentEdge, AgentNode } from "@/lib/api";

export const NODE_TYPE_KEY: Record<string, string> = {
  retrieval: "node_retrieval",
  llm: "node_llm",
  message: "node_message",
  condition: "node_condition",
  classifier: "node_classifier",
  note: "node_note",
};

export const NODE_OUTPUTS: Record<string, { key: string; labelKey: string }[]> = {
  retrieval: [
    { key: "context", labelKey: "agent.var_context" },
    { key: "graph_context", labelKey: "agent.var_graph_context" },
    { key: "hit_count", labelKey: "agent.var_hit_count" },
    { key: "top_score", labelKey: "agent.var_top_score" },
  ],
  classifier: [{ key: "label", labelKey: "agent.var_label" }],
  llm: [{ key: "output", labelKey: "agent.var_output" }],
};

export function normalizeCategories(raw: unknown): { name: string; description: string }[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((c) =>
    typeof c === "string"
      ? { name: c, description: "" }
      : { name: String((c as { name?: string })?.name ?? ""), description: String((c as { description?: string })?.description ?? "") }
  );
}

export function upstreamOf(nodes: AgentNode[], edges: AgentEdge[], nodeId: string): AgentNode[] {
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
