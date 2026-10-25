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

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim()) return;
    const prompt = input.trim();
    setInput("");
    setMessages((prev) => [...prev, { role: "user", text: prompt }]);

    try {
      const res = await fetch("/api/chat", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Cortex-Locale": locale,
        },
        body: JSON.stringify({ prompt, locale }),
      });
      if (!res.ok || !res.body) {
        setMessages((prev) => [
          ...prev,
          { role: "assistant", text: dict.chat.skeletonReply },
        ]);
        return;
      }
      const raw = await res.text();
      if (raw.includes("event: error") || !raw.trim()) {
        setMessages((prev) => [
          ...prev,
          { role: "assistant", text: dict.chat.skeletonReply },
        ]);
        return;
      }
      let assembled = "";
      for (const line of raw.split("\n")) {
        if (!line.startsWith("data: ")) continue;
        try {
          const ev = JSON.parse(line.slice(6));
          if (ev.kind === "text" && typeof ev.text === "string") {
            assembled += ev.text;
          } else if (
            ev.kind === "result" &&
            !assembled &&
            typeof ev.final_text === "string"
          ) {
            assembled = ev.final_text;
          }
        } catch {
          // ignore malformed SSE line
        }
      }
      setMessages((prev) => [
        ...prev,
        {
          role: "assistant",
          text: assembled.trim() || dict.chat.skeletonReply,
        },
      ]);
    } catch {
      setMessages((prev) => [
        ...prev,
        { role: "assistant", text: dict.chat.skeletonReply },
      ]);
    }
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
            {dict.chat.streamCardTitle}
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
