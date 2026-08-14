"use client";

import { useEffect } from "react";
import useSWR from "swr";
import { api } from "./api";

export interface SiteSettings {
  site_name: string;
  site_description: string;
  default_language: string;
  timezone: string;
  allow_registration: string;
}

// useSiteSettings fetches public site settings (site name, language, etc.)
// via SWR. The result is cached and shared across all components that call
// this hook, so only one network request is made per page load.
export function useSiteSettings() {
  const { data, mutate } = useSWR<SiteSettings>("site-settings", () => api.getSiteSettings(), {
    revalidateOnFocus: false,
  });
  return { settings: data, mutate };
}

// Helper to get the site name with fallback.
export function useSiteName() {
  const { settings } = useSiteSettings();
  return settings?.site_name || "ollmo";
}

// SiteTitle keeps the browser tab title in sync with the configured site
// name and description: "name - description" when both are set, otherwise
// just the name.
export function SiteTitle() {
  const { settings } = useSiteSettings();
  useEffect(() => {
    const name = settings?.site_name || "ollmo";
    const desc = settings?.site_description || "";
    document.title = desc ? `${name} - ${desc}` : name;
  }, [settings]);
  return null;
}
