"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { BookOpen, BarChart3, Cpu, Key, LayoutDashboard, ScrollText, Settings, ShieldAlert, UserCog, Users, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { api } from "@/lib/api";
import { useTranslations } from "next-intl";
import { decodeToken, getToken } from "@/lib/auth";
import { Logo } from "@/components/brand/logo";
import { UserMenu } from "@/components/features/user-menu";

type NavItem = {
  href: string;
  labelKey: string;
  icon: typeof Key;
  color?: string;
  exact?: boolean;
};

// NavGroup is one labeled section of the sidebar. The first group has no
// label (the dashboard home sits on top); the system group gets the amber
// super-admin styling instead of the muted section header.
type NavGroup = {
  labelKey?: string;
  system?: boolean;
  items: NavItem[];
};

// Icon colors keyed by token. Tailwind needs static class names.
const ICON_COLOR: Record<string, string> = {
  blue: "text-teal-500 dark:text-teal-400",
  green: "text-green-500 dark:text-green-400",
  purple: "text-purple-500 dark:text-purple-400",
  orange: "text-orange-500 dark:text-orange-400",
  cyan: "text-cyan-500 dark:text-cyan-400",
  indigo: "text-indigo-500 dark:text-indigo-400",
  amber: "text-amber-500 dark:text-amber-400",
  rose: "text-rose-500 dark:text-rose-400",
};

export function DashboardSidebar({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  const [isAdmin, setIsAdmin] = useState(false);
  const [isSuperAdmin, setIsSuperAdmin] = useState(false);
  const t = useTranslations();
  const { data: profile } = useSWR("profile", () => api.getProfile());

  // Derive admin/super-admin status from the JWT payload.
  useEffect(() => {
    const token = getToken();
    if (token) {
      const payload = decodeToken(token);
      setIsAdmin(payload?.role === "admin");
      setIsSuperAdmin(payload?.is_super_admin ?? false);
    }
  }, []);

  // Backend management nav grouped by semantics. The dashboard itself is
  // admin/super-admin only (the layout gate redirects members), so the
  // workspace/config/admin groups are gated on canManage.
  // Chat is in the frontend product (/), not in the dashboard.
  const canManage = isAdmin || isSuperAdmin;
  const groups: NavGroup[] = [
    {
      items: [
        { href: "/dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard, color: "blue", exact: true },
      ],
    },
    ...(canManage
      ? [
          {
            labelKey: "nav.group_workspace",
            items: [
              { href: "/dashboard/knowledge-bases", labelKey: "nav.knowledge_bases", icon: BookOpen, color: "green" },
            ],
          },
          {
            labelKey: "nav.group_config",
            items: [
              { href: "/dashboard/settings/models", labelKey: "nav.models", icon: Cpu, color: "orange" },
              { href: "/dashboard/settings/api-keys", labelKey: "nav.api_keys", icon: Key, color: "cyan" },
            ],
          },
          {
            labelKey: "nav.group_admin",
            items: [
              { href: "/dashboard/tenant", labelKey: "nav.my_tenant", icon: Users, color: "indigo" },
              // Analytics hosts both the overview and the usage & cost tab
              // (the standalone bills page redirects here).
              { href: "/dashboard/analytics", labelKey: "nav.analytics", icon: BarChart3, color: "purple" },
              { href: "/dashboard/audit", labelKey: "nav.audit", icon: ScrollText, color: "amber" },
            ],
          },
        ]
      : []),
    ...(isSuperAdmin
      ? [{
          labelKey: "nav.system_admin",
          system: true,
          items: [
            { href: "/dashboard/system-settings", labelKey: "nav.system_settings", icon: Settings },
            { href: "/dashboard/system-tenants", labelKey: "nav.tenants", icon: Users },
            { href: "/dashboard/system-users", labelKey: "nav.system_users", icon: UserCog },
          ],
        } as NavGroup]
      : []),
  ];

  return (
    <aside className="w-64 border-r bg-muted/30 p-4 flex flex-col sticky top-0 h-screen">
      <div className="mb-4 px-2 flex items-center justify-between">
        {/* The logo leads back to the product (chat) home; the dashboard
            itself is reachable through the "dashboard" nav item above. */}
        <Link
          href="/"
          className="hover:opacity-80 transition-opacity"
          onClick={onNavigate}
        >
          <Logo forceDefault />
        </Link>
        {onNavigate && (
          <button
            onClick={onNavigate}
            className="md:hidden text-muted-foreground hover:text-foreground"
            aria-label={t("common.close")}
          >
            <X className="h-5 w-5" />
          </button>
        )}
      </div>
      <nav className="space-y-1 flex-1 min-h-0 overflow-y-auto">
        {groups.map((group, gi) => (
          <div
            key={gi}
            className={cn(
              "space-y-1",
              gi > 0 && "pt-2",
              group.system &&
                "mt-1 rounded-lg border border-amber-200 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/30 p-2"
            )}
          >
            {group.labelKey && (
              <p
                className={cn(
                  "px-2 pb-1 text-xs font-semibold",
                  group.system
                    ? "flex items-center gap-1.5 text-amber-700 dark:text-amber-400"
                    : "uppercase tracking-wide text-muted-foreground"
                )}
              >
                {group.system && <ShieldAlert className="h-3.5 w-3.5" />}
                {t(group.labelKey)}
              </p>
            )}
            {group.items.map((item) => {
              const Icon = item.icon;
              const active = item.exact
                ? pathname === item.href
                : pathname === item.href || pathname?.startsWith(item.href + "/");
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onNavigate}
                  className={cn(
                    "flex items-center gap-2 rounded-md px-3 py-1.5 text-sm",
                    active
                      ? "bg-accent text-primary font-medium"
                      : group.system
                        ? "text-amber-700 dark:text-amber-400 hover:bg-amber-100/60 dark:hover:bg-amber-900/30"
                        : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
                  )}
                >
                  <Icon className={cn("h-4 w-4 shrink-0", active ? "text-primary" : ICON_COLOR[item.color ?? "blue"])} />
                  {t(item.labelKey)}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
      <UserMenu
        variant="dashboard"
        profile={profile}
        onNavigate={onNavigate}
        onLogout={() => api.logout()}
      />
    </aside>
  );
}
