"use client";

import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import { Menu } from "lucide-react";
import { DashboardSidebar } from "@/components/features/dashboard-sidebar";
import { isAuthenticated, decodeToken, getToken } from "@/lib/auth";
import { useSiteName } from "@/lib/use-site-settings";
import { useUserLocaleSync } from "@/lib/use-user-locale-sync";
import { useTranslations } from "next-intl";

// Client-side gate. The dashboard is a team-management area: only admins
// (and super admins) may enter. Unauthenticated users go to /login; regular
// members go back to the chat home.
export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useTranslations();
  const siteName = useSiteName();
  const year = new Date().getFullYear();
  const [ready, setReady] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // Apply the logged-in user's stored language preference.
  useUserLocaleSync();

  useEffect(() => {
    if (!isAuthenticated()) {
      router.replace("/login");
      return;
    }
    const token = getToken();
    const payload = token ? decodeToken(token) : null;
    const isSuperAdmin = payload?.is_super_admin ?? false;
    if (payload?.role !== "admin" && !isSuperAdmin) {
      router.replace("/");
      return;
    }
    setReady(true);
  }, [router]);

  // Close the mobile sidebar whenever the route changes.
  useEffect(() => {
    setSidebarOpen(false);
  }, [pathname]);

  if (!ready) return null;

  return (
    <div className="flex min-h-screen">
      {/* Desktop sidebar */}
      <div className="hidden md:block">
        <DashboardSidebar />
      </div>

      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div
            className="absolute inset-0 bg-black/40"
            onClick={() => setSidebarOpen(false)}
            aria-hidden="true"
          />
          <div className="relative h-full">
            <DashboardSidebar onNavigate={() => setSidebarOpen(false)} />
          </div>
        </div>
      )}

      <div className="flex-1 flex flex-col min-w-0">
        {/* Mobile top bar */}
        <header className="md:hidden flex items-center gap-3 border-b px-4 h-12 shrink-0">
          <button
            onClick={() => setSidebarOpen(true)}
            className="text-muted-foreground hover:text-foreground"
            aria-label={siteName}
          >
            <Menu className="h-5 w-5" />
          </button>
          <span className="font-semibold">{siteName}</span>
        </header>

        <main className="flex-1 p-4 md:p-6 min-w-0">{children}</main>

        <footer className="shrink-0 border-t px-4 py-3 text-center text-xs text-muted-foreground">
          <span>{t("footer_copyright", { year })}</span>
          <span className="mx-2">·</span>
          <a
            href="https://ollmo.com/"
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-foreground"
          >
            {t("footer_site")}
          </a>
        </footer>
      </div>
    </div>
  );
}
