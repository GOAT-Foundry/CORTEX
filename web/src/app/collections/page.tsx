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
import { Database } from "lucide-react";

export default function CollectionsPage() {
  const { dict } = useI18n();

  return (
    <div className="max-w-5xl space-y-6">
      <div>
        <h1 className="text-xl font-bold">{dict.collections.title}</h1>
        <p className="text-sm text-zinc-400">{dict.collections.subtitle}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Database className="h-4 w-4 text-sky-400" />
            /api/collections/&#123;name&#125;
          </CardTitle>
          <CardDescription>{dict.collections.cardSubtitle}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            <Badge>collection_list</Badge>
            <Badge>collection_get</Badge>
            <Badge>collection_put</Badge>
            <Badge>collection_delete</Badge>
          </div>
          <pre className="rounded-lg border border-zinc-800 bg-zinc-950 p-4 text-xs text-zinc-300 overflow-x-auto">
            {`${dict.collections.codeComment}
PUT /api/collections/${dict.collections.sampleCollection}/item-1
{
  "title": "Domain Record",
  "status": "active"
}`}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}
