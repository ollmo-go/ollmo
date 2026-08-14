"use client";

import { useEffect } from "react";
import useSWR from "swr";
import { api } from "@/lib/api";
import { useLocaleState } from "@/lib/i18n";
import { isAuthenticated } from "@/lib/auth";

// useUserLocaleSync applies the logged-in user's stored language preference
// to the UI locale on load. An explicit backend preference ("en"/"zh") wins
// over the local default; an empty preference leaves the local choice alone.
// Uses the shared "profile" SWR key so it reuses the cached request.
export function useUserLocaleSync() {
  const authed = typeof window !== "undefined" && isAuthenticated();
  const { data: profile } = useSWR(authed ? "profile" : null, () => api.getProfile());
  const { setLocale } = useLocaleState();

  useEffect(() => {
    if (profile?.language === "en" || profile?.language === "zh") {
      setLocale(profile.language);
    }
  }, [profile?.language, setLocale]);
}
