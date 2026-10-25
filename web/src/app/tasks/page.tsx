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
import { Network, Play } from "lucide-react";

export default function TasksPage() {
  const { dict } = useI18n();

  return (
    <div className="max-w-5xl space-y-6">
      <div>
        <h1 className="text-xl font-bold">{dict.tasks.title}</h1>
        <p className="text-sm text-zinc-400">{dict.tasks.subtitle}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Network className="h-4 w-4 text-sky-400" />
            Planner ⇄ Worker Autonomous Pipeline
          </CardTitle>
          <CardDescription>
            Norma coordinator &amp; subagent fan-out with artifact synthesis
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-2">
            <input
              placeholder={dict.tasks.newGoalPlaceholder}
              className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-sky-500"
            />
            <Button>
              <Play className="h-4 w-4" />
              {dict.tasks.launchGoal}
            </Button>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 pt-2">
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">1. Goal Decomposition</Badge>
              <p className="text-xs text-zinc-400">
                Breaks high-level objectives into an executable DAG of subtasks.
              </p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">2. Parallel Workers</Badge>
              <p className="text-xs text-zinc-400">
                Dispatches isolated Norma subagents concurrently with tool
                access.
              </p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <Badge className="mb-2">3. Artifacts &amp; Reports</Badge>
              <p className="text-xs text-zinc-400">
                Aggregates worker outputs into structured JSON/Markdown
                artifacts.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
