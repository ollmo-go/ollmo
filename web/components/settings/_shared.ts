import type { ProviderModelRef } from "@/lib/api";

export type ProviderKind = "llm" | "embedding" | "rerank";

export interface DraftModel {
  key: string;
  rowId?: string;
  model: string;
  name: string;
  context_length?: number;
  max_tokens?: number;
  input_price?: number;
  output_price?: number;
  is_default?: boolean;
}

let seq = 0;
export function nextKey(): string {
  seq += 1;
  return `tmp-${seq}`;
}

export function parseCapacity(text: string): number | undefined {
  const s = text.trim().toLowerCase();
  if (!s) return undefined;
  const m = /^(\d+(?:\.\d+)?)([km]?)$/.exec(s);
  if (!m) return undefined;
  const n = Number(m[1]);
  if (!Number.isFinite(n)) return undefined;
  if (m[2] === "k") return Math.round(n * 1000);
  if (m[2] === "m") return Math.round(n * 1000 * 1000);
  return Math.round(n);
}

export function formatCapacity(n?: number): string {
  if (!n) return "";
  if (n >= 1000 * 1000 && n % 1000 === 0) return `${n / (1000 * 1000)}M`;
  if (n >= 1000 && n % 1000 === 0) return `${n / 1000}K`;
  return String(n);
}

export function modelsForKind(
  card: { chat_models?: ProviderModelRef[]; embed_models?: ProviderModelRef[]; rerank_models?: ProviderModelRef[] },
  kind: ProviderKind
): ProviderModelRef[] {
  if (kind === "llm") return card.chat_models ?? [];
  if (kind === "embedding") return card.embed_models ?? [];
  return card.rerank_models ?? [];
}

export function draftFromRef(m: ProviderModelRef): DraftModel {
  return {
    key: m.id,
    rowId: m.id,
    model: m.model,
    name: m.name,
    context_length: m.context_length || undefined,
    max_tokens: m.max_tokens || undefined,
    input_price: m.input_price || undefined,
    output_price: m.output_price || undefined,
    is_default: m.is_default,
  };
}

export function kindLabel(t: (k: string) => string, kind: ProviderKind): string {
  if (kind === "llm") return t("settings.section_llm");
  if (kind === "embedding") return t("settings.section_embedding");
  return t("settings.section_rerank");
}

export type FilterKind = "all" | ProviderKind;
export const FILTERS: FilterKind[] = ["all", "llm", "embedding", "rerank"];

export type CapacityFieldKey = "context_length" | "max_tokens" | "input_price" | "output_price";
