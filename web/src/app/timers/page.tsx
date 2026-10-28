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
import { Clock, History, Play, Plus, RefreshCw, Trash2 } from "lucide-react";

interface TimerEntry {
  id: string;
  name: string;
  schedule: string;
  run_at?: string;
  prompt: string;
  agent_id: string;
  enabled: boolean;
  created_by: string;
  last_run_at?: string;
  next_run_at?: string;
  last_status?: string;
}

interface TimerRunEntry {
  id: number;
  timer_id: string;
  timer_name: string;
  trigger: string;
  status: string;
  output?: string;
  error?: string;
  duration_ms: number;
  started_at: string;
}

interface AgentOption {
  id: string;
  name: string;
}

export default function TimersPage() {
  const { dict } = useI18n();

  const [timers, setTimers] = useState<TimerEntry[]>([]);
  const [runs, setRuns] = useState<TimerRunEntry[]>([]);
  const [agents, setAgents] = useState<AgentOption[]>([]);
  const [runningId, setRunningId] = useState<string>("");
  const [errorMsg, setErrorMsg] = useState<string>("");

  const [name, setName] = useState<string>("");
  const [schedule, setSchedule] = useState<string>("15m");
  const [runAt, setRunAt] = useState<string>("");
  const [agentId, setAgentId] = useState<string>("default");
  const [prompt, setPrompt] = useState<string>("");

  const loadTimersAndRuns = () => {
    fetch("/api/timers")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (!data) return;
        if (Array.isArray(data.timers)) setTimers(data.timers);
        if (Array.isArray(data.runs)) setRuns(data.runs);
      })
      .catch(() => {});

    fetch("/api/agents")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.agents)) setAgents(data.agents);
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadTimersAndRuns();
  }, []);

  const handleCreateTimer = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg("");
    if (!name.trim() || !schedule.trim() || !prompt.trim()) return;

    try {
      const res = await fetch("/api/timers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          schedule: schedule.trim(),
          run_at: runAt.trim(),
          agent_id: agentId,
          prompt: prompt.trim(),
          enabled: true,
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        setErrorMsg(data.error || "Failed to create timer");
        return;
      }
      setName("");
      setPrompt("");
      loadTimersAndRuns();
    } catch {
      setErrorMsg("Network error creating timer");
    }
  };

  const handleToggleEnabled = async (t: TimerEntry, nextEnabled: boolean) => {
    await fetch(`/api/timers/${t.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled: nextEnabled }),
    }).catch(() => {});
    loadTimersAndRuns();
  };

  const handleRunNow = async (id: string) => {
    setRunningId(id);
    setErrorMsg("");
    try {
      await fetch(`/api/timers/${id}/run`, { method: "POST" });
      loadTimersAndRuns();
    } finally {
      setRunningId("");
    }
  };

  const handleDeleteTimer = async (id: string) => {
    await fetch(`/api/timers/${id}`, { method: "DELETE" }).catch(() => {});
    loadTimersAndRuns();
  };

  return (
    <div className="max-w-5xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold">{dict.timers.title}</h1>
          <p className="text-sm text-zinc-400">{dict.timers.subtitle}</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge className="border-sky-500/30 bg-sky-500/10 text-sky-300">
            {dict.timers.agentToolBadge}
          </Badge>
          <Button size="sm" variant="outline" onClick={loadTimersAndRuns}>
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Create Timer Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Clock className="h-4 w-4 text-sky-400" />
            {dict.timers.cardTitle}
          </CardTitle>
          <CardDescription>{dict.timers.cardSubtitle}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleCreateTimer} className="space-y-3 text-xs">
            <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={dict.timers.namePlaceholder}
                className="rounded border border-zinc-800 bg-zinc-950 px-3 py-2 text-zinc-100"
              />
              <input
                value={schedule}
                onChange={(e) => setSchedule(e.target.value)}
                placeholder={dict.timers.schedulePlaceholder}
                className="rounded border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-zinc-100"
              />
              <input
                value={runAt}
                onChange={(e) => setRunAt(e.target.value)}
                placeholder={dict.timers.runAtPlaceholder}
                disabled={schedule.trim().toLowerCase() !== "once"}
                className="rounded border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-zinc-100 disabled:opacity-40"
              />
              <select
                value={agentId}
                onChange={(e) => setAgentId(e.target.value)}
                className="rounded border border-zinc-800 bg-zinc-900 px-3 py-2 text-zinc-100"
              >
                {agents.length === 0 ? (
                  <option value="default">default</option>
                ) : (
                  agents.map((ag) => (
                    <option key={ag.id} value={ag.id}>
                      {ag.name} ({ag.id})
                    </option>
                  ))
                )}
              </select>
            </div>

            <div className="flex flex-wrap gap-1.5">
              <button
                type="button"
                onClick={() => setSchedule("15m")}
                className="text-[11px] rounded border border-zinc-800 bg-zinc-900 px-2 py-0.5 text-zinc-300 hover:border-sky-500/40 cursor-pointer"
              >
                interval: 15m
              </button>
              <button
                type="button"
                onClick={() => setSchedule("0 * * * *")}
                className="text-[11px] rounded border border-zinc-800 bg-zinc-900 px-2 py-0.5 text-zinc-300 hover:border-sky-500/40 cursor-pointer"
              >
                cron: 0 * * * *
              </button>
              <button
                type="button"
                onClick={() => {
                  setSchedule("once");
                  if (!runAt) {
                    setRunAt(
                      new Date(Date.now() + 3600_000).toISOString()
                    );
                  }
                }}
                className="text-[11px] rounded border border-zinc-800 bg-zinc-900 px-2 py-0.5 text-zinc-300 hover:border-sky-500/40 cursor-pointer"
              >
                once + run_at
              </button>
            </div>

            <textarea
              rows={2}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder={dict.timers.promptPlaceholder}
              className="w-full rounded border border-zinc-800 bg-zinc-950 p-3 text-zinc-100"
            />

            {errorMsg && (
              <div className="text-xs text-rose-400">{errorMsg}</div>
            )}

            <Button type="submit">
              <Plus className="h-4 w-4" />
              {dict.timers.createBtn}
            </Button>
          </form>
        </CardContent>
      </Card>

      {/* Active Timers List */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {dict.timers.activeTimersTitle} ({timers.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {timers.length === 0 ? (
            <p className="text-sm text-zinc-500">{dict.timers.empty}</p>
          ) : (
            timers.map((t) => (
              <div
                key={t.id}
                className="rounded-lg border border-zinc-800 bg-zinc-950/70 p-4 space-y-2"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-sm text-zinc-100">
                      {t.name}
                    </span>
                    <Badge className="font-mono text-[11px]">
                      {t.schedule === "once"
                        ? `once @ ${t.run_at}`
                        : t.schedule}
                    </Badge>
                    <Badge className="text-[11px]">{t.agent_id}</Badge>
                    <Badge
                      className={
                        t.created_by === "agent"
                          ? "border-sky-500/40 bg-sky-500/10 text-sky-300"
                          : "text-zinc-400"
                      }
                    >
                      {t.created_by === "agent"
                        ? dict.timers.createdByAgent
                        : dict.timers.createdByUser}
                    </Badge>
                    {t.last_status && t.last_status !== "idle" && (
                      <Badge
                        className={
                          t.last_status === "succeeded"
                            ? "border-emerald-500/40 text-emerald-300"
                            : t.last_status === "failed"
                            ? "border-rose-500/40 text-rose-300"
                            : "text-amber-300"
                        }
                      >
                        {t.last_status}
                      </Badge>
                    )}
                  </div>

                  <div className="flex items-center gap-2">
                    <label className="flex items-center gap-1.5 text-xs text-zinc-300 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={t.enabled}
                        onChange={(e) =>
                          handleToggleEnabled(t, e.target.checked)
                        }
                      />
                      {dict.timers.enabledLabel}
                    </label>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={runningId === t.id}
                      onClick={() => handleRunNow(t.id)}
                    >
                      <Play className="h-3.5 w-3.5" />
                      {runningId === t.id
                        ? dict.timers.runningBtn
                        : dict.timers.runNowBtn}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      title={dict.timers.deleteBtn}
                      onClick={() => handleDeleteTimer(t.id)}
                    >
                      <Trash2 className="h-3.5 w-3.5 text-rose-400" />
                    </Button>
                  </div>
                </div>

                <p className="text-xs text-zinc-300 whitespace-pre-wrap">
                  {t.prompt}
                </p>

                <div className="flex flex-wrap gap-4 text-[11px] text-zinc-500 font-mono">
                  {t.next_run_at && (
                    <span>
                      {dict.timers.nextRunLabel}: {t.next_run_at}
                    </span>
                  )}
                  {t.last_run_at && (
                    <span>
                      {dict.timers.lastRunLabel}: {t.last_run_at}
                    </span>
                  )}
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      {/* Execution History Logs */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <History className="h-4 w-4 text-sky-400" />
            {dict.timers.runsHistoryTitle}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {runs.length === 0 ? (
            <p className="text-xs text-zinc-500">{dict.timers.noRunsYet}</p>
          ) : (
            runs.map((r) => (
              <details
                key={r.id}
                className="rounded-lg border border-zinc-800 bg-zinc-950/70 p-3 text-xs"
              >
                <summary className="cursor-pointer flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-zinc-200">
                      {r.timer_name}
                    </span>
                    <Badge className="text-[10px]">{r.trigger}</Badge>
                    <Badge
                      className={
                        r.status === "succeeded"
                          ? "border-emerald-500/40 text-emerald-300"
                          : r.status === "failed"
                          ? "border-rose-500/40 text-rose-300"
                          : "text-amber-300"
                      }
                    >
                      {r.status}
                    </Badge>
                  </div>
                  <div className="text-[11px] text-zinc-500 font-mono">
                    {r.duration_ms}ms · {r.started_at}
                  </div>
                </summary>
                <div className="mt-2 pt-2 border-t border-zinc-800/80 space-y-1">
                  {r.error && (
                    <pre className="text-rose-400 whitespace-pre-wrap font-mono text-[11px]">
                      {r.error}
                    </pre>
                  )}
                  {r.output && (
                    <pre className="text-zinc-300 whitespace-pre-wrap font-mono text-[11px]">
                      {r.output}
                    </pre>
                  )}
                </div>
              </details>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
