"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import useSWR from "swr";
import { ArrowLeft, MessageSquareQuote, Pencil, Plus, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api, Annotation, Paginated } from "@/lib/api";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";
import { cn } from "@/lib/utils";

export default function AnnotationsPage() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const t = useTranslations();
  const confirm = useConfirm();

  const { data: page, mutate } = useSWR<Paginated<Annotation>>(
    `annotations-${kbId}`,
    () => api.listAnnotations(kbId, 1, 100)
  );

  // Create/edit form state; editingId === null means "create".
  const [formOpen, setFormOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");
  const [saving, setSaving] = useState(false);

  function openCreate() {
    setEditingId(null);
    setQuestion("");
    setAnswer("");
    setFormOpen(true);
  }

  function openEdit(a: Annotation) {
    setEditingId(a.id);
    setQuestion(a.question);
    setAnswer(a.answer);
    setFormOpen(true);
  }

  async function save() {
    if (!question.trim() || !answer.trim()) return;
    setSaving(true);
    try {
      if (editingId) {
        await api.updateAnnotation(kbId, editingId, { question, answer });
      } else {
        await api.createAnnotation(kbId, { question, answer });
      }
      setFormOpen(false);
      mutate();
      toast.success(t("toast.updated"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function toggleEnabled(a: Annotation, enabled: boolean) {
    try {
      await api.updateAnnotation(kbId, a.id, { enabled });
      mutate(
        (p) =>
          p
            ? { ...p, items: p.items.map((it) => (it.id === a.id ? { ...it, enabled } : it)) }
            : p,
        false
      );
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    }
  }

  async function remove(a: Annotation) {
    const ok = await confirm({
      title: t("annotation.delete_confirm"),
      description: a.question,
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteAnnotation(kbId, a.id);
      mutate();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <div>
      <Link
        href={`/dashboard/knowledge-bases/${kbId}`}
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4"
      >
        <ArrowLeft className="h-4 w-4 mr-1" /> {t("kb.back")}
      </Link>

      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("annotation.title")}</h1>
          <p className="text-sm text-muted-foreground mt-1">{t("annotation.desc")}</p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          {t("annotation.add")}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("annotation.list")}</CardTitle>
        </CardHeader>
        <CardContent>
          {!page && (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          )}
          {page && page.items.length === 0 && (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <MessageSquareQuote className="h-10 w-10 mb-3 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("annotation.empty")}</p>
            </div>
          )}
          <div className="space-y-3">
            {page?.items.map((a) => (
              <div key={a.id} className="rounded-md border p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">{a.question}</p>
                    <p className="text-sm text-muted-foreground mt-1 line-clamp-3">{a.answer}</p>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={() => toggleEnabled(a, !a.enabled)}
                      className={cn(
                        "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
                        a.enabled ? "bg-primary" : "bg-muted"
                      )}
                      title={a.enabled ? t("doc.enabled") : t("doc.disabled")}
                      aria-label={a.enabled ? t("doc.enabled") : t("doc.disabled")}
                    >
                      <span
                        className={cn(
                          "inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform",
                          a.enabled ? "translate-x-4" : "translate-x-1"
                        )}
                      />
                    </button>
                    <Button size="icon" variant="ghost" onClick={() => openEdit(a)} title={t("common.edit")}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button size="icon" variant="ghost" onClick={() => remove(a)} title={t("common.delete")}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {formOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setFormOpen(false)} aria-hidden="true" />
          <div className="relative w-full max-w-lg rounded-lg border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">
                {editingId ? t("annotation.edit") : t("annotation.add")}
              </h3>
              <Button size="icon" variant="ghost" onClick={() => setFormOpen(false)} aria-label={t("common.close")}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="space-y-3">
              <div>
                <p className="text-sm font-medium mb-1">{t("annotation.question")}</p>
                <Input
                  autoFocus
                  value={question}
                  onChange={(e) => setQuestion(e.target.value)}
                  placeholder={t("annotation.question_placeholder")}
                />
              </div>
              <div>
                <p className="text-sm font-medium mb-1">{t("annotation.answer")}</p>
                <textarea
                  className="w-full text-sm rounded-md border border-input bg-background px-3 py-2 resize-y focus:outline-none focus:ring-2 focus:ring-ring"
                  rows={6}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                  placeholder={t("annotation.answer_placeholder")}
                />
              </div>
              <p className="text-xs text-muted-foreground">{t("annotation.match_hint")}</p>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <Button variant="outline" onClick={() => setFormOpen(false)} disabled={saving}>
                {t("common.cancel")}
              </Button>
              <Button onClick={save} disabled={saving || !question.trim() || !answer.trim()}>
                {saving ? "..." : t("common.save")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
