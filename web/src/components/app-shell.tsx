"use client";

import React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Bot,
  Clock,
  Database,
  Globe,
  LayoutDashboard,
  MessageSquare,
  Network,
  Settings,
} from "lucide-react";
import { useI18n } from "@/i18n/provider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { locale, dict, setLocale } = useI18n();

  const navItems = [
    { href: "/", label: dict.nav.overview, icon: LayoutDashboard },
    { href: "/chat", label: dict.nav.chat, icon: MessageSquare },
    { href: "/tasks", label: dict.nav.tasks, icon: Network },
    { href: "/timers", label: dict.nav.timers, icon: Clock },
    { href: "/collections", label: dict.nav.collections, icon: Database },
    { href: "/settings", label: dict.nav.settings, icon: Settings },
  ];

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 flex">
      <aside className="w-64 border-r border-zinc-800/80 bg-zinc-900/40 flex flex-col justify-between p-4">
        <div>
          <div className="flex items-center gap-2.5 px-2 py-3">
            <div className="h-8 w-8 rounded-lg bg-sky-500/20 border border-sky-500/40 flex items-center justify-center text-sky-400">
              <Bot className="h-5 w-5" />
            </div>
            <div>
              <div className="font-bold tracking-wide text-zinc-100">
                {dict.app.title}
              </div>
              <div className="text-[11px] text-zinc-400">
                {dict.app.sdkBadge}
              </div>
            </div>
          </div>

          <nav className="mt-6 space-y-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active =
                item.href === "/"
                  ? pathname === "/"
                  : pathname.startsWith(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    active
                      ? "bg-sky-500/15 text-sky-300 border border-sky-500/30"
                      : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-100"
                  )}
                >
                  <Icon className="h-4 w-4" />
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </nav>
        </div>

        <div className="rounded-lg border border-zinc-800 bg-zinc-900/70 p-3 text-xs text-zinc-400 space-y-2">
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1.5 text-zinc-300 font-medium">
              <Globe className="h-3.5 w-3.5 text-sky-400" />
              {dict.app.languageLabel}
            </span>
            <Badge className="uppercase">{locale}</Badge>
          </div>
          <div className="grid grid-cols-2 gap-1.5">
            <Button
              size="sm"
              variant={locale === "en" ? "default" : "outline"}
              onClick={() => setLocale("en")}
            >
              English
            </Button>
            <Button
              size="sm"
              variant={locale === "ja" ? "default" : "outline"}
              onClick={() => setLocale("ja")}
            >
              日本語
            </Button>
          </div>
          <p className="text-[11px] leading-relaxed text-zinc-500">
            {dict.app.agentSyncNotice}
          </p>
        </div>
      </aside>

      <main className="flex-1 flex flex-col min-w-0">
        <header className="h-14 border-b border-zinc-800/80 px-6 flex items-center justify-between bg-zinc-950/80">
          <div className="text-sm text-zinc-400">{dict.app.subtitle}</div>
          <div className="flex items-center gap-2">
            <Badge className="border-emerald-500/30 bg-emerald-500/10 text-emerald-300">
              {dict.app.singleBinaryBadge}
            </Badge>
            <Badge className="border-sky-500/30 bg-sky-500/10 text-sky-300">
              {dict.app.copilotNativeBadge}
            </Badge>
          </div>
        </header>
        <div className="flex-1 p-6 overflow-y-auto">{children}</div>
      </main>
    </div>
  );
}
