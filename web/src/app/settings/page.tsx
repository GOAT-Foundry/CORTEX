"use client";

import React from "react";
import { useI18n } from "@/i18n/provider";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Globe, KeyRound, Plug, Wrench } from "lucide-react";

export default function SettingsPage() {
  const { dict, locale, directive } = useI18n();
  const p = dict.settings.presets;

  const presets = [
    {
      name: p.copilotName,
      endpoints: "/chat/completions · /responses · /v1/messages · /models",
      badge: p.copilotBadge,
    },
    {
      name: p.openaiName,
      endpoints: "/chat/completions · /responses",
      badge: p.openaiBadge,
    },
    {
      name: p.anthropicName,
      endpoints: "/v1/messages",
      badge: p.anthropicBadge,
    },
    {
      name: p.othersName,
      endpoints: p.othersEndpoints,
      badge: p.othersBadge,
    },
  ];

  return (
    <div className="max-w-5xl space-y-6">
      <div>
        <h1 className="text-xl font-bold">{dict.settings.title}</h1>
        <p className="text-sm text-zinc-400">{dict.settings.subtitle}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Globe className="h-4 w-4 text-sky-400" />
            {dict.app.agentDirectiveTitle} ({locale.toUpperCase()})
          </CardTitle>
          <CardDescription>{dict.app.agentSyncNotice}</CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-xs text-sky-300 whitespace-pre-wrap">
            {directive}
          </pre>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <KeyRound className="h-4 w-4 text-sky-400" />
            {dict.settings.copilotCardTitle}
          </CardTitle>
          <CardDescription>{dict.settings.copilotCardSubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button>{dict.settings.connectCopilot}</Button>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {presets.map((item) => (
              <div
                key={item.name}
                className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3 flex items-center justify-between"
              >
                <div>
                  <div className="text-sm font-medium text-zinc-200">
                    {item.name}
                  </div>
                  <div className="text-xs text-zinc-500">{item.endpoints}</div>
                </div>
                <Badge>{item.badge}</Badge>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Plug className="h-4 w-4 text-sky-400" />
              {dict.settings.skillsCardTitle}
            </CardTitle>
            <CardDescription>
              {dict.settings.skillsCardSubtitle}
            </CardDescription>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Wrench className="h-4 w-4 text-sky-400" />
              {dict.settings.toolsCardTitle}
            </CardTitle>
            <CardDescription>{dict.settings.toolsCardSubtitle}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    </div>
  );
}
