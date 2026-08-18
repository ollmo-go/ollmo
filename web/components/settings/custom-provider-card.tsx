"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { useTranslations } from "next-intl";
import { ModelListEditor } from "./model-list-editor";
import type { DraftModel, ProviderKind } from "./_shared";
import { kindLabel } from "./_shared";

export function CustomProviderCard({
  onDone,
  onCancel,
  existingEndpoints,
}: {
  onDone: () => void;
  onCancel: () => void;
  existingEndpoints?: string[];
}) {
  const t = useTranslations();
  const [name, setName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [chatModels, setChatModels] = useState<DraftModel[]>([]);
  const [embedModels, setEmbedModels] = useState<DraftModel[]>([]);
  const [rerankModels, setRerankModels] = useState<DraftModel[]>([]);
  const [busy, setBusy] = useState(false);

  async function create() {
    setBusy(true);
    try {
      await api.providers.create({
        catalog_id: "custom",
        name: name.trim() || undefined,
        endpoint,
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
        chat_models: chatModels
          .filter((m) => m.model.trim())
          .map((m) => ({
            model: m.model,
            name: m.name,
            context_length: m.context_length,
            max_tokens: m.max_tokens,
            input_price: m.input_price,
            output_price: m.output_price,
          })),
        embed_models: embedModels
          .filter((m) => m.model.trim())
          .map((m) => ({ model: m.model, name: m.name, context_length: m.context_length })),
        rerank_models: rerankModels
          .filter((m) => m.model.trim())
          .map((m) => ({ model: m.model, name: m.name, context_length: m.context_length })),
      });
      onDone();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const allModels = [...chatModels, ...embedModels, ...rerankModels];
  const endpointDup =
    endpoint.trim().length > 0 && (existingEndpoints ?? []).includes(endpoint.trim());
  const ready =
    endpoint.trim().length > 0 &&
    !endpointDup &&
    allModels.some((m) => m.model.trim());

  const probeBase = { endpoint: endpoint.trim(), api_key: apiKey.trim() };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_name")}</div>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("settings.provider_name")} />
      </div>
      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_endpoint")}</div>
        <Input
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          placeholder="https://gateway.example/v1"
        />
        {endpointDup && (
          <p className="text-xs text-destructive">{t("settings.provider_endpoint_exists")}</p>
        )}
      </div>
      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_key_input")}</div>
        <Input
          type="password"
          autoComplete="off"
          placeholder={t("llm.api_key")}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />
      </div>

      {(["llm", "embedding", "rerank"] as ProviderKind[]).map((kind) => {
        const models =
          kind === "llm" ? chatModels : kind === "embedding" ? embedModels : rerankModels;
        const setter =
          kind === "llm" ? setChatModels : kind === "embedding" ? setEmbedModels : setRerankModels;
        return (
          <ModelListEditor
            key={kind}
            kind={kind}
            models={models}
            onChange={setter}
            probe={probeBase}
            disabled={busy}
            title={kindLabel(t, kind)}
          />
        );
      })}

      <div className="flex justify-end gap-2 pt-2">
        <Button variant="outline" onClick={onCancel} disabled={busy}>
          {t("common.cancel")}
        </Button>
        <Button onClick={create} disabled={busy || !ready}>
          {busy ? "..." : t("settings.provider_add")}
        </Button>
      </div>
    </div>
  );
}
