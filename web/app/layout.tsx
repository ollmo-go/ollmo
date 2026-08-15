import type { Metadata } from "next";
import { Toaster } from "sonner";
import { SWRConfig } from "swr";
import "./globals.css";
import { I18nProvider } from "@/lib/i18n";
import { ConfirmProvider } from "@/components/ui/confirm";
import { SiteTitle } from "@/lib/use-site-settings";

export const metadata: Metadata = {
  title: "ollmo",
  description: "RAG platform",
  icons: {
    icon: "/favicon.ico?v=2",
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="min-h-screen bg-background font-sans antialiased">
        <SiteTitle />
        <I18nProvider>
          {/* Global SWR defaults: refetching every mounted list on tab focus
              is wasted traffic here — pages refresh via explicit mutate() or
              their own refreshInterval. */}
          <SWRConfig value={{ revalidateOnFocus: false }}>
            <ConfirmProvider>
              {children}
            </ConfirmProvider>
          </SWRConfig>
          <Toaster position="top-center" richColors closeButton />
        </I18nProvider>
      </body>
    </html>
  );
}
