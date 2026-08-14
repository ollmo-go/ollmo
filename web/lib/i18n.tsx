"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { NextIntlClientProvider } from "next-intl";
import en from "@/messages/en.json";
import zh from "@/messages/zh.json";

type Locale = "en" | "zh";

const messages: Record<Locale, Record<string, unknown>> = { en, zh };
const LOCALE_STORAGE_KEY = "locale";

interface LocaleContextType {
  locale: Locale;
  setLocale: (l: Locale) => void;
}

const LocaleContext = createContext<LocaleContextType | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => {
    try {
      const saved = localStorage.getItem(LOCALE_STORAGE_KEY);
      if (saved === "en" || saved === "zh") {
        setLocaleState(saved);
      }
    } catch {
      // localStorage may be unavailable — fall back to "en".
    }
    setHydrated(true);
  }, []);

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
    try {
      localStorage.setItem(LOCALE_STORAGE_KEY, l);
    } catch {
      // ignore write failures
    }
  }, []);

  const effectiveLocale = hydrated ? locale : "en";

  return (
    <NextIntlClientProvider
      locale={effectiveLocale}
      messages={messages[effectiveLocale]}
      timeZone="Asia/Shanghai"
    >
      <LocaleContext.Provider value={{ locale: effectiveLocale, setLocale }}>
        {children}
      </LocaleContext.Provider>
    </NextIntlClientProvider>
  );
}

export function useLocaleState() {
  const ctx = useContext(LocaleContext);
  if (!ctx) {
    throw new Error("useLocaleState must be used within I18nProvider");
  }
  return ctx;
}

export type { Locale };
