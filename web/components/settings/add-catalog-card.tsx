"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api, CatalogProvider } from "@/lib/api";
import { useTranslations } from "next-intl";
import type { ProviderKind } from "./_shared";
import { kindLabel } from "./_shared";

export function AddCatalogCard({
  spec,
  onDone,
  onCancel,
}: {
  spec: CatalogProvider;
  onDone: () => void;
  onCancel: () => void;
}) {
  const t = useTranslations();
  const needKey = spec.note !== "no_key";
  const [apiKey, setApiKey] = useState("");
  const [endpoint, setEndpoint] = useState(spec.endpoint);
  const allCatalogModels = [
    ...(spec.chat_models ?? []).map((m) => ({ ...m, kind: "llm" as ProviderKind })),
    ...(spec.embedding_models ?? []).map((m) => ({ ...m, kind: "embedding" as ProviderKind })),
    ...(spec.rerank_models ?? []).map((m) => ({ ...m, kind: "rerank" as ProviderKind })),
  ];
  const [picked, setPicked] = useState<Set<string>>(
    () => new Set(allCatalogModels.map((m) => `${m.kind}:${m.id}`))
  );
  const [busy, setBusy] = useState(false);

  function toggle(key: string) {
    setPicked((s) => {
      const n = new Set(s);
      if (!n.delete(key)) n.add(key);
      return n;
    });
  }

  async function create() {
    setBusy(true);
    try {
      const chatModels = (spec.chat_models ?? [])
        .filter((m) => picked.has(`llm:${m.id}`))
        .map((m) => ({ model: m.id, context_length: m.context_length }));
      const embedModels = (spec.embedding_models ?? [])
        .filter((m) => picked.has(`embedding:${m.id}`))
        .map((m) => ({ model: m.id, context_length: m.context_length }));
      const rerankModels = (spec.rerank_models ?? [])
        .filter((m) => picked.has(`rerank:${m.id}`))
        .map((m) => ({ model: m.id, context_length: m.context_length }));

      await api.providers.create({
        catalog_id: spec.id,
        ...(endpoint !== spec.endpoint ? { endpoint } : {}),
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
        chat_models: chatModels,
        embed_models: embedModels,
        rerank_models: rerankModels,
      });
      onDone();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const ready = (!needKey || apiKey.trim().length > 0) && picked.size > 0;

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="text-sm font-medium">{t("settings.provider_key_input")}</div>
        <Input
          type="password"
          autoComplete="off"
          placeholder={needKey ? t("llm.api_key") : t("settings.catalog_note_no_key")}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />
      </div>

      <details>
        <summary className="cursor-pointer text-sm text-muted-foreground hover:text-foreground">
          {t("settings.provider_advanced")}
        </summary>
        <div className="mt-3 space-y-2">
          <div className="text-sm font-medium">{t("settings.provider_endpoint")}</div>
          <Input value={endpoint} onChange={(e) => setEndpoint(e.target.value)} />
        </div>
      </details>

      <div className="space-y-3">
        <div className="text-sm font-medium">{t("settings.provider_models_title")}</div>
        {(["llm", "embedding", "rerank"] as ProviderKind[]).map((kind) => {
          const models = allCatalogModels.filter((m) => m.kind === kind);
          if (models.length === 0) return null;
          return (
            <div key={kind} className="space-y-1">
              <div className="text-xs font-medium text-muted-foreground">{kindLabel(t, kind)}</div>
              {models.map((m) => {
                const key = `${m.kind}:${m.id}`;
                return (
                  <label
                    key={key}
                    className="flex cursor-pointer items-center justify-between rounded-md border border-input px-3 py-2 text-sm hover:bg-muted"
                  >
                    <span className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={picked.has(key)}
                        onChange={() => toggle(key)}
                      />
                      <span className="font-mono text-xs">{m.id}</span>
                      {m.note === "free" && (
                        <span className="rounded bg-emerald-500/10 px-1.5 py-px text-[10px] text-emerald-600">
                          {t("settings.free_badge")}
                        </span>
                      )}
                    </span>
                    {!!m.context_length && (
                      <span className="text-xs text-muted-foreground">
                        {Math.floor(m.context_length / 1024)}K ctx
                      </span>
                    )}
                  </label>
                );
              })}
            </div>
          );
        })}
      </div>

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
