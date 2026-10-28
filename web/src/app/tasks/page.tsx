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
  FileText,
  GitBranch,
  Network,
  Play,
  RefreshCw,
  Trash2,
} from "lucide-react";

interface TaskNode {
  id: string;
  title: string;
  prompt: string;
  agent_id: string;
  depends_on: string[];
  status: string;
  output?: string;
  error?: string;
  duration_ms?: number;
}

interface TaskArtifact {
  id: string;
  task_id: string;
  node_id?: string;
  title: string;
  kind: string;
  content: string;
  created_at: string;
}

interface TaskRun {
  id: string;
  goal: string;
  locale: string;
  status: string;
  nodes: TaskNode[];
  final_report?: string;
  error?: string;
  created_at: string;
}

export default function TasksPage() {
  const { dict, locale } = useI18n();

  const [goalInput, setGoalInput] = useState<string>("");
  const [tasks, setTasks] = useState<TaskRun[]>([]);
  const [activeTask, setActiveTask] = useState<TaskRun | null>(null);
  const [artifacts, setArtifacts] = useState<TaskArtifact[]>([]);
  const [running, setRunning] = useState<boolean>(false);

  const loadTasks = () => {
    fetch("/api/tasks")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.tasks)) {
          setTasks(data.tasks);
        }
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadTasks();
  }, []);

  const handleSelectTask = async (id: string) => {
    try {
      const res = await fetch(`/api/tasks/${id}`);
      if (!res.ok) return;
      const data = await res.json();
      if (data.task) setActiveTask(data.task);
      if (Array.isArray(data.artifacts)) setArtifacts(data.artifacts);
    } catch {
      // ignore
    }
  };

  const handleDeleteTask = async (id: string) => {
    await fetch(`/api/tasks/${id}`, { method: "DELETE" }).catch(() => {});
    if (activeTask?.id === id) {
      setActiveTask(null);
      setArtifacts([]);
    }
    loadTasks();
  };

  const handleLaunchGoal = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!goalInput.trim() || running) return;
    const goal = goalInput.trim();
    setGoalInput("");
    setRunning(true);
    setArtifacts([]);

    try {
      const res = await fetch("/api/tasks", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
          "X-Cortex-Locale": locale,
        },
        body: JSON.stringify({
          goal,
          locale,
          stream: true,
        }),
      });

      if (!res.ok || !res.body) {
        setRunning(false);
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      const processLine = (line: string) => {
        if (!line.startsWith("data: ")) return;
        try {
          const payload = JSON.parse(line.slice(6));
          if (payload.task) {
            setActiveTask(payload.task);
          }
          if (payload.node) {
            const updatedNode: TaskNode = payload.node;
            setActiveTask((prev) => {
              if (!prev) return prev;
              return {
                ...prev,
                nodes: prev.nodes.map((n) =>
                  n.id === updatedNode.id ? updatedNode : n
                ),
              };
            });
          }
          if (payload.artifact) {
            const art: TaskArtifact = payload.artifact;
            setArtifacts((prev) => {
              if (prev.some((a) => a.id === art.id)) return prev;
              return [...prev, art];
            });
          }
          if (Array.isArray(payload.artifacts)) {
            setArtifacts(payload.artifacts);
          }
        } catch {
          // ignore malformed SSE line
        }
      };

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";
        for (const l of lines) {
          processLine(l.trim());
        }
      }
      if (buffer.trim()) {
        processLine(buffer.trim());
      }
      loadTasks();
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="max-w-6xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">{dict.tasks.title}</h1>
          <p className="text-sm text-zinc-400">{dict.tasks.subtitle}</p>
        </div>
        <Button size="sm" variant="outline" onClick={loadTasks}>
          <RefreshCw className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* Goal Input & Pipeline Overview */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Network className="h-4 w-4 text-sky-400" />
            {dict.tasks.pipelineTitle}
          </CardTitle>
          <CardDescription>{dict.tasks.pipelineSubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <form onSubmit={handleLaunchGoal} className="flex gap-2">
            <input
              value={goalInput}
              onChange={(e) => setGoalInput(e.target.value)}
              placeholder={dict.tasks.newGoalPlaceholder}
              className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-sky-500"
            />
            <Button type="submit" disabled={running}>
              <Play className="h-4 w-4" />
              {running ? dict.tasks.runningGoal : dict.tasks.launchGoal}
            </Button>
          </form>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 pt-1">
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">{dict.tasks.step1Badge}</Badge>
              <p className="text-xs text-zinc-400">{dict.tasks.step1Desc}</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">{dict.tasks.step2Badge}</Badge>
              <p className="text-xs text-zinc-400">{dict.tasks.step2Desc}</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">{dict.tasks.step3Badge}</Badge>
              <p className="text-xs text-zinc-400">{dict.tasks.step3Desc}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
        {/* Left Sidebar: Past Task Runs */}
        <Card className="lg:col-span-1">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">
              {dict.tasks.historyTitle} ({tasks.length})
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1.5 max-h-[520px] overflow-y-auto">
            {tasks.length === 0 ? (
              <p className="text-xs text-zinc-500 py-6 text-center">
                {dict.tasks.noTasksYet}
              </p>
            ) : (
              tasks.map((tr) => {
                const selected = activeTask?.id === tr.id;
                return (
                  <div
                    key={tr.id}
                    onClick={() => handleSelectTask(tr.id)}
                    className={`rounded-lg p-2.5 text-xs cursor-pointer transition-colors flex items-start justify-between gap-2 ${
                      selected
                        ? "bg-sky-500/15 border border-sky-500/30 text-sky-200"
                        : "border border-zinc-800/80 bg-zinc-950/60 text-zinc-300 hover:bg-zinc-900"
                    }`}
                  >
                    <div className="min-w-0 space-y-1">
                      <div className="font-medium truncate">{tr.goal}</div>
                      <div className="flex items-center gap-1.5">
                        <Badge
                          className={
                            tr.status === "completed"
                              ? "border-emerald-500/40 text-emerald-300 text-[10px]"
                              : tr.status === "failed"
                              ? "border-rose-500/40 text-rose-300 text-[10px]"
                              : "text-amber-300 text-[10px]"
                          }
                        >
                          {tr.status}
                        </Badge>
                        <span className="text-[10px] text-zinc-500">
                          {tr.nodes?.length || 0} nodes
                        </span>
                      </div>
                    </div>
                    <button
                      type="button"
                      title={dict.tasks.deleteTask}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteTask(tr.id);
                      }}
                      className="text-zinc-500 hover:text-rose-400 cursor-pointer"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                );
              })
            )}
          </CardContent>
        </Card>

        {/* Right Area: DAG Visualizer & Artifacts Viewer */}
        <div className="lg:col-span-3 space-y-4">
          {!activeTask ? (
            <Card>
              <CardContent className="py-16 text-center text-sm text-zinc-500">
                {dict.tasks.noArtifacts}
              </CardContent>
            </Card>
          ) : (
            <>
              {/* DAG Nodes Card */}
              <Card>
                <CardHeader className="pb-3">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-sm flex items-center gap-2">
                      <GitBranch className="h-4 w-4 text-sky-400" />
                      {dict.tasks.dagTitle}: {activeTask.goal}
                    </CardTitle>
                    <Badge
                      className={
                        activeTask.status === "completed"
                          ? "border-emerald-500/40 text-emerald-300"
                          : activeTask.status === "failed"
                          ? "border-rose-500/40 text-rose-300"
                          : "text-amber-300"
                      }
                    >
                      {activeTask.status}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  {activeTask.nodes.map((node) => (
                    <div
                      key={node.id}
                      className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 flex flex-col justify-between space-y-2"
                    >
                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between gap-1">
                          <Badge className="font-mono text-[10px]">
                            {node.id}
                          </Badge>
                          <Badge
                            className={
                              node.status === "completed"
                                ? "border-emerald-500/40 text-emerald-300 text-[10px]"
                                : node.status === "failed"
                                ? "border-rose-500/40 text-rose-300 text-[10px]"
                                : node.status === "running"
                                ? "border-amber-500/40 text-amber-300 text-[10px]"
                                : "text-zinc-400 text-[10px]"
                            }
                          >
                            {node.status}
                          </Badge>
                        </div>
                        <div className="font-semibold text-xs text-zinc-100">
                          {node.title}
                        </div>
                        <div className="text-[11px] text-zinc-500">
                          agent:{" "}
                          <span className="font-mono text-sky-300">
                            {node.agent_id}
                          </span>
                        </div>
                        <div className="text-[10px] text-zinc-500 font-mono">
                          {node.depends_on && node.depends_on.length > 0
                            ? `${dict.tasks.dependsOnLabel}: ${node.depends_on.join(", ")}`
                            : dict.tasks.rootNodeLabel}
                        </div>
                      </div>

                      {node.output && (
                        <details className="text-[11px] pt-1 border-t border-zinc-800/80">
                          <summary className="cursor-pointer text-sky-400">
                            Output ({node.duration_ms ?? 0}ms)
                          </summary>
                          <pre className="mt-1.5 whitespace-pre-wrap text-zinc-300 font-mono max-h-36 overflow-y-auto">
                            {node.output}
                          </pre>
                        </details>
                      )}
                    </div>
                  ))}
                </CardContent>
              </Card>

              {/* Artifacts & Final Report Card */}
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm flex items-center gap-2">
                    <FileText className="h-4 w-4 text-sky-400" />
                    {dict.tasks.artifactsTitle} ({artifacts.length})
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {activeTask.final_report && (
                    <div className="rounded-lg border border-sky-500/30 bg-zinc-950 p-4 space-y-2">
                      <Badge className="border-sky-500/40 bg-sky-500/10 text-sky-300">
                        Final Report
                      </Badge>
                      <pre className="text-xs text-zinc-200 whitespace-pre-wrap font-mono max-h-72 overflow-y-auto">
                        {activeTask.final_report}
                      </pre>
                    </div>
                  )}

                  {artifacts
                    .filter((a) => a.kind !== "report")
                    .map((art) => (
                      <details
                        key={art.id}
                        className="rounded-lg border border-zinc-800 bg-zinc-950/70 p-3 text-xs"
                      >
                        <summary className="cursor-pointer flex items-center justify-between">
                          <span className="font-medium text-zinc-200">
                            {art.title}
                          </span>
                          <Badge className="font-mono text-[10px]">
                            {art.node_id || art.kind}
                          </Badge>
                        </summary>
                        <pre className="mt-2 pt-2 border-t border-zinc-800 whitespace-pre-wrap text-zinc-300 font-mono text-[11px]">
                          {art.content}
                        </pre>
                      </details>
                    ))}
                </CardContent>
              </Card>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
