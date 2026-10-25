"use client";

import React, { useState } from "react";
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
import { Paperclip, Send, Sparkles } from "lucide-react";

export default function ChatPage() {
  const { dict, locale } = useI18n();
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<
    Array<{ role: "user" | "assistant"; text: string }>
  >([]);

  const handleSend = (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;
    const prompt = input.trim();
    setInput("");
    setMessages((prev) => [
      ...prev,
      { role: "user", text: prompt },
      {
        role: "assistant",
        text:
          locale === "ja"
            ? "[SSE Stream Skeleton] /api/chat (Norma v0.4.5 Session) へ接続準備完了。LLM プロバイダーを設定するとリアルタイム応答・ツール呼び出しがストリーミングされます。"
            : "[SSE Stream Skeleton] Ready to stream from /api/chat (Norma v0.4.5 Session). Configure an LLM provider in Settings to stream live responses and tool calls.",
      },
    ]);
  };

  return (
    <div className="max-w-4xl space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">{dict.chat.title}</h1>
          <p className="text-sm text-zinc-400">{dict.chat.subtitle}</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-zinc-400">{dict.chat.agentLabel}:</span>
          <Badge className="border-sky-500/40 bg-sky-500/10 text-sky-300">
            <Sparkles className="h-3 w-3 mr-1" />
            {dict.chat.defaultAgent}
          </Badge>
        </div>
      </div>

      <Card className="min-h-[420px] flex flex-col justify-between">
        <CardHeader className="border-b border-zinc-800/80 py-3">
          <CardTitle className="text-sm font-medium text-zinc-300">
            SSE Event Stream · Thinking · Tool Calls
          </CardTitle>
          <CardDescription className="text-xs">
            POST /api/chat (locale={locale})
          </CardDescription>
        </CardHeader>
        <CardContent className="flex-1 py-4 space-y-3">
          {messages.length === 0 ? (
            <div className="text-sm text-zinc-500 py-12 text-center">
              {dict.chat.placeholder}
            </div>
          ) : (
            messages.map((m, idx) => (
              <div
                key={idx}
                className={
                  m.role === "user"
                    ? "ml-auto max-w-xl rounded-lg bg-sky-500/20 border border-sky-500/30 px-4 py-2.5 text-sm"
                    : "mr-auto max-w-2xl rounded-lg bg-zinc-800/70 border border-zinc-700/70 px-4 py-2.5 text-sm text-zinc-200"
                }
              >
                {m.text}
              </div>
            ))
          )}
        </CardContent>
        <form
          onSubmit={handleSend}
          className="p-3 border-t border-zinc-800/80 flex items-center gap-2"
        >
          <Button type="button" variant="outline" size="sm">
            <Paperclip className="h-4 w-4" />
          </Button>
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={dict.chat.placeholder}
            className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-sky-500"
          />
          <Button type="submit">
            <Send className="h-4 w-4" />
            {dict.chat.send}
          </Button>
        </form>
      </Card>
    </div>
  );
}
