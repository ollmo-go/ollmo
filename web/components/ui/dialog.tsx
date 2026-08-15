"use client";

import { X } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";

// Centered modal over a dimmed backdrop, mirroring the Drawer's styling.
export function Dialog({
  title,
  description,
  children,
  footer,
  onClose,
  width = "max-w-md",
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  onClose: () => void;
  width?: string;
}) {
  const t = useTranslations();
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`relative w-full ${width} rounded-lg border border-input bg-background shadow-xl animate-in fade-in zoom-in-95`}
      >
        <div className="flex items-start justify-between border-b px-5 py-4">
          <div>
            <h3 className="font-semibold">{title}</h3>
            {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
          </div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label={t("common.close")}>
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="max-h-[70vh] overflow-y-auto p-5">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t px-5 py-4">{footer}</div>}
      </div>
    </div>
  );
}
