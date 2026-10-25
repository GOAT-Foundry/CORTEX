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
import { Badge } from "@/components/ui/badge";
import { Clock } from "lucide-react";

export default function TimersPage() {
  const { dict } = useI18n();

  return (
    <div className="max-w-5xl space-y-6">
      <div>
        <h1 className="text-xl font-bold">{dict.timers.title}</h1>
        <p className="text-sm text-zinc-400">{dict.timers.subtitle}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Clock className="h-4 w-4 text-sky-400" />
            {dict.timers.cardTitle}
          </CardTitle>
          <CardDescription>{dict.timers.cardSubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            <Badge>cron: &quot;0 * * * *&quot;</Badge>
            <Badge>interval: &quot;15m&quot;</Badge>
            <Badge>schedule: &quot;once&quot; + run_at</Badge>
            <Badge className="border-sky-500/30 bg-sky-500/10 text-sky-300">
              {dict.timers.agentToolBadge}
            </Badge>
          </div>
          <p className="text-sm text-zinc-400 pt-2">{dict.timers.empty}</p>
        </CardContent>
      </Card>
    </div>
  );
}
