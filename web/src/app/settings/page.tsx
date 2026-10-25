"use client";

import React, { useEffect, useState } from "react";
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
import {
  Activity,
  CheckCircle2,
  Globe,
  KeyRound,
  Plug,
  RefreshCw,
  Server,
  Wrench,
} from "lucide-react";

interface ProviderConfig {
  id: string;
  name: string;
  format: string;
  base_url: string;
  api_key?: string;
  has_api_key: boolean;
  default_model: string;
  auth_type: string;
  enabled: boolean;
  priority: number;
  input_cost_per_1m: number;
  output_cost_per_1m: number;
}

interface CopilotStatus {
  connected: boolean;
  has_oauth_token: boolean;
  session_valid: boolean;
  api_endpoint: string;
}

interface CopilotDeviceCode {
  device_code: string;
  user_code: string;
  verification_uri: string;
  interval: number;
}

interface CopilotModel {
  id: string;
  name: string;
  vendor: string;
  wire_formats: string[];
}

interface UsageSummary {
  total_calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  total_cost_usd: number;
  by_provider: Array<{
    provider_id: string;
    model: string;
    calls: number;
    input_tokens: number;
    output_tokens: number;
    cost_usd: number;
  }>;
}

export default function SettingsPage() {
  const { dict, locale, directive } = useI18n();

  const [configs, setConfigs] = useState<ProviderConfig[]>([]);
  const [copilot, setCopilot] = useState<CopilotStatus>({
    connected: false,
    has_oauth_token: false,
    session_valid: false,
    api_endpoint: "https://api.githubcopilot.com",
  });
  const [deviceCode, setDeviceCode] = useState<CopilotDeviceCode | null>(null);
  const [copilotModels, setCopilotModels] = useState<CopilotModel[]>([]);
  const [usage, setUsage] = useState<UsageSummary | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>("");

  const loadAll = () => {
    fetch("/api/providers")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (!data) return;
        if (Array.isArray(data.configs)) setConfigs(data.configs);
        if (data.copilot) setCopilot(data.copilot);
      })
      .catch(() => {});

    fetch("/api/usage")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data) setUsage(data);
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadAll();
  }, []);

  const handleStartDeviceFlow = async () => {
    setStatusMessage("");
    try {
      const res = await fetch("/api/providers/copilot/device-code", {
        method: "POST",
      });
      const data = await res.json();
      if (res.ok && data.user_code) {
        setDeviceCode(data);
      } else {
        setStatusMessage(data.error || "Failed to start device flow");
      }
    } catch {
      setStatusMessage("Network error starting device flow");
    }
  };

  const handlePollDeviceFlow = async () => {
    if (!deviceCode) return;
    try {
      const res = await fetch("/api/providers/copilot/poll", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ device_code: deviceCode.device_code }),
      });
      const data = await res.json();
      if (data.status === "authorized") {
        setDeviceCode(null);
        setStatusMessage("GitHub Copilot authorized!");
        loadAll();
      } else {
        setStatusMessage(`Status: ${data.status} ${data.error || ""}`);
      }
    } catch {
      setStatusMessage("Failed to poll authorization status");
    }
  };

  const handleDisconnectCopilot = async () => {
    await fetch("/api/providers/copilot/auth", { method: "DELETE" }).catch(
      () => {}
    );
    setDeviceCode(null);
    loadAll();
  };

  const handleFetchCopilotModels = async () => {
    try {
      const res = await fetch("/api/providers/copilot/models");
      const data = await res.json();
      if (Array.isArray(data.models)) {
        setCopilotModels(data.models);
      }
    } catch {
      // ignore
    }
  };

  const handleUpdateField = (
    id: string,
    field: keyof ProviderConfig,
    value: string | number | boolean
  ) => {
    setConfigs((prev) =>
      prev.map((c) => (c.id === id ? { ...c, [field]: value } : c))
    );
  };

  const handleSaveProvider = async (cfg: ProviderConfig) => {
    setStatusMessage("");
    try {
      const res = await fetch(`/api/providers/${cfg.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(cfg),
      });
      if (res.ok) {
        setStatusMessage(`Saved ${cfg.name}`);
        loadAll();
      }
    } catch {
      setStatusMessage(`Failed to save ${cfg.name}`);
    }
  };

  const handleTestProvider = async (id: string) => {
    setStatusMessage("");
    try {
      const res = await fetch("/api/providers/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id }),
      });
      const data = await res.json();
      setStatusMessage(
        `${id}: ${data.message || (data.ok ? "OK" : data.error)}`
      );
    } catch {
      setStatusMessage(`${id}: connection test failed`);
    }
  };

  return (
    <div className="max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">{dict.settings.title}</h1>
          <p className="text-sm text-zinc-400">{dict.settings.subtitle}</p>
        </div>
        {statusMessage && (
          <Badge className="border-sky-500/40 bg-sky-500/10 text-sky-300">
            {statusMessage}
          </Badge>
        )}
      </div>

      {/* Active Locale & Agent Output Language Sync */}
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

      {/* GitHub Copilot Native Integration */}
      <Card>
        <CardHeader className="flex flex-row items-start justify-between">
          <div className="space-y-1">
            <CardTitle className="text-base flex items-center gap-2">
              <KeyRound className="h-4 w-4 text-sky-400" />
              {dict.settings.copilotCardTitle}
            </CardTitle>
            <CardDescription>
              {dict.settings.copilotCardSubtitle}
            </CardDescription>
          </div>
          <Badge
            className={
              copilot.connected
                ? "border-emerald-500/40 bg-emerald-500/15 text-emerald-300"
                : "border-zinc-700 bg-zinc-800 text-zinc-400"
            }
          >
            <CheckCircle2 className="h-3 w-3 mr-1" />
            {copilot.connected
              ? dict.settings.connectedBadge
              : dict.settings.disconnectedBadge}
          </Badge>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button onClick={handleStartDeviceFlow}>
              {dict.settings.connectCopilot}
            </Button>
            <Button variant="outline" onClick={handleFetchCopilotModels}>
              <RefreshCw className="h-3.5 w-3.5" />
              {dict.settings.fetchModels}
            </Button>
            {copilot.connected && (
              <Button variant="ghost" onClick={handleDisconnectCopilot}>
                {dict.settings.disconnectCopilot}
              </Button>
            )}
          </div>

          {deviceCode && (
            <div className="rounded-lg border border-sky-500/40 bg-sky-500/10 p-4 space-y-3">
              <div className="text-xs text-zinc-300">
                {dict.settings.deviceCodePrompt}
              </div>
              <div className="text-2xl font-mono font-bold tracking-widest text-sky-300">
                {deviceCode.user_code}
              </div>
              <div className="flex items-center gap-3">
                <a
                  href={deviceCode.verification_uri}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs underline text-sky-400 hover:text-sky-300"
                >
                  {dict.settings.openGithub} ({deviceCode.verification_uri})
                </a>
                <Button size="sm" onClick={handlePollDeviceFlow}>
                  {dict.settings.pollCopilot}
                </Button>
              </div>
            </div>
          )}

          {copilotModels.length > 0 && (
            <div className="space-y-2 pt-2">
              <div className="text-xs font-medium text-zinc-400">
                {dict.settings.modelsLoaded}:
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-2">
                {copilotModels.map((m) => (
                  <div
                    key={m.id}
                    className="rounded-md border border-zinc-800 bg-zinc-950 p-2.5 flex flex-col justify-between gap-1"
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-zinc-200">
                        {m.name}
                      </span>
                      <Badge className="text-[10px]">{m.vendor}</Badge>
                    </div>
                    <div className="text-[11px] text-zinc-500 font-mono">
                      {m.wire_formats.join(" · ")}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Provider Profiles & Failover Chain */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="h-4 w-4 text-sky-400" />
            {dict.settings.providersTitle}
          </CardTitle>
          <CardDescription>{dict.settings.providersSubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {configs.map((cfg) => (
            <div
              key={cfg.id}
              className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-4 space-y-3"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <span className="font-semibold text-sm text-zinc-100">
                    {cfg.name}
                  </span>
                  <Badge className="font-mono text-[11px]">{cfg.id}</Badge>
                  <Badge
                    className={
                      cfg.enabled
                        ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
                        : "text-zinc-500"
                    }
                  >
                    {dict.settings.priorityLabel} #{cfg.priority}
                  </Badge>
                </div>
                <div className="flex items-center gap-2">
                  <label className="flex items-center gap-1.5 text-xs text-zinc-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={cfg.enabled}
                      onChange={(e) =>
                        handleUpdateField(cfg.id, "enabled", e.target.checked)
                      }
                    />
                    {dict.settings.enabledLabel}
                  </label>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => handleTestProvider(cfg.id)}
                  >
                    {dict.settings.testBtn}
                  </Button>
                  <Button size="sm" onClick={() => handleSaveProvider(cfg)}>
                    {dict.settings.saveBtn}
                  </Button>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-4 gap-2 text-xs">
                <div>
                  <label className="block text-zinc-500 mb-1">
                    {dict.settings.formatLabel}
                  </label>
                  <select
                    value={cfg.format}
                    onChange={(e) =>
                      handleUpdateField(cfg.id, "format", e.target.value)
                    }
                    className="w-full rounded border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-zinc-200"
                  >
                    <option value="openai">
                      openai (/chat/completions)
                    </option>
                    <option value="openai-responses">
                      openai-responses (/responses)
                    </option>
                    <option value="anthropic">anthropic (/v1/messages)</option>
                  </select>
                </div>
                <div>
                  <label className="block text-zinc-500 mb-1">
                    {dict.settings.modelLabel}
                  </label>
                  <input
                    value={cfg.default_model}
                    onChange={(e) =>
                      handleUpdateField(cfg.id, "default_model", e.target.value)
                    }
                    className="w-full rounded border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-zinc-200"
                  />
                </div>
                <div>
                  <label className="block text-zinc-500 mb-1">
                    {dict.settings.baseUrlLabel}
                  </label>
                  <input
                    value={cfg.base_url}
                    onChange={(e) =>
                      handleUpdateField(cfg.id, "base_url", e.target.value)
                    }
                    className="w-full rounded border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-zinc-200"
                  />
                </div>
                <div>
                  <label className="block text-zinc-500 mb-1">
                    {dict.settings.priorityLabel}
                  </label>
                  <input
                    type="number"
                    min={1}
                    value={cfg.priority}
                    onChange={(e) =>
                      handleUpdateField(
                        cfg.id,
                        "priority",
                        Number(e.target.value) || 1
                      )
                    }
                    className="w-full rounded border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-zinc-200"
                  />
                </div>
              </div>

              {cfg.auth_type === "api_key" && (
                <div className="text-xs">
                  <label className="block text-zinc-500 mb-1">
                    {dict.settings.apiKeyLabel}
                  </label>
                  <input
                    type="password"
                    value={cfg.api_key || ""}
                    placeholder={dict.settings.apiKeyPlaceholder}
                    onChange={(e) =>
                      handleUpdateField(cfg.id, "api_key", e.target.value)
                    }
                    className="w-full rounded border border-zinc-800 bg-zinc-900 px-2 py-1.5 text-zinc-200"
                  />
                </div>
              )}
            </div>
          ))}
        </CardContent>
      </Card>

      {/* Token & Cost Telemetry */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Activity className="h-4 w-4 text-sky-400" />
            {dict.settings.telemetryTitle}
          </CardTitle>
          <CardDescription>{dict.settings.telemetrySubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3">
              <div className="text-xs text-zinc-500">
                {dict.settings.totalCalls}
              </div>
              <div className="text-lg font-bold text-zinc-100">
                {usage?.total_calls ?? 0}
              </div>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3">
              <div className="text-xs text-zinc-500">
                {dict.settings.inputTokens}
              </div>
              <div className="text-lg font-bold text-zinc-100">
                {(usage?.input_tokens ?? 0).toLocaleString()}
              </div>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3">
              <div className="text-xs text-zinc-500">
                {dict.settings.outputTokens}
              </div>
              <div className="text-lg font-bold text-zinc-100">
                {(usage?.output_tokens ?? 0).toLocaleString()}
              </div>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3">
              <div className="text-xs text-zinc-500">
                {dict.settings.totalCost}
              </div>
              <div className="text-lg font-bold text-emerald-400">
                ${(usage?.total_cost_usd ?? 0).toFixed(4)}
              </div>
            </div>
          </div>
          {(!usage || usage.total_calls === 0) && (
            <p className="text-xs text-zinc-500">{dict.settings.noUsageYet}</p>
          )}
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
