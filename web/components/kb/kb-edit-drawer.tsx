"use client";

import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";
import { toast } from "sonner";
import { Drawer } from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, EmbeddingModel, KnowledgeBase, Paginated, ProviderCard } from "@/lib/api";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";
import { ProviderModelSelect } from "@/components/ui/provider-model-select";

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

// KBEditDrawer is the shared create/edit form for knowledge bases, used by
// the KB list page and the KB detail page. kb=null opens it in create mode.
// Embedding options are fetched through the shared "embedding-list" SWR key
// so both pages reuse one cache entry.
export function KBEditDrawer({
  kb,
  onClose,
  onSaved,
}: {
  kb: KnowledgeBase | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = !!kb;
  const t = useTranslations();
  const confirm = useConfirm();
  const { data: embedData } = useSWR<Paginated<EmbeddingModel>>(
    "embedding-list",
    () => api.listEmbeddings(1, 50)
  );
  const { data: providerCards } = useSWR<ProviderCard[]>("providers", () =>
    api.providers.list()
  );
  const providers = providerCards ?? [];

  const [form, setForm] = useState<FormState>(EMPTY);
  const [originalModelId, setOriginalModelId] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!kb) {
      setForm(EMPTY);
      setOriginalModelId("");
      return;
    }
    setForm({
      id: kb.id,
      name: kb.name,
      description: kb.description || "",
      embeddingModelId: kb.embedding_model_id,
      chunkSize: kb.chunk_size,
      chunkOverlap: kb.chunk_overlap,
    });
    setOriginalModelId(kb.embedding_model_id);
  }, [kb]);

  // The tenant's default embedding provider's model name, shown in the hint
  // when "use tenant default" is selected.
  const defaultEmbedModel = useMemo(
    () => (embedData?.items ?? []).find((p) => p.is_default && p.status === "active")?.model ?? "",
    [embedData]
  );

  // The tenant's default embedding provider's id, used to detect whether
  // selecting "use tenant default" actually changes the pinned model.
  const defaultEmbedModelId = useMemo(
    () => (embedData?.items ?? []).find((p) => p.is_default && p.status === "active")?.id ?? "",
    [embedData]
  );

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
      onSaved();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Drawer
      title={isEdit ? t("kb.edit_title") : t("kb.new_title")}
      onClose={onClose}
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
          <ProviderModelSelect
            models={embedData?.items ?? []}
            providers={providers}
            value={form.embeddingModelId}
            onChange={(v) => setForm({ ...form, embeddingModelId: v })}
            defaultLabel={
              defaultEmbedModel
                ? `${t("kb.embedding_default")} (${defaultEmbedModel})`
                : t("kb.embedding_none")
            }
            providerAllLabel={t("common.provider_all")}
            otherLabel={t("common.provider_other")}
            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          />
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
  );
}
