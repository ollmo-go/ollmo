"use client";

import { ProviderSection } from "@/components/settings/provider-section";

// Unified model settings page: one provider card manages all model types
// (LLM, Embedding, Rerank) with a single API key.
export default function ModelsPage() {
  return <ProviderSection />;
}
