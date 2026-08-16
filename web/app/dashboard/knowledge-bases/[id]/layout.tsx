"use client";

import { useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { useParams, usePathname, useRouter } from "next/navigation";
import { MoreVertical, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { api, KnowledgeBase } from "@/lib/api";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { KBEditDrawer } from "@/components/kb/kb-edit-drawer";

// Knowledge-base tabs. Every KB capability is one tab away so users can
// switch between documents / Q&A / agent / pipeline / memories / executions
// without returning to the documents page first. URLs stay stable (deep
// links and bookmarks keep working).
const TABS = [
  { suffix: "", key: "kb.tab_documents", exact: true },
  { suffix: "/pipeline", key: "kb.tab_pipeline" },
  { suffix: "/agent", key: "kb.tab_agent" },
  { suffix: "/annotations", key: "kb.tab_annotations" },
  { suffix: "/memories", key: "kb.tab_memories" },
  { suffix: "/executions", key: "kb.tab_executions" },
];

export default function KnowledgeBaseLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const pathname = usePathname();
  const router = useRouter();
  const t = useTranslations();
  const confirm = useConfirm();
  // Same SWR key as the documents page ("kb-<id>") so the KB object is
  // fetched once and shared across all tabs.
  const { data: kb, mutate: mutateKB } = useSWR<KnowledgeBase>(`kb-${kbId}`, () => api.getKB(kbId));
  const base = `/dashboard/knowledge-bases/${kbId}`;

  const [kbMenuOpen, setKbMenuOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);

  // KB-level operations live here, next to the tab bar: visibility applies
  // to the whole knowledge base, not to any single document.

  // Toggle KB visibility between "private" (owner only) and "team"
  // (all tenant members). Updates the cached KB object optimistically.
  async function setVisibility(visibility: "private" | "team") {
    if (!kb || kb.visibility === visibility) return;
    const prev = kb;
    mutateKB({ ...kb, visibility }, false);
    try {
      await api.setKBVisibility(kbId, visibility);
      toast.success(t("toast.updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutateKB(prev, false);
    }
  }

  // Delete the whole KB, then return to the list page.
  async function removeKB() {
    if (!kb) return;
    const ok = await confirm({
      title: t("kb.delete_confirm"),
      description: kb.name,
      confirmText: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await api.deleteKB(kbId);
      toast.success(t("toast.deleted"));
      router.push("/dashboard/knowledge-bases");
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight truncate">
        {kb?.name || t("kb.loading")}
      </h1>
      <div className="flex items-end gap-1 border-b mt-3 mb-6">
        <div className="flex gap-1 overflow-x-auto overflow-y-hidden pb-px">
          {TABS.map((tab) => {
            const href = tab.suffix === "" ? base : base + tab.suffix;
            const active = tab.exact ? pathname === base : pathname === href || pathname?.startsWith(href + "/");
            return (
              <Link
                key={tab.suffix || "_"}
                href={href}
                className={cn(
                  "px-3 py-2 text-sm whitespace-nowrap border-b-2 -mb-px transition-colors",
                  active
                    ? "border-primary text-primary font-medium"
                    : "border-transparent text-muted-foreground hover:text-foreground"
                )}
              >
                {t(tab.key)}
              </Link>
            );
          })}
        </div>

        {/* KB-level actions, right-aligned on the tab row */}
        <div className="ml-auto flex shrink-0 items-center gap-2 pb-1">
          {kb && (
            <div className="inline-flex rounded-md border overflow-hidden">
              <button
                onClick={() => setVisibility("private")}
                className={cn(
                  "px-2.5 py-1.5 text-xs transition-colors",
                  kb.visibility === "private"
                    ? "bg-primary text-primary-foreground"
                    : "bg-background hover:bg-accent"
                )}
                title={t("kb.visibility_private_desc")}
              >
                {t("kb.private")}
              </button>
              <button
                onClick={() => setVisibility("team")}
                className={cn(
                  "px-2.5 py-1.5 text-xs transition-colors border-l",
                  kb.visibility === "team"
                    ? "bg-primary text-primary-foreground"
                    : "bg-background hover:bg-accent"
                )}
                title={t("kb.visibility_team_desc")}
              >
                {t("kb.team_shared")}
              </button>
            </div>
          )}
          {kb && (
            <div className="relative">
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                aria-label={t("common.more")}
                onClick={() => setKbMenuOpen((v) => !v)}
              >
                <MoreVertical className="h-4 w-4" />
              </Button>
              {kbMenuOpen && (
                <>
                  <div className="fixed inset-0 z-40" onClick={() => setKbMenuOpen(false)} />
                  <div className="absolute right-0 top-full mt-1 z-50 w-28 rounded-md border border-border bg-popover shadow-md py-1">
                    <button
                      onClick={() => { setKbMenuOpen(false); setEditOpen(true); }}
                      className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-accent transition-colors"
                    >
                      <Pencil className="h-3.5 w-3.5" /> {t("common.edit")}
                    </button>
                    <button
                      onClick={() => { setKbMenuOpen(false); removeKB(); }}
                      className="flex w-full items-center gap-2 px-3 py-1.5 text-sm text-destructive hover:bg-accent transition-colors"
                    >
                      <Trash2 className="h-3.5 w-3.5" /> {t("common.delete")}
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </div>
      {children}

      {editOpen && kb && (
        <KBEditDrawer
          kb={kb}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            setEditOpen(false);
            mutateKB();
          }}
        />
      )}
    </div>
  );
}
