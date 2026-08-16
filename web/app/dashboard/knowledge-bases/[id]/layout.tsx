"use client";

import Link from "next/link";
import useSWR from "swr";
import { useParams, usePathname } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { api, KnowledgeBase } from "@/lib/api";
import { useTranslations } from "next-intl";

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
  const t = useTranslations();
  // Same SWR key as the documents page ("kb-<id>") so the KB object is
  // fetched once and shared across all tabs.
  const { data: kb } = useSWR<KnowledgeBase>(`kb-${kbId}`, () => api.getKB(kbId));
  const base = `/dashboard/knowledge-bases/${kbId}`;

  return (
    <div>
      <Link
        href="/dashboard/knowledge-bases"
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-3"
      >
        <ArrowLeft className="h-4 w-4 mr-1" /> {t("kb.back")}
      </Link>
      <h1 className="text-2xl font-semibold tracking-tight truncate">
        {kb?.name || t("kb.loading")}
      </h1>
      <div className="flex gap-1 border-b mt-3 mb-6">
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
      {children}
    </div>
  );
}
