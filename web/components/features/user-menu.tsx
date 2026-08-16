"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import {
  ChevronDown,
  ExternalLink,
  LogOut,
  Settings,
  UserRound,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { UserProfile } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm";
import { logout } from "@/lib/auth";
import { AVATAR_BG } from "@/lib/ui-colors";

function avatarColor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) | 0;
  return AVATAR_BG[(h >>> 0) % AVATAR_BG.length];
}

function initialOf(name: string): string {
  const ch = name.trim()[0];
  return ch ? ch.toUpperCase() : "?";
}

type UserMenuProps = {
  profile?: UserProfile | null;
  // chat: the menu lives in the product sidebar and opens the ProfileDialog;
  // dashboard: it lives in the admin sidebar and links to /dashboard/profile.
  variant: "chat" | "dashboard";
  isAdmin?: boolean;
  onOpenProfile?: () => void;
  onNavigate?: () => void;
  onLogout?: () => void | Promise<void>;
};

// UserMenu is the DeepSeek-style compact account card pinned to the bottom
// of a sidebar: avatar + name + chevron trigger, and an upward-opening menu
// that folds profile, secondary entries and sign-out into one place.
export function UserMenu({
  profile,
  variant,
  isAdmin = false,
  onOpenProfile,
  onNavigate,
  onLogout,
}: UserMenuProps) {
  const t = useTranslations();
  const confirm = useConfirm();
  const [open, setOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  const name = profile?.name?.trim() || t("nav.profile");
  const roleText =
    profile?.role === "admin" ? t("profile.role_admin") : t("profile.role_member");
  const subtitle = [profile?.tenant_name, roleText].filter(Boolean).join(" · ");

  // Close on outside pointer and Escape, matching DeepSeek's popover feel.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent | TouchEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("touchstart", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("touchstart", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  async function handleLogout() {
    const ok = await confirm({
      title: t("nav.sign_out"),
      description: t("nav.sign_out_confirm"),
      confirmText: t("nav.sign_out"),
    });
    if (!ok) return;
    setLoggingOut(true);
    try {
      if (onLogout) await onLogout();
      else logout();
    } finally {
      setLoggingOut(false);
    }
  }

  function closeAnd(fn?: () => void) {
    setOpen(false);
    fn?.();
  }

  const itemClass =
    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/60 hover:text-foreground";

  return (
    <div ref={rootRef} className="relative border-t pt-2">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-accent/60"
      >
        <span
          className={cn(
            "flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold text-white",
            avatarColor(name)
          )}
        >
          {initialOf(name)}
        </span>
        <span className="min-w-0 flex-1 truncate text-left">{name}</span>
        <ChevronDown
          className={cn(
            "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
            open && "rotate-180"
          )}
        />
      </button>

      {open && (
        <div
          role="menu"
          className="absolute bottom-full left-0 right-0 z-50 mb-1 overflow-hidden rounded-xl border bg-background p-1.5 shadow-lg"
        >
          <div className="flex items-center gap-2.5 rounded-lg px-2 py-2">
            <span
              className={cn(
                "flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-semibold text-white",
                avatarColor(name)
              )}
            >
              {initialOf(name)}
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{name}</p>
              {subtitle && (
                <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
              )}
            </div>
          </div>

          <div className="my-1 h-px bg-border" />

          {variant === "chat" ? (
            <>
              <button
                role="menuitem"
                className={itemClass}
                onClick={() => closeAnd(onOpenProfile)}
              >
                <UserRound className="h-4 w-4 shrink-0" />
                {t("nav.profile")}
              </button>
              {isAdmin && (
                <Link
                  role="menuitem"
                  href="/dashboard"
                  className={itemClass}
                  onClick={() => closeAnd(onNavigate)}
                >
                  <Settings className="h-4 w-4 shrink-0" />
                  {t("nav.admin")}
                </Link>
              )}
            </>
          ) : (
            <>
              <Link
                role="menuitem"
                href="/dashboard/profile"
                className={itemClass}
                onClick={() => closeAnd(onNavigate)}
              >
                <UserRound className="h-4 w-4 shrink-0" />
                {t("nav.profile")}
              </Link>
              <Link
                role="menuitem"
                href="/"
                className={itemClass}
                onClick={() => closeAnd(onNavigate)}
              >
                <ExternalLink className="h-4 w-4 shrink-0" />
                {t("nav.back_to_chat")}
              </Link>
            </>
          )}

          <div className="my-1 h-px bg-border" />

          <button
            role="menuitem"
            onClick={handleLogout}
            disabled={loggingOut}
            className={cn(
              itemClass,
              "text-destructive hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/40 dark:hover:text-red-400 disabled:opacity-50"
            )}
          >
            <LogOut className="h-4 w-4 shrink-0" />
            {loggingOut ? t("nav.signing_out") : t("nav.sign_out")}
          </button>
        </div>
      )}
    </div>
  );
}
