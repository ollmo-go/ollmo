"use client";

import { useState } from "react";
import useSWR from "swr";
import { Plus, Star, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Drawer } from "@/components/ui/drawer";
import { Skeleton } from "@/components/ui/skeleton";
import { api, ProviderCard } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { useConfirm } from "@/components/ui/confirm";
import { STATUS_DOT } from "@/lib/ui-colors";
import { kindLabel, FILTERS, modelsForKind } from "./_shared";
import type { FilterKind, ProviderKind } from "./_shared";
import { ProviderEditor } from "./provider-editor";
import { AddCatalogCard } from "./add-catalog-card";
import { CustomProviderCard } from "./custom-provider-card";

export function ProviderSection() {
  const t = useTranslations();
  const confirm = useConfirm();
  const cardsSWR = useSWR("providers", () => api.providers.list());
  const catalogSWR = useSWR("provider-catalog", () => api.providers.catalog());
  const cards = cardsSWR.data ?? [];
  const catalog = (catalogSWR.data ?? []).filter((c) => c.id !== "custom");

  const [filter, setFilter] = useState<FilterKind>("all");
  const [drawer, setDrawer] = useState<null | { mode: "add"; type: "catalog" | "custom" } | { mode: "edit"; card: ProviderCard }>(null);
  const [addCatalogId, setAddCatalogId] = useState("");
  const [testing, setTesting] = useState<string | null>(null);

  const mutate = cardsSWR.mutate;

  const addable = catalog.filter((c) => !cards.some((x) => x.catalog_id === c.id));
  const addSpec = addable.find((c) => c.id === addCatalogId) ?? addable[0];

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

                    {allModels.length > 0 && (
                      <div className="mt-3 space-y-2 pl-4">
                        {(["llm", "embedding", "rerank"] as ProviderKind[]).map((kind) => {
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
