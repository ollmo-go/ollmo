"use client";

import { cn } from "@/lib/utils";
import { useSiteName, useSiteSettings } from "@/lib/use-site-settings";

export function Logo({
  size = "h-7 w-7",
  showName = true,
  nameClass = "text-xl font-bold leading-tight",
  className,
  forceDefault = false,
}: {
  size?: string;
  showName?: boolean;
  nameClass?: string;
  className?: string;
  // The admin console always keeps the built-in ollmo mark, regardless of
  // the custom site logo configured for the public site.
  forceDefault?: boolean;
}) {
  const siteName = useSiteName();
  const { settings } = useSiteSettings();
  const logoUrl = forceDefault ? "" : settings?.site_logo || "";

  return (
    <div className={cn("flex items-center gap-2", className)}>
      {logoUrl ? (
        // Render as <img>: SVG in this context executes no scripts, and the
        // data URL avoids all browser caching issues when the logo changes.
        <img
          src={logoUrl}
          alt={siteName}
          className={cn("shrink-0 object-contain", size)}
          aria-hidden="true"
        />
      ) : (
        <svg
          viewBox="0 0 1024 1024"
          className={cn("shrink-0", size)}
          fill="#3370FF"
          aria-hidden="true"
        >
          <path d="M256 576H128v320h128V576z m128 0l128 134.4 128-128V448h384v576h-384v-256l-38.4 38.4L512 889.6l-128-128V1024H0V448h384v128z m512 0h-128v320h128V576z m128-384c0 108.8-83.2 192-192 192s-192-83.2-192-192 83.2-192 192-192 192 83.2 192 192z m-128 0c0-38.4-25.6-64-64-64s-64 25.6-64 64 25.6 64 64 64 64-25.6 64-64zM384 192c0 108.8-83.2 192-192 192S0 300.8 0 192s83.2-192 192-192 192 83.2 192 192zM256 192c0-38.4-25.6-64-64-64s-64 25.6-64 64 25.6 64 64 64 64-25.6 64-64z" />
        </svg>
      )}
      {showName && <h2 className={nameClass}>{siteName}</h2>}
    </div>
  );
}
