"use client";

import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { toast } from "sonner";
import { Bot, MoreVertical, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api, EmbeddingModel, KnowledgeBase, Paginated } from "@/lib/api";
import { KBEditDrawer } from "@/components/kb/kb-edit-drawer";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";

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

  // undefined = closed; null = create; KB = edit. The form itself lives in
  // the shared KBEditDrawer (also used by the KB detail page).
  const [editing, setEditing] = useState<KnowledgeBase | null | undefined>(undefined);
  const [query, setQuery] = useState("");
  // Which KB card has its "..." overflow menu open (edit/delete live there).
  const [menuKbId, setMenuKbId] = useState<string | null>(null);

  // Click-away closes the open card menu. The trigger and menu stop
  // propagation on mousedown so their own clicks are not swallowed.
  useEffect(() => {
    if (!menuKbId) return;
    const close = () => setMenuKbId(null);
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [menuKbId]);

  const hasProviders = useMemo(
    () => (embedData?.items ?? []).some((p) => p.status === "active"),
    [embedData]
  );

  // id -> model name map for displaying KB cards.
  const embedNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of embedData?.items ?? []) m.set(p.id, p.model);
    return m;
  }, [embedData]);

  const filtered = data?.items?.filter(
    (kb) =>
      kb.name.toLowerCase().includes(query.toLowerCase()) ||
      (kb.description || "").toLowerCase().includes(query.toLowerCase())
  );

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

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("nav.knowledge_bases")}</h1>
        <Button onClick={() => setEditing(null)}>
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
          <Card key={kb.id} className="relative hover:bg-accent/40 transition-colors h-full">
            {/* Low-frequency actions (edit/delete) collapsed into an overflow menu */}
            <div className="absolute top-1.5 right-1.5 z-10">
              <Button
                size="icon"
                variant="ghost"
                className="h-7 w-7"
                aria-label={t("common.more")}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.preventDefault();
                  setMenuKbId(menuKbId === kb.id ? null : kb.id);
                }}
              >
                <MoreVertical className="h-4 w-4" />
              </Button>
              {menuKbId === kb.id && (
                <div
                  className="absolute right-0 top-full mt-1 rounded-md border border-border bg-popover shadow-md py-1 w-28"
                  onMouseDown={(e) => e.stopPropagation()}
                >
                  <button
                    onClick={() => { setMenuKbId(null); setEditing(kb); }}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-accent transition-colors"
                  >
                    <Pencil className="h-3.5 w-3.5" /> {t("common.edit")}
                  </button>
                  <button
                    onClick={() => { setMenuKbId(null); remove(kb); }}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-sm text-destructive hover:bg-accent transition-colors"
                  >
                    <Trash2 className="h-3.5 w-3.5" /> {t("common.delete")}
                  </button>
                </div>
              )}
            </div>
            <Link href={`/dashboard/knowledge-bases/${kb.id}`}>
              <CardHeader>
                <CardTitle className="text-base pr-8">{kb.name}</CardTitle>
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
            <div className="px-4 pb-3">
              <Button size="sm" variant="ghost" asChild>
                <Link href={`/dashboard/knowledge-bases/${kb.id}/agent`}>
                  <Bot className="h-3.5 w-3.5 mr-1" /> {t("kb.agent")}
                </Link>
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

      {editing !== undefined && (
        <KBEditDrawer
          kb={editing}
          onClose={() => setEditing(undefined)}
          onSaved={() => {
            setEditing(undefined);
            mutate();
          }}
        />
      )}
    </div>
  );
}
