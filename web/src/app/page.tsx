"use client";

import React from "react";
import { useI18n } from "@/i18n/provider";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Bot,
  Clock,
  Database,
  Globe,
  MessageSquare,
  Network,
} from "lucide-react";

export default function OverviewPage() {
  const { dict } = useI18n();
  const c = dict.overview.cards;

  const features = [
    { title: c.copilotTitle, desc: c.copilotDesc, icon: Bot },
    { title: c.chatTitle, desc: c.chatDesc, icon: MessageSquare },
    { title: c.tasksTitle, desc: c.tasksDesc, icon: Network },
    { title: c.timersTitle, desc: c.timersDesc, icon: Clock },
    { title: c.collectionsTitle, desc: c.collectionsDesc, icon: Database },
    { title: c.i18nTitle, desc: c.i18nDesc, icon: Globe },
  ];

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="space-y-2">
        <h1 className="text-2xl font-bold tracking-tight">
          {dict.overview.heading}
        </h1>
        <p className="text-zinc-400 text-sm max-w-3xl leading-relaxed">
          {dict.overview.description}
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {features.map((f) => {
          const Icon = f.icon;
          return (
            <Card key={f.title}>
              <CardHeader className="pb-3">
                <div className="h-9 w-9 rounded-lg bg-sky-500/15 border border-sky-500/30 flex items-center justify-center text-sky-400 mb-2">
                  <Icon className="h-4 w-4" />
                </div>
                <CardTitle className="text-base">{f.title}</CardTitle>
              </CardHeader>
              <CardContent>
                <CardDescription className="leading-relaxed">
                  {f.desc}
                </CardDescription>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
