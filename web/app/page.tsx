"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { useTranslations } from "next-intl";
import { useAuthStore, isAuthenticated, decodeToken } from "@/lib/auth";
import { api } from "@/lib/api";
import { Logo } from "@/components/brand/logo";
import { ChatApp } from "@/components/chat/chat-app";
import { LanguageToggle } from "@/components/features/language-toggle";
import { useUserLocaleSync } from "@/lib/use-user-locale-sync";

export default function Home() {
  const router = useRouter();
  const t = useTranslations();
  const token = useAuthStore((s) => s.token);
  const [checked, setChecked] = useState(false);
  const [authed, setAuthed] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);
  const { data: profile } = useSWR(authed ? "profile" : null, () => api.getProfile());
  // Apply the logged-in user's stored language preference.
  useUserLocaleSync();

  useEffect(() => {
    if (token && isAuthenticated()) {
      setAuthed(true);
      const payload = decodeToken(token);
      setIsAdmin(payload?.role === "admin");
      setChecked(true);
      return;
    }
    setAuthed(false);
    (async () => {
      try {
        const st = await api.installStatus();
        if (!st.installed) {
          router.replace("/install");
          return;
        }
      } catch {
        // API unreachable; show the page normally.
      }
      setChecked(true);
    })();
  }, [token, router]);

  if (!checked) return null;

  return (
    <div className="flex flex-col h-screen">
      <header className="flex items-center justify-between px-4 h-14 border-b shrink-0">
        {/* Plain anchor on purpose: clicking the logo re-requests the
            homepage, resetting any open conversation. A next/link to the
            current route is a no-op and would keep the chat state. */}
        <a href="/" className="hover:opacity-80 transition-opacity">
          <Logo />
        </a>
        <LanguageToggle />
      </header>

      <main className="flex-1 min-h-0 p-4">
        <ChatApp authed={authed} profile={profile} isAdmin={isAdmin} />
      </main>
    </div>
  );
}
