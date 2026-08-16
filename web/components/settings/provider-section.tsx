"use client";

import { useState } from "react";
import useSWR from "swr";
import { ChevronDown, ChevronRight, Plus, RefreshCw, Star, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Drawer } from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api, CatalogProvider, DiscoveredModel, ProviderCard, ProviderModelRef } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { useConfirm } from "@/components/ui/confirm";
import { STATUS_DOT } from "@/lib/ui-colors";

export type ProviderKind = "llm" | "embedding" | "rerank";

// One editable model row. rowId is set for rows already stored server-side;
// freshly added rows carry only a local key until Apply. max_tokens/prices
// are chat-specific and stay undefined for embedding/rerank rows.
interface DraftModel {
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
function nextKey(): string {
  seq += 1;
  return `tmp-${seq}`;
}

// Capacity fields are edited as K/M-suffixed text (256K -> 256000).
function parseCapacity(text: string): number | undefined {
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

function formatCapacity(n?: number): string {
  if (!n) return "";
  if (n >= 1000 * 1000 && n % 1000 === 0) return `${n / (1000 * 1000)}M`;
  if (n >= 1000 && n % 1000 === 0) return `${n / 1000}K`;
  return String(n);
}

function modelsForKind(card: ProviderCard, kind: ProviderKind): ProviderModelRef[] {
  if (kind === "llm") return card.chat_models ?? [];
  if (kind === "embedding") return card.embed_models ?? [];
  return card.rerank_models ?? [];
}

// A stored row into an editable draft; 0-valued capacities normalize to
// undefined so an untouched row diffs clean against the server's omitted 0s.
function draftFromRef(m: ProviderModelRef): DraftModel {
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

function kindLabel(t: (k: string) => string, kind: ProviderKind): string {
  if (kind === "llm") return t("settings.section_llm");
  if (kind === "embedding") return t("settings.section_embedding");
  return t("settings.section_rerank");
}

// Filter segment for the provider list: all, or a single model kind.
type FilterKind = "all" | ProviderKind;
const FILTERS: FilterKind[] = ["all", "llm", "embedding", "rerank"];

// Unified provider settings page: one card per provider (newest first),
// filterable by model kind. Add and edit happen in a right-hand drawer.
export function ProviderSection() {
  const t = useTranslations();
  const confirm = useConfirm();
  const cardsSWR = useSWR("providers", () => api.providers.list());
  const catalogSWR = useSWR("provider-catalog", () => api.providers.catalog());
  const cards = cardsSWR.data ?? [];
  const catalog = (catalogSWR.data ?? []).filter((c) => c.id !== "custom");

  const [filter, setFilter] = useState<FilterKind>("all");
  // drawer: null (idle) | add catalog/custom | edit a card
  const [drawer, setDrawer] = useState<null | { mode: "add"; type: "catalog" | "custom" } | { mode: "edit"; card: ProviderCard }>(null);
  const [addCatalogId, setAddCatalogId] = useState("");
  const [testing, setTesting] = useState<string | null>(null);

  const mutate = cardsSWR.mutate;

  const addable = catalog.filter((c) => !cards.some((x) => x.catalog_id === c.id));
  const addSpec = addable.find((c) => c.id === addCatalogId) ?? addable[0];

  // Filter cards down to those serving the selected model kind.
  const filteredCards = cards.filter((card) => {
    if (filter === "all") return true;
    return modelsForKind(card, filter).length > 0;
  });

  async function removeCard(card: ProviderCard) {
    const ok = await confirm({
      title: t("settings.provider_delete_confirm"),
      description: card.name,
      destructive: true,
    });
    if (!ok) return;
    try {
      await api.providers.remove(card.id);
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function testModel(kind: ProviderKind, mid: string, testFn: (id: string) => Promise<{ reply: string }>) {
    setTesting(mid);
    try {
      const r = await testFn(mid);
      toast.success(t("settings.provider_test_ok", { reply: r.reply.slice(0, 80) }));
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    } finally {
      setTesting(null);
    }
  }

  return (
    <section className="space-y-4">
      {/* Toolbar: model-kind filter on the left, add entries on the right */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-1 rounded-lg border border-input bg-muted/40 p-1">
          {FILTERS.map((k) => (
            <button
              key={k}
              type="button"
              className={cn(
                "rounded-md px-3 py-1.5 text-sm transition-colors",
                filter === k
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground"
              )}
              onClick={() => setFilter(k)}
            >
              {k === "all" ? t("settings.provider_filter_all") : kindLabel(t, k)}
            </button>
          ))}
        </div>

        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={addable.length === 0}
            onClick={() => {
              setAddCatalogId("");
              setDrawer({ mode: "add", type: "catalog" });
            }}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t("settings.provider_add")}
          </Button>
          <Button variant="outline" onClick={() => setDrawer({ mode: "add", type: "custom" })}>
            <Plus className="h-4 w-4 mr-1" />
            {t("settings.provider_add_custom")}
          </Button>
        </div>
      </div>

      {cardsSWR.isLoading ? (
        <Skeleton className="h-32" />
      ) : filteredCards.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {t("settings.provider_empty")}
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3">
          {filteredCards.map((card) => {
            const allModels = [
              ...(card.chat_models ?? []),
              ...(card.embed_models ?? []),
              ...(card.rerank_models ?? []),
            ];
            // A provider is "connected" when at least one of its models has
            // passed a live test (same endpoint + key), regardless of kind.
            const testedOk = allModels.some((m) => m.last_test_status === "success");
            return (
              <li key={card.id}>
                <Card>
                  <CardContent className="py-4">
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-2">
                        <span
                          role="img"
                          aria-label={
                            !card.has_key
                              ? t("settings.provider_status_missing")
                              : testedOk
                                ? t("settings.provider_status_ok")
                                : t("settings.provider_status_untested")
                          }
                          title={
                            !card.has_key
                              ? t("settings.provider_status_missing")
                              : testedOk
                                ? t("settings.provider_status_ok")
                                : t("settings.provider_status_untested")
                          }
                          className={cn(
                            "h-2 w-2 shrink-0 rounded-full",
                            !card.has_key
                              ? STATUS_DOT.idle
                              : testedOk
                                ? STATUS_DOT.ok
                                : STATUS_DOT.warn
                          )}
                        />
                        <span className="truncate font-medium">{card.name}</span>
                        {card.catalog_id === "custom" && (
                          <span className="rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
                            {t("settings.provider_custom_tag")}
                          </span>
                        )}
                        <span className="truncate text-xs text-muted-foreground">{card.endpoint}</span>
                      </div>
                      <div className="flex shrink-0 gap-1">
                        <Button size="sm" variant="outline" onClick={() => setDrawer({ mode: "edit", card })}>
                          {t("common.edit")}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => removeCard(card)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </div>

                    {/* Collapsed summary: model names grouped by kind */}
                    {allModels.length > 0 && (
                      <div className="mt-3 space-y-2 pl-4">
                        {(["llm", "embedding", "rerank"] as ProviderKind[]).map((kind) => {
                          // When a kind filter is active, hide the other kinds' sections.
                          if (filter !== "all" && filter !== kind) return null;
                          const models = modelsForKind(card, kind);
                          if (models.length === 0) return null;
                          return (
                            <div key={kind}>
                              <div className="mb-1 text-xs font-medium text-muted-foreground">
                                {kindLabel(t, kind)}
                              </div>
                              <div className="flex flex-wrap items-center gap-1.5">
                                {models.map((m) => (
                                  <span
                                    key={m.id}
                                    title={m.model}
                                    className={cn(
                                      "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs",
                                      m.is_default
                                        ? "border-primary/40 bg-primary/10 text-primary"
                                        : "border-input text-muted-foreground"
                                    )}
                                  >
                                    {m.is_default && <Star className="h-3 w-3" />}
                                    {m.name || m.model}
                                  </span>
                                ))}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </CardContent>
                </Card>
              </li>
            );
          })}
        </ul>
      )}

      {/* Add-from-catalog drawer */}
      {drawer?.mode === "add" && drawer.type === "catalog" && addSpec ? (
        <Drawer title={t("settings.provider_add")} onClose={() => setDrawer(null)} width="max-w-lg">
          <div className="space-y-4">
            <div className="space-y-2">
              <div className="text-sm font-medium">{t("settings.provider")}</div>
              <select
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                value={addSpec.id}
                aria-label={t("settings.provider")}
                onChange={(e) => setAddCatalogId(e.target.value)}
              >
                {addable.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
            <AddCatalogCard
              spec={addSpec}
              onDone={() => {
                setDrawer(null);
                mutate();
              }}
              onCancel={() => setDrawer(null)}
            />
          </div>
        </Drawer>
      ) : drawer?.mode === "add" && drawer.type === "custom" ? (
        <Drawer title={t("settings.provider_add_custom")} onClose={() => setDrawer(null)} width="max-w-lg">
          <CustomProviderCard
            existingEndpoints={cards.map((c) => c.endpoint)}
            onDone={() => {
              setDrawer(null);
              mutate();
            }}
            onCancel={() => setDrawer(null)}
          />
        </Drawer>
      ) : null}

      {/* Edit drawer */}
      {drawer?.mode === "edit" && (
        <Drawer title={t("common.edit")} onClose={() => setDrawer(null)} width="max-w-lg">
          <ProviderEditor
            card={drawer.card}
            testing={testing}
            onTest={testModel}
            onClose={(changed) => {
              setDrawer(null);
              if (changed) mutate();
            }}
          />
        </Drawer>
      )}
    </section>
  );
}

// The editor for one existing provider, shown in the edit drawer: a
// write-only key field, an advanced area (base URL + model lists per kind),
// and one Apply that writes the card and diffs the model rows.
function ProviderEditor({
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

  // Toggle the tenant default: setting it clears the others; unsetting it
  // leaves the tenant without a default (fallback prompts the user).
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

      // Diff model rows for each kind.
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
            // Chat rows carry max_tokens and prices too; embedding/rerank
            // rows only diff name/context_length.
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
            await api.providers.addModel(card.id, kind, {
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
            const original =
              kind === "llm"
                ? card.chat_models ?? []
                : kind === "embedding"
                  ? card.embed_models ?? []
                  : card.rerank_models ?? [];
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

// Create card for a catalog provider, shown in the add drawer: the key
// (unless the catalog says none is needed), an optional endpoint override,
// and the catalog models to land as rows — checked by default.
function AddCatalogCard({
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
  // All models across kinds, keyed by `${kind}:${modelId}` for checkbox state.
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

// Create card for a custom gateway, shown in the add drawer: name, base URL,
// key, and a model list that can be fetched from the endpoint.
function CustomProviderCard({
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
  // An endpoint identifies a provider; reject a base URL already in use.
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

// The editable model rows plus the "fetch available models" action. Fetching
// probes the endpoint the FORM currently shows (key typed but not stored
// included), and the reply is candidates the user picks from — never a write.
function ModelListEditor({
  kind,
  models,
  onChange,
  probe,
  testing,
  onTest,
  onMakeDefault,
  disabled,
  title,
}: {
  kind: ProviderKind;
  models: DraftModel[];
  onChange: (models: DraftModel[]) => void;
  probe: { endpoint: string; api_key?: string; provider_id?: string };
  testing?: string | null;
  onTest?: (mid: string) => void;
  onMakeDefault?: (rowId: string, isDefault: boolean) => void;
  disabled?: boolean;
  title?: string;
}) {
  const t = useTranslations();
  const [probing, setProbing] = useState(false);
  const [candidates, setCandidates] = useState<DiscoveredModel[] | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  // Rows whose capacity disclosure is open, keyed by draft key.
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // Raw per-field text while a capacity input is being typed, keyed by
  // `${draftKey}:${field}`; removed on blur so the canonical value shows.
  const [textBuf, setTextBuf] = useState<Record<string, string>>({});
  // Tracks which row is running an unsaved-model connectivity test.
  const [localTesting, setLocalTesting] = useState<string | null>(null);

  function patch(key: string, next: Partial<DraftModel>) {
    onChange(models.map((m) => (m.key === key ? { ...m, ...next } : m)));
  }

  function toggleExpanded(key: string) {
    setExpanded((cur) => {
      const next = new Set(cur);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  }

  type CapacityField = "context_length" | "max_tokens" | "input_price" | "output_price";

  function fieldText(m: DraftModel, field: CapacityField): string {
    const buf = textBuf[`${m.key}:${field}`];
    if (buf !== undefined) return buf;
    if (field === "context_length") return formatCapacity(m.context_length);
    const v = m[field];
    return v === undefined || v === null || v === 0 ? "" : String(v);
  }

  function setField(m: DraftModel, field: CapacityField, raw: string) {
    setTextBuf((cur) => ({ ...cur, [`${m.key}:${field}`]: raw }));
    if (field === "context_length" || field === "max_tokens") {
      patch(m.key, { [field]: parseCapacity(raw) } as Partial<DraftModel>);
    } else {
      const n = raw.trim() === "" ? undefined : Number(raw);
      patch(m.key, { [field]: n !== undefined && Number.isFinite(n) ? n : undefined } as Partial<DraftModel>);
    }
  }

  function settleField(m: DraftModel, field: CapacityField) {
    setTextBuf((cur) => {
      const next = { ...cur };
      delete next[`${m.key}:${field}`];
      return next;
    });
  }

  // Saved rows test via the stored model id (onTest); fresh rows fire a
  // probe-model call against the form's endpoint+key so a model can be
  // validated before it is ever saved.
  async function testRow(m: DraftModel) {
    if (m.rowId) {
      onTest?.(m.rowId);
      return;
    }
    if (!m.model.trim()) {
      toast.error(t("settings.provider_model_id_empty"));
      return;
    }
    if (!probe.endpoint) {
      toast.error(t("settings.fetch_need_endpoint"));
      return;
    }
    setLocalTesting(m.key);
    try {
      await api.providers.probeModel({
        kind,
        endpoint: probe.endpoint,
        api_key: probe.api_key ?? "",
        model: m.model.trim(),
      });
      toast.success(`${m.model.trim()} OK`);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLocalTesting(null);
    }
  }

  async function fetchModels() {
    setProbing(true);
    try {
      const found = await api.providers.probe({
        endpoint: probe.endpoint,
        ...(probe.api_key ? { api_key: probe.api_key } : {}),
        ...(probe.provider_id ? { provider_id: probe.provider_id } : {}),
      });
      if (found.length === 0) {
        toast.error(t("settings.fetch_models_empty"));
        return;
      }
      const known = new Set(models.map((m) => m.model));
      setCandidates(found);
      setPicked(new Set(found.filter((m) => !known.has(m.id)).map((m) => m.id)));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setProbing(false);
    }
  }

  function adoptPicked() {
    if (!candidates) return;
    const byModel = new Map(models.map((m) => [m.model, m]));
    for (const c of candidates) {
      if (!picked.has(c.id)) continue;
      if (byModel.has(c.id)) continue;
      byModel.set(c.id, {
        key: nextKey(),
        model: c.id,
        name: c.name || "",
        context_length: c.context_length,
      });
    }
    onChange([...byModel.values()]);
    setCandidates(null);
    setPicked(new Set());
  }

  const askable = probe.endpoint.length > 0;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{title ?? t("settings.provider_models_title")}</div>
        <button
          type="button"
          className="text-xs text-muted-foreground hover:text-foreground disabled:opacity-50"
          disabled={disabled || probing || !askable}
          title={!askable ? t("settings.fetch_need_endpoint") : undefined}
          onClick={fetchModels}
        >
          <RefreshCw className={cn("mr-1 inline h-3.5 w-3.5", probing && "animate-spin")} />
          {probing ? t("common.loading") : t("settings.fetch_models")}
        </button>
      </div>

      {models.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("settings.provider_models_empty")}</p>
      )}

      {models.map((m) => {
        const open = expanded.has(m.key);
        return (
          <div key={m.key} className="rounded-md border border-input px-2 py-1.5">
            <div className="flex items-center gap-2">
              {m.rowId ? (
                <span className="h-9 min-w-0 flex-1 truncate rounded-md border border-input bg-muted/40 px-3 pt-2 font-mono text-xs leading-5">
                  {m.model}
                </span>
              ) : (
                <Input
                  className="min-w-0 flex-1 font-mono text-xs"
                  placeholder={t("settings.provider_model_id")}
                  value={m.model}
                  disabled={disabled}
                  onChange={(e) => patch(m.key, { model: e.target.value })}
                />
              )}
              <Input
                className="min-w-0 flex-1 text-xs"
                placeholder={t("settings.provider_model_name")}
                value={m.name}
                disabled={disabled}
                onChange={(e) => patch(m.key, { name: e.target.value })}
              />
              <button
                type="button"
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
                aria-label={t("settings.model_capacity")}
                aria-expanded={open}
                title={t("settings.model_capacity")}
                disabled={disabled}
                onClick={() => toggleExpanded(m.key)}
              >
                {open ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </button>
              {m.rowId && onMakeDefault && (
                <button
                  type="button"
                  className="shrink-0 text-muted-foreground hover:text-primary disabled:opacity-50"
                  title={m.is_default ? t("settings.unset_default") : t("settings.set_default")}
                  disabled={disabled}
                  onClick={() => onMakeDefault(m.rowId!, !!m.is_default)}
                >
                  <Star className={cn("h-3.5 w-3.5", m.is_default && "fill-primary text-primary")} />
                </button>
              )}
              {(m.rowId ? onTest : true) && (
                <button
                  type="button"
                  className="shrink-0 text-muted-foreground hover:text-primary disabled:opacity-50"
                  title={t("settings.test")}
                  disabled={disabled || testing === m.rowId || localTesting === m.key}
                  onClick={() => testRow(m)}
                >
                  <Zap
                    className={cn(
                      "h-3.5 w-3.5",
                      (testing === m.rowId || localTesting === m.key) && "animate-pulse"
                    )}
                  />
                </button>
              )}
              <button
                type="button"
                className="shrink-0 text-muted-foreground hover:text-destructive disabled:opacity-50"
                title={t("common.delete")}
                disabled={disabled}
                onClick={() => onChange(models.filter((x) => x.key !== m.key))}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
            {open && (
              <div className="mt-2 grid grid-cols-2 gap-2 border-t pt-2 sm:grid-cols-4">
                <CapacityField
                  m={m}
                  field="context_length"
                  fieldText={fieldText}
                  setField={setField}
                  settleField={settleField}
                  placeholder="128K"
                  disabled={disabled}
                  t={t}
                />
                {kind === "llm" && (
                  <>
                    <CapacityField
                      m={m}
                      field="max_tokens"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="2048"
                      disabled={disabled}
                      t={t}
                    />
                    <CapacityField
                      m={m}
                      field="input_price"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="0"
                      disabled={disabled}
                      t={t}
                    />
                    <CapacityField
                      m={m}
                      field="output_price"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="0"
                      disabled={disabled}
                      t={t}
                    />
                  </>
                )}
              </div>
            )}
          </div>
        );
      })}

      <Button
        variant="ghost"
        size="sm"
        className="text-xs text-muted-foreground"
        disabled={disabled}
        onClick={() => onChange([...models, { key: nextKey(), model: "", name: "" }])}
      >
        <Plus className="h-3.5 w-3.5 mr-1" />
        {t("settings.provider_add_model")}
      </Button>

      {candidates !== null && (
        <Dialog
          onClose={() => {
            setCandidates(null);
            setPicked(new Set());
          }}
          title={t("settings.fetch_dialog_title")}
          description={t("settings.fetch_dialog_desc")}
          footer={
            <>
              <Button
                variant="outline"
                onClick={() => {
                  setCandidates(null);
                  setPicked(new Set());
                }}
              >
                {t("common.cancel")}
              </Button>
              <Button onClick={adoptPicked} disabled={picked.size === 0}>
                {t("settings.fetch_adopt")}
              </Button>
            </>
          }
        >
          <ul className="space-y-1">
            {candidates.map((c) => (
              <li key={c.id}>
                <label className="flex cursor-pointer items-center justify-between rounded-md border border-input px-3 py-2 text-sm hover:bg-muted">
                  <span className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={picked.has(c.id)}
                      onChange={() =>
                        setPicked((s) => {
                          const n = new Set(s);
                          if (!n.delete(c.id)) n.add(c.id);
                          return n;
                        })
                      }
                    />
                    <span className="font-mono text-xs">{c.id}</span>
                  </span>
                  {!!c.context_length && (
                    <span className="text-xs text-muted-foreground">
                      {Math.floor(c.context_length / 1024)}K ctx
                    </span>
                  )}
                </label>
              </li>
            ))}
          </ul>
        </Dialog>
      )}
    </div>
  );
}

type CapacityFieldKey = "context_length" | "max_tokens" | "input_price" | "output_price";

function capacityLabel(field: CapacityFieldKey, t: ReturnType<typeof useTranslations>): string {
  if (field === "context_length") return t("settings.model_context_window");
  if (field === "max_tokens") return t("settings.model_max_tokens");
  if (field === "input_price") return t("settings.model_input_price");
  return t("settings.model_output_price");
}

// Price fields carry their per-1M-token unit in a hover tooltip so the
// short label never wraps inside the disclosure grid.
function capacityUnitHint(field: CapacityFieldKey, t: ReturnType<typeof useTranslations>): string | undefined {
  if (field === "input_price" || field === "output_price") {
    return t("settings.model_price_unit");
  }
  return undefined;
}

// One capacity field inside a row's disclosure: a small labeled input that
// edits K/M-suffixed text and settles back to the canonical value on blur.
function CapacityField({
  m,
  field,
  fieldText,
  setField,
  settleField,
  placeholder,
  disabled,
  t,
}: {
  m: DraftModel;
  field: CapacityFieldKey;
  fieldText: (m: DraftModel, field: CapacityFieldKey) => string;
  setField: (m: DraftModel, field: CapacityFieldKey, raw: string) => void;
  settleField: (m: DraftModel, field: CapacityFieldKey) => void;
  placeholder: string;
  disabled?: boolean;
  t: ReturnType<typeof useTranslations>;
}) {
  return (
    <label className="space-y-1">
      <span
        className="text-[11px] text-muted-foreground"
        title={capacityUnitHint(field, t)}
      >
        {capacityLabel(field, t)}
      </span>
      <Input
        className="h-8 text-xs"
        inputMode="numeric"
        placeholder={placeholder}
        value={fieldText(m, field)}
        disabled={disabled}
        onChange={(e) => setField(m, field, e.target.value)}
        onBlur={() => settleField(m, field)}
      />
    </label>
  );
}
