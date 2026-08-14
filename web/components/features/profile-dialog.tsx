"use client";

import { X } from "lucide-react";
import { ProfileEditor } from "@/components/features/profile-form";
import { useTranslations } from "next-intl";

// ProfileDialog is the foreground modal for editing the current user's
// name and password. Triggered from the chat sidebar — keeps regular
// users out of the backend dashboard.
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
      <div className="relative w-full max-w-lg rounded-lg border bg-background p-6 shadow-lg max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">{t("profile.title")}</h3>
          <button
            onClick={() => onOpenChange(false)}
            className="text-muted-foreground hover:text-foreground"
            aria-label={t("common.close")}
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <ProfileEditor onSaved={() => onOpenChange(false)} />
      </div>
    </div>
  );
}
