"use client";

import { useMemo, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { toast } from "sonner";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Drawer } from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { api, EmbeddingModel, KnowledgeBase, Paginated } from "@/lib/api";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";

type FormState = {
  id?: string;
  name: string;
  description: string;
  embeddingModelId: string;
  chunkSize: number;
  chunkOverlap: number;
};

const EMPTY: FormState = {
  name: "",
  description: "",
  embeddingModelId: "",
  chunkSize: 500,
  chunkOverlap: 50,
};

export default function KnowledgeBasesPage() {
  const { data, mutate } = useSWR<Paginated<KnowledgeBase>>(
    "kb-list",
    () => api.listKBs(1, 50)
  );
  const { data: embedData } = useSWR<Paginated<EmbeddingModel>>(
    "embedding-list",
    () => api.listEmbeddings(1, 50)
  );
  const t = useTranslations();
  const confirm = useConfirm();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [form, setForm] = useState<FormState>(EMPTY);
  const [originalModelId, setOriginalModelId] = useState("");
  const [saving, setSaving] = useState(false);
  const [query, setQuery] = useState("");

  const isEdit = !!form.id;

  // Active embedding providers as {id, model} options. When editing, always
  // include the KB's current id even if its provider was deleted/deactivated
  // so the dropdown shows a valid selection.
  const modelOptions = useMemo(() => {
    const items = (embedData?.items ?? [])
      .filter((p) => p.status === "active")
      .map((p) => ({ id: p.id, model: p.model }));
    if (isEdit && form.embeddingModelId && !items.some((o) => o.id === form.embeddingModelId)) {
      const cur = (embedData?.items ?? []).find((p) => p.id === form.embeddingModelId);
      items.unshift({ id: form.embeddingModelId, model: cur?.model || form.embeddingModelId });
    }
    return items;
  }, [embedData, isEdit, form.embeddingModelId]);

  // hasProviders reflects whether the user can create a new KB: at least one
  // active embedding provider must exist.
  const activeProviderCount = useMemo(
    () => (embedData?.items ?? []).filter((p) => p.status === "active").length,
    [embedData]
  );
  const hasProviders = activeProviderCount > 0;

  // The tenant's default embedding provider's model name, shown in the hint
  // when "use tenant default" is selected.
  const defaultEmbedModel = useMemo(
    () => (embedData?.items ?? []).find((p) => p.is_default && p.status === "active")?.model ?? "",
    [embedData]
  );

  // id -> model name map for displaying KB cards.
  const embedNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of embedData?.items ?? []) m.set(p.id, p.model);
    return m;
  }, [embedData]);

  // The tenant's default embedding provider's id, used to detect whether
  // selecting "use tenant default" actually changes the pinned model.
  const defaultEmbedModelId = useMemo(
    () => (embedData?.items ?? []).find((p) => p.is_default && p.status === "active")?.id ?? "",
    [embedData]
  );

  const filtered = data?.items?.filter(
    (kb) =>
      kb.name.toLowerCase().includes(query.toLowerCase()) ||
      (kb.description || "").toLowerCase().includes(query.toLowerCase())
  );

  function openCreate() {
    setForm({
      ...EMPTY,
      embeddingModelId: "",
    });
    setOriginalModelId("");
    setDrawerOpen(true);
  }

  function openEdit(kb: KnowledgeBase) {
    setForm({
      id: kb.id,
      name: kb.name,
      description: kb.description || "",
      embeddingModelId: kb.embedding_model_id,
      chunkSize: kb.chunk_size,
      chunkOverlap: kb.chunk_overlap,
    });
    setOriginalModelId(kb.embedding_model_id);
    setDrawerOpen(true);
  }

  async function remove(kb: KnowledgeBase) {
    const ok = await confirm({
      title: t("kb.delete_confirm"),
      description: kb.name,
      confirmText: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await api.deleteKB(kb.id);
      toast.success(t("toast.deleted"));
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function submit() {
    if (!form.name.trim()) {
      toast.error(t("kb.name_required"));
      return;
    }

    // Warn the user when the embedding model changes: existing vectors will
    // be dropped and all docs re-embedded, which can take a while. An empty
    // value means "use tenant default" — it counts as a change only when the
    // current id differs from the resolved default id.
    const effectiveModelId = form.embeddingModelId || defaultEmbedModelId;
    const modelChanged = isEdit && effectiveModelId !== originalModelId;
    if (modelChanged) {
      const ok = await confirm({
        title: t("kb.model_change_title"),
        description: t("kb.model_change_desc"),
        confirmText: t("common.confirm"),
        destructive: true,
      });
      if (!ok) return;
    }

    setSaving(true);
    try {
      if (isEdit) {
        await api.updateKB(form.id!, {
          name: form.name,
          description: form.description,
          embedding_model_id: modelChanged ? form.embeddingModelId : undefined,
        });
        toast.success(modelChanged ? t("toast.updated_with_reembed") : t("toast.updated"));
      } else {
        await api.createKB({
          name: form.name,
          description: form.description,
          embedding_model_id: form.embeddingModelId,
          chunk_size: form.chunkSize,
          chunk_overlap: form.chunkOverlap,
        });
        toast.success(t("toast.created"));
      }
      setDrawerOpen(false);
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("nav.knowledge_bases")}</h1>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" /> {t("kb.new")}
        </Button>
      </div>

      {!hasProviders && (
        <p className="text-sm text-muted-foreground border rounded-md p-3 bg-muted/30 mb-4">
          {t.rich("kb.need_embedding_provider", {
            link: (chunks) => (
              <Link href="/dashboard/settings/models" className="underline">{chunks}</Link>
            ),
          })}
        </p>
      )}

      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t("common.search")}
        className="mb-4 max-w-xs"
      />

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {!data && Array.from({ length: 6 }).map((_, i) => (
          <Card key={i}>
            <CardHeader>
              <Skeleton className="h-4 w-28" />
            </CardHeader>
            <CardContent className="space-y-2">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-2/3" />
            </CardContent>
          </Card>
        ))}
        {filtered?.map((kb) => (
          <Card key={kb.id} className="hover:bg-accent/40 transition-colors h-full">
            <Link href={`/dashboard/knowledge-bases/${kb.id}`}>
              <CardHeader>
                <CardTitle className="text-base">{kb.name}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-1 text-sm text-muted-foreground">
                <p className="truncate">{kb.description || t("kb.no_description")}</p>
                <p>
                  {kb.doc_count} {t("kb.docs")} ·{" "}
                  {kb.embedding_model_id
                    ? embedNameById.get(kb.embedding_model_id) ?? kb.embedding_model_id
                    : t("kb.no_embedding")}
                  <span
                    className={`ml-2 px-1.5 py-0.5 rounded text-xs ${
                      kb.visibility === "team"
                        ? "bg-blue-100 text-blue-700"
                        : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {kb.visibility === "team" ? t("kb.team_shared") : t("kb.private")}
                  </span>
                </p>
                <p>chunk {kb.chunk_size}/{kb.chunk_overlap}</p>
              </CardContent>
            </Link>
            <div className="px-4 pb-3 flex items-center gap-1">
              <Button size="sm" variant="ghost" onClick={() => openEdit(kb)}>
                <Pencil className="h-3.5 w-3.5 mr-1" /> {t("common.edit")}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => remove(kb)} className="text-destructive hover:text-destructive">
                <Trash2 className="h-3.5 w-3.5 mr-1" /> {t("common.delete")}
              </Button>
            </div>
          </Card>
        ))}
        {data && data.items.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {t.rich("kb.empty_hint", {
              b: (chunks) => <b>{chunks}</b>,
            })}
          </p>
        )}
      </div>

      {drawerOpen && (
        <Drawer
          title={isEdit ? t("kb.edit_title") : t("kb.new_title")}
          onClose={() => setDrawerOpen(false)}
          onSave={submit}
          saving={saving}
        >
          <div className="space-y-4">
            <div className="space-y-1">
              <Label htmlFor="kb-name">{t("kb.name")}</Label>
              <Input
                id="kb-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="kb-desc">{t("kb.description")}</Label>
              <Input
                id="kb-desc"
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="kb-model">{t("kb.embedding_model")}</Label>
              <select
                id="kb-model"
                className="w-full h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={form.embeddingModelId}
                onChange={(e) => setForm({ ...form, embeddingModelId: e.target.value })}
              >
                <option value="">
                  {defaultEmbedModel
                    ? `${t("kb.embedding_default")} (${defaultEmbedModel})`
                    : t("kb.embedding_none")}
                </option>
                {modelOptions.map((m) => (
                  <option key={m.id} value={m.id}>{m.model}</option>
                ))}
              </select>
              {!form.embeddingModelId && defaultEmbedModel && (
                <p className="text-xs text-muted-foreground">
                  {t("kb.embedding_hint", { model: defaultEmbedModel })}
                </p>
              )}
              {!form.embeddingModelId && !defaultEmbedModel && (
                <p className="text-xs text-muted-foreground">
                  {t("kb.embedding_none_hint")}
                </p>
              )}
              {isEdit && form.embeddingModelId !== originalModelId && (
                <p className="text-xs text-destructive">
                  {t("kb.model_change_warning")}
                </p>
              )}
            </div>
            {!isEdit && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1">
                  <Label htmlFor="kb-chunk">{t("kb.chunk_size")}</Label>
                  <Input
                    id="kb-chunk"
                    type="number"
                    value={form.chunkSize}
                    onChange={(e) => setForm({ ...form, chunkSize: Number(e.target.value) })}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="kb-overlap">{t("kb.overlap")}</Label>
                  <Input
                    id="kb-overlap"
                    type="number"
                    value={form.chunkOverlap}
                    onChange={(e) => setForm({ ...form, chunkOverlap: Number(e.target.value) })}
                  />
                </div>
              </div>
            )}
            {isEdit && (
              <p className="text-sm text-muted-foreground border rounded-md p-3 bg-muted/30">
                {t("kb.edit_hint")}
              </p>
            )}
          </div>
        </Drawer>
      )}
    </div>
  );
}
