import type { Metadata } from "next";
import "./globals.css";
import { I18nProvider } from "@/i18n/provider";
import { AppShell } from "@/components/app-shell";

export const metadata: Metadata = {
  title: "CORTEX — AI Agent Application Framework",
  description:
    "Clean-room AI Agent Application Framework built with Go, Norma v0.4.5, Next.js, and SQLite.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body>
        <I18nProvider>
          <AppShell>{children}</AppShell>
        </I18nProvider>
      </body>
    </html>
  );
}
