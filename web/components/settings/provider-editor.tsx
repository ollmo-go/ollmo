"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api, ProviderCard, ProviderModelRef } from "@/lib/api";
import { useTranslations } from "next-intl";
import { ModelListEditor } from "./model-list-editor";
import type { DraftModel, ProviderKind } from "./_shared";
import { draftFromRef, kindLabel } from "./_shared";

export function ProviderEditor({
  card,
  testing,
  onTest,
  onClose,
}: {
  card: ProviderCard;
  testing: string | null;
  onTest: (kind: ProviderKind, mid: string, fn: (id: string) => Promise<{ reply: string }>) => void;
  onClose: (changed: boolean) => void;
}) {
  const t = useTranslations();
  const [name, setName] = useState(card.name);
  const [apiKey, setApiKey] = useState("");
  const [endpoint, setEndpoint] = useState(card.endpoint);
  const [chatModels, setChatModels] = useState<DraftModel[]>(() =>
    (card.chat_models ?? []).map(draftFromRef)
  );
  const [embedModels, setEmbedModels] = useState<DraftModel[]>(() =>
    (card.embed_models ?? []).map(draftFromRef)
  );
  const [rerankModels, setRerankModels] = useState<DraftModel[]>(() =>
    (card.rerank_models ?? []).map(draftFromRef)
  );
  const [busy, setBusy] = useState(false);

  async function toggleDefault(kind: ProviderKind, rowId: string, current: boolean) {
    const next = !current;
    try {
      if (kind === "llm") await api.updateLLM(rowId, { is_default: next });
      else if (kind === "embedding") await api.updateEmbedding(rowId, { is_default: next });
      else await api.updateRerank(rowId, { is_default: next });
      const setter =
        kind === "llm" ? setChatModels : kind === "embedding" ? setEmbedModels : setRerankModels;
      setter((ms) =>
        ms.map((m) =>
          m.rowId === rowId
            ? { ...m, is_default: next }
            : next
              ? { ...m, is_default: false }
              : m
        )
      );
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function apply() {
    setBusy(true);
    try {
      const key = apiKey.trim();
      const nameChanged = name.trim() !== "" && name.trim() !== card.name;
      if (nameChanged || key || endpoint !== card.endpoint) {
        await api.providers.update(card.id, {
          ...(nameChanged ? { name: name.trim() } : {}),
          ...(endpoint !== card.endpoint ? { endpoint } : {}),
          ...(key ? { api_key: key } : {}),
        });
      }

      const diffKind = async (
        kind: ProviderKind,
        current: DraftModel[],
        original: ProviderModelRef[]
      ) => {
        const kept = new Set(current.filter((m) => m.rowId).map((m) => m.rowId));
        for (const o of original) {
          if (!kept.has(o.id)) await api.providers.removeModel(card.id, kind, o.id);
        }
        for (const m of current) {
          if (!m.model.trim()) continue;
          if (m.rowId) {
            const o = original.find((x) => x.id === m.rowId);
            const capsChanged =
              kind === "llm" &&
              ((o?.max_tokens ?? undefined) !== (m.max_tokens ?? undefined) ||
                (o?.input_price ?? undefined) !== (m.input_price ?? undefined) ||
                (o?.output_price ?? undefined) !== (m.output_price ?? undefined));
            if (o && (o.name !== m.name || (o.context_length ?? undefined) !== (m.context_length ?? undefined) || capsChanged)) {
              await api.providers.updateModel(card.id, kind, m.rowId, {
                name: m.name,
                context_length: m.context_length,
                ...(kind === "llm"
                  ? {
                      max_tokens: m.max_tokens,
                      input_price: m.input_price,
                      output_price: m.output_price,
                    }
                  : {}),
              });
            }
          } else {
            const ref = await api.providers.addModel(card.id, kind, {
              model: m.model,
              name: m.name,
              context_length: m.context_length,
              ...(kind === "llm"
                ? {
                    max_tokens: m.max_tokens,
                    input_price: m.input_price,
                    output_price: m.output_price,
                  }
                : {}),
            });
            if (m.is_default && ref.id) {
              if (kind === "llm") await api.updateLLM(ref.id, { is_default: true });
              else if (kind === "embedding") await api.updateEmbedding(ref.id, { is_default: true });
              else await api.updateRerank(ref.id, { is_default: true });
            }
          }
        }
      };

      await diffKind("llm", chatModels, card.chat_models ?? []);
      await diffKind("embedding", embedModels, card.embed_models ?? []);
      await diffKind("rerank", rerankModels, card.rerank_models ?? []);
      onClose(true);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const probeBase = { endpoint, api_key: apiKey.trim(), provider_id: card.id };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_name")}</div>
        <Input
          value={name}
          placeholder={t("settings.provider_name_placeholder")}
          onChange={(e) => setName(e.target.value)}
        />
      </div>

      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_key_input")}</div>
        <Input
          type="password"
          autoComplete="off"
          placeholder={card.has_key ? t("settings.provider_key_stored") : t("llm.api_key")}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />
      </div>

      <details>
        <summary className="cursor-pointer text-sm text-muted-foreground hover:text-foreground">
          {t("settings.provider_advanced")}
        </summary>
        <div className="mt-3 space-y-4">
          <div className="space-y-2">
            <div className="text-sm font-medium">{t("settings.provider_endpoint")}</div>
            <Input value={endpoint} onChange={(e) => setEndpoint(e.target.value)} />
          </div>

          {(["llm", "embedding", "rerank"] as ProviderKind[]).map((kind) => {
            const models =
              kind === "llm" ? chatModels : kind === "embedding" ? embedModels : rerankModels;
            const setter =
              kind === "llm"
                ? setChatModels
                : kind === "embedding"
                  ? setEmbedModels
                  : setRerankModels;
            return (
              <div key={kind} className="space-y-2">
                <ModelListEditor
                  kind={kind}
                  models={models}
                  onChange={setter}
                  probe={probeBase}
                  testing={testing}
                  onTest={(mid) => {
                    const fn =
                      kind === "llm"
                        ? (id: string) => api.testLLM(id)
                        : kind === "embedding"
                          ? (id: string) => api.testEmbedding(id)
                          : (id: string) => api.testRerank(id);
                    onTest(kind, mid, fn);
                  }}
                  onMakeDefault={(rowId, isDefault) => toggleDefault(kind, rowId, isDefault)}
                  disabled={busy}
                  title={kindLabel(t, kind)}
                />
              </div>
            );
          })}
        </div>
      </details>

      <div className="flex justify-end gap-2 pt-2">
        <Button variant="outline" onClick={() => onClose(false)} disabled={busy}>
          {t("common.cancel")}
        </Button>
        <Button onClick={apply} disabled={busy || !endpoint}>
          {busy ? "..." : t("settings.provider_apply")}
        </Button>
      </div>
    </div>
  );
}
