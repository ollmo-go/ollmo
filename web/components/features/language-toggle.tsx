"use client";

import { useLocaleState } from "@/lib/i18n";
import { cn } from "@/lib/utils";

// LanguageToggle is a compact 中文/EN switch. Uses the locale stored in
// localStorage via useLocaleState, so the choice persists across sessions
// without touching the URL.
export function LanguageToggle({ className }: { className?: string }) {
  const { locale, setLocale } = useLocaleState();
  return (
    <div className={cn("inline-flex items-center rounded-md border text-xs", className)}>
      <button
        onClick={() => setLocale("zh")}
        className={cn(
          "px-2 py-1 transition-colors",
          locale === "zh" ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"
        )}
      >
        中文
      </button>
      <button
        onClick={() => setLocale("en")}
        className={cn(
          "px-2 py-1 transition-colors",
          locale === "en" ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"
        )}
      >
        EN
      </button>
    </div>
  );
}
