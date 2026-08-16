"use client";

import { X } from "lucide-react";
import { ProfilePanel } from "@/components/features/profile-form";
import { useTranslations } from "next-intl";

// ProfileDialog is the foreground modal triggered from the chat sidebar —
// keeps regular users out of the backend dashboard. It hosts the personal
// center (profile / password / usage) with a vertical section nav.
export function ProfileDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations();
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/40"
        onClick={() => onOpenChange(false)}
        aria-hidden="true"
      />
      {/* Fixed height (capped by viewport) keeps the window stable when
          switching sections; the content column scrolls internally. */}
      <div className="relative w-full max-w-2xl h-[min(520px,90vh)] rounded-lg border bg-background shadow-lg flex flex-col overflow-hidden">
        <div className="flex items-center justify-between px-6 pt-5 pb-3 shrink-0 border-b">
          <h3 className="text-lg font-semibold">{t("profile.title")}</h3>
          <button
            onClick={() => onOpenChange(false)}
            className="text-muted-foreground hover:text-foreground"
            aria-label={t("common.close")}
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <ProfilePanel onSaved={() => onOpenChange(false)} />
      </div>
    </div>
  );
}
