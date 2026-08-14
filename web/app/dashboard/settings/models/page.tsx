"use client";

import { useState } from "react";
import { api, EmbeddingModel, LLMModel, RerankModel } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { ModelSection, ModelSectionConfig } from "@/components/settings/model-section";

const PROVIDERS = ["openai", "anthropic", "deepseek", "zhipu", "ollama", "custom"];

const PROVIDER_DEFAULTS: Record<string, { endpoint: string; model: string }> = {
  openai: { endpoint: "https://api.openai.com/v1", model: "gpt-4o-mini" },
  anthropic: { endpoint: "https://api.anthropic.com/v1", model: "claude-3-5-sonnet-latest" },
  deepseek: { endpoint: "https://api.deepseek.com/v1", model: "deepseek-chat" },
  zhipu: { endpoint: "https://open.bigmodel.cn/api/paas/v4", model: "glm-5.2" },
  ollama: { endpoint: "http://localhost:11434/v1", model: "llama3" },
  custom: { endpoint: "", model: "" },
};

const EMBEDDING_PRESETS: Record<string, { endpoint: string; model: string }> = {
  bge_large_zh: { endpoint: "http://localhost:9997", model: "bge-large-zh-v1.5" },
  bge_m3: { endpoint: "http://localhost:9997", model: "bge-m3" },
  openai: { endpoint: "https://api.openai.com/v1", model: "text-embedding-3-small" },
  zhipu: { endpoint: "https://open.bigmodel.cn/api/paas/v4", model: "embedding-3" },
  custom: { endpoint: "", model: "" },
};

const RERANK_PRESETS: Record<string, { endpoint: string; model: string }> = {
  bge_reranker_v2_m3: { endpoint: "https://api.siliconflow.cn", model: "BAAI/bge-reranker-v2-m3" },
  bge_reranker_v2_gemma: { endpoint: "https://api.siliconflow.cn", model: "BAAI/bge-reranker-v2-gemma" },
  cohere: { endpoint: "https://api.cohere.ai", model: "rerank-multilingual-v3.0" },
  jina: { endpoint: "https://api.jina.ai/v1", model: "jina-reranker-v2-base-multilingual" },
  custom: { endpoint: "", model: "" },
};

const llmConfig: ModelSectionConfig<LLMModel> = {
  swrKey: "llm-list",
  listFn: () => api.listLLMs(1, 50),
  createFn: (body) => api.createLLM(body),
  updateFn: (id, body) => api.updateLLM(id, body),
  deleteFn: (id) => api.deleteLLM(id),
  testFn: (id) => api.testLLM(id),
  sectionTitleKey: "settings.section_llm",
  namespace: "llm",
  defaultForm: {
    provider: "openai",
    endpoint: PROVIDER_DEFAULTS.openai.endpoint,
    model: PROVIDER_DEFAULTS.openai.model,
    temperature: 0.7,
    max_tokens: 2048,
    top_p: 1,
    is_default: false,
  },
  toEditForm: (p) => ({
    name: p.name,
    provider: p.provider,
    endpoint: p.endpoint,
    model: p.model,
    api_key: "",
    temperature: p.temperature,
    max_tokens: p.max_tokens,
    top_p: p.top_p,
    is_default: p.is_default,
  }),
  buildCreateBody: (form) => ({
    name: form.name,
    provider: form.provider,
    endpoint: form.endpoint,
    model: form.model,
    api_key: form.api_key,
    temperature: form.temperature,
    max_tokens: form.max_tokens,
    top_p: form.top_p,
    is_default: form.is_default,
  } as Partial<LLMModel>),
  buildUpdateBody: (form) => {
    const body: Record<string, unknown> = {
      name: form.name,
      provider: form.provider,
      endpoint: form.endpoint,
      model: form.model,
      temperature: form.temperature,
      max_tokens: form.max_tokens,
      top_p: form.top_p,
      is_default: form.is_default,
    };
    if (form.api_key) body.api_key = form.api_key;
    return body;
  },
  fields: [
    { key: "name", labelKey: "llm.name", type: "text" },
    {
      key: "provider",
      labelKey: "llm.provider",
      type: "select",
      options: PROVIDERS,
      onOptionChange: (option) => {
        const d = PROVIDER_DEFAULTS[option];
        return { provider: option, endpoint: d?.endpoint ?? "", model: d?.model ?? "" };
      },
    },
    { key: "endpoint", labelKey: "llm.endpoint", type: "text" },
    { key: "model", labelKey: "llm.model", type: "text" },
    { key: "api_key", labelKey: "llm.api_key", type: "password" },
    { key: "is_default", labelKey: "llm.set_default", type: "checkbox" },
    { key: "temperature", labelKey: "llm.temperature", type: "number", parse: (v) => Number(v) },
    { key: "max_tokens", labelKey: "llm.max_tokens", type: "number", parse: (v) => Number(v) },
  ],
  renderSecondary: (p) => `${p.provider} · ${p.model} · ${p.endpoint}`,
};

const embeddingConfig: ModelSectionConfig<EmbeddingModel> = {
  swrKey: "embedding-list",
  listFn: () => api.listEmbeddings(1, 50),
  createFn: (body) => api.createEmbedding(body),
  updateFn: (id, body) => api.updateEmbedding(id, body),
  deleteFn: (id) => api.deleteEmbedding(id),
  testFn: (id) => api.testEmbedding(id),
  sectionTitleKey: "settings.section_embedding",
  namespace: "embedding",
  defaultForm: {
    preset: "bge_large_zh",
    endpoint: EMBEDDING_PRESETS.bge_large_zh.endpoint,
    model: EMBEDDING_PRESETS.bge_large_zh.model,
    batch_size: 32,
    is_default: false,
  },
  toEditForm: (p) => ({
    name: p.name,
    preset: "custom",
    endpoint: p.endpoint,
    model: p.model,
    api_key: "",
    batch_size: p.batch_size,
    is_default: p.is_default,
  }),
  buildCreateBody: (form) => ({
    name: form.name,
    endpoint: form.endpoint,
    model: form.model,
    api_key: form.api_key,
    batch_size: form.batch_size,
    is_default: form.is_default,
  } as Partial<EmbeddingModel>),
  buildUpdateBody: (form) => {
    const body: Record<string, unknown> = {
      name: form.name,
      endpoint: form.endpoint,
      model: form.model,
      batch_size: form.batch_size,
      is_default: form.is_default,
    };
    if (form.api_key) body.api_key = form.api_key;
    return body;
  },
  fields: [
    { key: "name", labelKey: "embedding.name", type: "text" },
    {
      key: "preset",
      labelKey: "llm.provider",
      type: "select",
      options: Object.keys(EMBEDDING_PRESETS),
      onOptionChange: (option, form) => {
        const d = EMBEDDING_PRESETS[option];
        return { ...form, preset: option, endpoint: d?.endpoint ?? "", model: d?.model ?? "" };
      },
    },
    { key: "endpoint", labelKey: "embedding.endpoint", type: "text" },
    { key: "model", labelKey: "embedding.model", type: "text" },
    { key: "api_key", labelKey: "embedding.api_key", type: "password" },
    { key: "batch_size", labelKey: "embedding.batch_size", type: "number", parse: (v) => parseInt(v) || 32 },
    { key: "is_default", labelKey: "embedding.set_default", type: "checkbox" },
  ],
  renderSecondary: (p, t) => `${p.model} · ${t("embedding.dim")}: ${p.dim} · ${p.endpoint}`,
  formatTestReply: (reply, t) => {
    if (reply === "ok") return t("embedding.test_ok");
    if (reply === "empty") return t("embedding.test_empty");
    return reply;
  },
};

const rerankConfig: ModelSectionConfig<RerankModel> = {
  swrKey: "rerank-list",
  listFn: () => api.listReranks(1, 50),
  createFn: (body) => api.createRerank(body),
  updateFn: (id, body) => api.updateRerank(id, body),
  deleteFn: (id) => api.deleteRerank(id),
  testFn: (id) => api.testRerank(id),
  sectionTitleKey: "settings.section_rerank",
  namespace: "rerank",
  defaultForm: {
    preset: "bge_reranker_v2_m3",
    endpoint: RERANK_PRESETS.bge_reranker_v2_m3.endpoint,
    model: RERANK_PRESETS.bge_reranker_v2_m3.model,
    top_n: 10,
    is_default: false,
  },
  toEditForm: (p) => ({
    name: p.name,
    preset: "custom",
    endpoint: p.endpoint,
    model: p.model,
    api_key: "",
    top_n: p.top_n,
    is_default: p.is_default,
  }),
  buildCreateBody: (form) => ({
    name: form.name,
    endpoint: form.endpoint,
    model: form.model,
    api_key: form.api_key,
    top_n: form.top_n,
    is_default: form.is_default,
  } as Partial<RerankModel>),
  buildUpdateBody: (form) => {
    const body: Record<string, unknown> = {
      name: form.name,
      endpoint: form.endpoint,
      model: form.model,
      top_n: form.top_n,
      is_default: form.is_default,
    };
    if (form.api_key) body.api_key = form.api_key;
    return body;
  },
  fields: [
    { key: "name", labelKey: "rerank.name", type: "text" },
    {
      key: "preset",
      labelKey: "llm.provider",
      type: "select",
      options: Object.keys(RERANK_PRESETS),
      onOptionChange: (option, form) => {
        const d = RERANK_PRESETS[option];
        return { ...form, preset: option, endpoint: d?.endpoint ?? "", model: d?.model ?? "" };
      },
    },
    { key: "endpoint", labelKey: "rerank.endpoint", type: "text" },
    { key: "model", labelKey: "rerank.model", type: "text" },
    { key: "api_key", labelKey: "rerank.api_key", type: "password" },
    { key: "top_n", labelKey: "rerank.top_n", type: "number", parse: (v) => parseInt(v) || 10 },
    { key: "is_default", labelKey: "rerank.set_default", type: "checkbox" },
  ],
  renderSecondary: (p) => `${p.model} · ${p.endpoint}`,
  formatTestReply: (reply, t) => {
    if (reply === "ok") return t("rerank.test_ok");
    if (reply === "empty") return t("rerank.test_empty");
    return reply;
  },
};

export default function ModelSettingsPage() {
  const t = useTranslations();
  const [tab, setTab] = useState<"llm" | "embedding" | "rerank">("llm");

  const tabs = [
    { key: "llm" as const, label: t("settings.section_llm") },
    { key: "embedding" as const, label: t("settings.section_embedding") },
    { key: "rerank" as const, label: t("settings.section_rerank") },
  ];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("settings.models_title")}</h1>
      </div>

      <div className="flex gap-1 border-b mb-6">
        {tabs.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setTab(tb.key)}
            className={cn(
              "px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors",
              tab === tb.key
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {tb.label}
          </button>
        ))}
      </div>

      {tab === "llm" && <ModelSection config={llmConfig} first />}
      {tab === "embedding" && <ModelSection config={embeddingConfig} first />}
      {tab === "rerank" && <ModelSection config={rerankConfig} first />}
    </div>
  );
}
