"use client";

import { X } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

export function Drawer({
  title,
  children,
  onClose,
  onSave,
  saving,
  width = "max-w-md",
}: {
  title: string;
  children: React.ReactNode;
  onClose: () => void;
  onSave?: () => void;
  saving?: boolean;
  width?: string;
}) {
  const t = useTranslations();
  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className={`relative h-full w-full ${width} bg-background shadow-xl flex flex-col animate-in slide-in-from-right`}>
        <div className="flex items-center justify-between border-b px-5 py-4">
          <h3 className="font-semibold">{title}</h3>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label={t("common.close")}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-5">
          {children}
        </div>
        {onSave && (
          <div className="flex justify-end gap-2 border-t px-5 py-4">
            <Button variant="outline" onClick={onClose}>
              <X className="h-4 w-4 mr-1" /> {t("common.cancel")}
            </Button>
            <Button onClick={onSave} disabled={saving}>
              {saving ? "..." : t("common.save")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
