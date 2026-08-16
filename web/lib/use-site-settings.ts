"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import useSWR from "swr";
import { api } from "./api";

export interface SiteSettings {
  site_name: string;
  site_description: string;
  default_language: string;
  timezone: string;
  allow_registration: string;
  // Custom site logo as a data URL; empty means the built-in default logo.
  site_logo?: string;
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
// just the name. Next.js resets document.title to the layout metadata on
// every route change, so we re-apply whenever the pathname changes.
export function SiteTitle() {
  const { settings } = useSiteSettings();
  const pathname = usePathname();
  useEffect(() => {
    const name = settings?.site_name || "ollmo";
    const desc = settings?.site_description || "";
    const title = desc ? `${name} - ${desc}` : name;
    // Defer past Next.js's own metadata commit for this route.
    const t = setTimeout(() => {
      document.title = title;
    }, 0);
    return () => clearTimeout(t);
  }, [settings, pathname]);
  return null;
}
