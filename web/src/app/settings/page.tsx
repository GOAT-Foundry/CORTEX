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
import { KeyRound, Plug, Wrench } from "lucide-react";

export default function SettingsPage() {
  const { dict } = useI18n();

  const presets = [
    {
      name: "GitHub Copilot (Native)",
      endpoints: "/chat/completions · /responses · /v1/messages · /models",
      badge: "OAuth Device Flow",
    },
    {
      name: "OpenAI",
      endpoints: "/chat/completions · /responses",
      badge: "API Key",
    },
    {
      name: "Anthropic Claude",
      endpoints: "/v1/messages",
      badge: "API Key",
    },
    {
      name: "Google Gemini / OpenRouter / Ollama",
      endpoints: "OpenAI Compatible",
      badge: "Preset",
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
            <KeyRound className="h-4 w-4 text-sky-400" />
            GitHub Copilot Native &amp; LLM Provider Presets
          </CardTitle>
          <CardDescription>
            Automatic session token exchange, provider failover, and token/cost
            telemetry
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button>{dict.settings.connectCopilot}</Button>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {presets.map((p) => (
              <div
                key={p.name}
                className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3 flex items-center justify-between"
              >
                <div>
                  <div className="text-sm font-medium text-zinc-200">
                    {p.name}
                  </div>
                  <div className="text-xs text-zinc-500">{p.endpoints}</div>
                </div>
                <Badge>{p.badge}</Badge>
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
              Skills &amp; MCP Connections
            </CardTitle>
            <CardDescription>
              skills/*/SKILL.md packages &amp; Model Context Protocol servers
            </CardDescription>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Wrench className="h-4 w-4 text-sky-400" />
              Custom Tools &amp; Web Search (ddgs)
            </CardTitle>
            <CardDescription>
              Python / Bash / HTTP custom tools and keyless DuckDuckGo search
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    </div>
  );
}
