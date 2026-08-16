"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import useSWR from "swr";
import { Brain, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, Memory, Paginated } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";

export default function MemoriesPage() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const { data, mutate } = useSWR<Paginated<Memory>>(`memories-${kbId}`, () =>
    api.listMemories(kbId, 1, 50)
  );
  const t = useTranslations();
  const confirm = useConfirm();

  async function del(id: string) {
    const ok = await confirm({
      title: t("memory.delete_confirm"),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteMemory(kbId, id);
      mutate();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("memory.title")}</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {t("memory.subtitle")}
          </p>
        </div>
      </div>

      <div className="space-y-3">
        {data?.items.length === 0 && (
          <Card>
            <CardContent className="py-12 text-center">
              <Brain className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
              <p className="text-sm text-muted-foreground">
                {t("memory.empty")}
              </p>
            </CardContent>
          </Card>
        )}
        {data?.items.map((m) => (
          <Card key={m.id}>
            <CardContent className="py-4">
              <div className="flex items-start justify-between gap-3">
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium mb-1">{m.title}</div>
                  <p className="text-sm text-muted-foreground">{m.summary}</p>
                  {m.key_points && (
                    <ul className="mt-2 text-xs text-muted-foreground list-disc list-inside">
                      {parseKeyPoints(m.key_points).map((kp, i) => (
                        <li key={i}>{kp}</li>
                      ))}
                    </ul>
                  )}
                  <p className="text-xs text-muted-foreground mt-2">
                    {new Date(m.created_at).toLocaleString()}
                  </p>
                </div>
                <button
                  onClick={() => del(m.id)}
                  className="text-muted-foreground hover:text-destructive p-1"
                  title={t("memory.delete_tooltip")}
                  aria-label={t("memory.delete")}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

function parseKeyPoints(s: string): string[] {
  try {
    const arr = JSON.parse(s);
    if (Array.isArray(arr)) return arr.map(String);
  } catch {
    // not JSON, return empty
  }
  return [];
}
