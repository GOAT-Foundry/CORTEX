"use client";

import React, { useEffect, useRef, useState } from "react";
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
  Brain,
  Edit3,
  FileText,
  MessageSquarePlus,
  Paperclip,
  Send,
  Sparkles,
  Square,
  Trash2,
  Wrench,
  X,
} from "lucide-react";

interface AgentProfile {
  id: string;
  name: string;
  description: string;
}

interface ChatSession {
  id: string;
  title: string;
  agent_id: string;
  provider_id: string;
  model: string;
  updated_at: string;
}

interface ChatToolCall {
  id: string;
  name: string;
  input?: string;
  output?: string;
  is_error?: boolean;
}

interface ChatAttachment {
  name: string;
  media_type: string;
  content: string;
  size_bytes?: number;
}

interface ChatMessage {
  id?: number;
  role: "user" | "assistant";
  content: string;
  thinking?: string;
  tool_calls?: ChatToolCall[];
  attachments?: ChatAttachment[];
  input_tokens?: number;
  output_tokens?: number;
}

export default function ChatPage() {
  const { dict, locale } = useI18n();

  const [agents, setAgents] = useState<AgentProfile[]>([]);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string>("");
  const [selectedAgentId, setSelectedAgentId] = useState<string>("default");
  const [modelOverride, setModelOverride] = useState<string>("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState<string>("");
  const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
  const [streaming, setStreaming] = useState<boolean>(false);

  const abortRef = useRef<AbortController | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const sessionLoadSeqRef = useRef<number>(0);

  const loadSessionsAndAgents = () => {
    fetch("/api/agents")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.agents)) {
          setAgents(data.agents);
        }
      })
      .catch(() => {});

    fetch("/api/chat/sessions")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data && Array.isArray(data.sessions)) {
          setSessions(data.sessions);
        }
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadSessionsAndAgents();
  }, []);

  const handleSelectSession = async (id: string) => {
    if (streaming) handleStop();
    const seq = ++sessionLoadSeqRef.current;
    setActiveSessionId(id);
    try {
      const res = await fetch(`/api/chat/sessions/${id}`);
      if (!res.ok || seq !== sessionLoadSeqRef.current) return;
      const data = await res.json();
      if (seq !== sessionLoadSeqRef.current) return;
      if (data.session) {
        setSelectedAgentId(data.session.agent_id || "default");
        setModelOverride(data.session.model || "");
      }
      if (Array.isArray(data.messages)) {
        setMessages(data.messages);
      }
    } catch {
      // ignore
    }
  };

  const handleNewChat = async () => {
    if (streaming) handleStop();
    const seq = ++sessionLoadSeqRef.current;
    try {
      const res = await fetch("/api/chat/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          title: dict.chat.newChat,
          agent_id: selectedAgentId,
          model: modelOverride,
        }),
      });
      if (res.ok && seq === sessionLoadSeqRef.current) {
        const created: ChatSession = await res.json();
        if (seq !== sessionLoadSeqRef.current) return;
        setActiveSessionId(created.id);
        setMessages([]);
        loadSessionsAndAgents();
      }
    } catch {
      if (seq === sessionLoadSeqRef.current) {
        setActiveSessionId("");
        setMessages([]);
      }
    }
  };

  const handleRenameSession = async (cs: ChatSession) => {
    const nextTitle = window.prompt(dict.chat.renamePrompt, cs.title);
    if (!nextTitle || !nextTitle.trim()) return;
    await fetch(`/api/chat/sessions/${cs.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: nextTitle.trim() }),
    }).catch(() => {});
    loadSessionsAndAgents();
  };

  const handleDeleteSession = async (id: string) => {
    if (activeSessionId === id && streaming) handleStop();
    if (activeSessionId === id) {
      sessionLoadSeqRef.current++;
    }
    await fetch(`/api/chat/sessions/${id}`, { method: "DELETE" }).catch(
      () => {}
    );
    if (activeSessionId === id) {
      setActiveSessionId("");
      setMessages([]);
    }
    loadSessionsAndAgents();
  };

  const handleAgentChange = async (nextAgent: string) => {
    setSelectedAgentId(nextAgent);
    if (activeSessionId) {
      await fetch(`/api/chat/sessions/${activeSessionId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent_id: nextAgent }),
      }).catch(() => {});
      loadSessionsAndAgents();
    }
  };

  const handleModelCommit = async (nextModel: string) => {
    if (activeSessionId) {
      await fetch(`/api/chat/sessions/${activeSessionId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model: nextModel }),
      }).catch(() => {});
      loadSessionsAndAgents();
    }
  };

  const handleFilesSelected = async (
    e: React.ChangeEvent<HTMLInputElement>
  ) => {
    const files = e.target.files;
    if (!files || files.length === 0) return;

    const readFile = (file: File): Promise<ChatAttachment> =>
      new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = () => {
          resolve({
            name: file.name,
            media_type: file.type || "text/plain",
            content: typeof reader.result === "string" ? reader.result : "",
            size_bytes: file.size,
          });
        };
        reader.readAsText(file);
      });

    const loaded = await Promise.all(Array.from(files).map(readFile));
    setAttachments((prev) => [...prev, ...loaded]);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    setStreaming(false);
  };

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if ((!input.trim() && attachments.length === 0) || streaming) return;

    const prompt = input.trim();
    const sentAttachments = [...attachments];
    setInput("");
    setAttachments([]);

    const userMsg: ChatMessage = {
      role: "user",
      content: prompt,
      attachments: sentAttachments,
    };
    const placeholderAssistant: ChatMessage = {
      role: "assistant",
      content: "",
      thinking: "",
      tool_calls: [],
    };
    setMessages((prev) => [...prev, userMsg, placeholderAssistant]);
    setStreaming(true);

    const ctrl = new AbortController();
    abortRef.current = ctrl;

    const endpoint = activeSessionId
      ? `/api/chat/sessions/${activeSessionId}/messages`
      : "/api/chat";

    try {
      const res = await fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Cortex-Locale": locale,
        },
        body: JSON.stringify({
          session_id: activeSessionId,
          agent_id: selectedAgentId,
          model: modelOverride,
          prompt,
          locale,
          attachments: sentAttachments,
        }),
        signal: ctrl.signal,
      });

      if (!res.ok || !res.body) {
        setMessages((prev) => {
          const copy = [...prev];
          copy[copy.length - 1] = {
            role: "assistant",
            content: dict.chat.skeletonReply,
          };
          return copy;
        });
        setStreaming(false);
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let textAcc = "";
      let thinkingAcc = "";
      let toolsAcc: ChatToolCall[] = [];
      let inTok = 0;
      let outTok = 0;
      let hadError = false;

      const updateAssistant = (fallbackIfEmpty: boolean) => {
        setMessages((prev) => {
          const copy = [...prev];
          copy[copy.length - 1] = {
            role: "assistant",
            content:
              textAcc ||
              (fallbackIfEmpty && toolsAcc.length === 0
                ? dict.chat.skeletonReply
                : ""),
            thinking: thinkingAcc,
            tool_calls: [...toolsAcc],
            input_tokens: inTok,
            output_tokens: outTok,
          };
          return copy;
        });
      };

      const processLine = (line: string) => {
        if (!line.startsWith("data: ")) return;
        try {
          const ev = JSON.parse(line.slice(6));
          if (ev.session_id && !activeSessionId) {
            setActiveSessionId(ev.session_id);
          }
          if (ev.error) {
            hadError = true;
            return;
          }
          switch (ev.kind) {
            case "text":
              if (typeof ev.text === "string") textAcc += ev.text;
              updateAssistant(false);
              break;
            case "thinking":
              if (typeof ev.text === "string") thinkingAcc += ev.text;
              updateAssistant(false);
              break;
            case "tool_use":
              toolsAcc = [
                ...toolsAcc,
                {
                  id: ev.tool_id || `tool-${toolsAcc.length}`,
                  name: ev.tool_name || "tool",
                  input: ev.input || "",
                },
              ];
              updateAssistant(false);
              break;
            case "tool_result":
              toolsAcc = toolsAcc.map((tc) =>
                tc.id === ev.tool_id
                  ? { ...tc, output: ev.output || "", is_error: !!ev.is_error }
                  : tc
              );
              updateAssistant(false);
              break;
            case "usage":
              inTok = ev.input_tokens || inTok;
              outTok = ev.output_tokens || outTok;
              updateAssistant(false);
              break;
            case "result":
              if (!textAcc && typeof ev.final_text === "string") {
                textAcc = ev.final_text;
              }
              updateAssistant(true);
              break;
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
      if (hadError && !textAcc) {
        updateAssistant(true);
      }
      loadSessionsAndAgents();
    } catch {
      setMessages((prev) => {
        const copy = [...prev];
        if (!copy[copy.length - 1]?.content) {
          copy[copy.length - 1] = {
            role: "assistant",
            content: dict.chat.skeletonReply,
          };
        }
        return copy;
      });
    } finally {
      setStreaming(false);
      abortRef.current = null;
    }
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-4 gap-4 max-w-6xl">
      {/* Left Sidebar: Multi-Turn Sessions */}
      <Card className="lg:col-span-1 flex flex-col h-[640px]">
        <CardHeader className="py-3 px-4 border-b border-zinc-800/80 flex flex-row items-center justify-between">
          <CardTitle className="text-sm font-semibold">
            {dict.chat.sessionsTitle}
          </CardTitle>
          <Button size="sm" onClick={handleNewChat}>
            <MessageSquarePlus className="h-3.5 w-3.5" />
            {dict.chat.newChat}
          </Button>
        </CardHeader>
        <CardContent className="flex-1 p-2 overflow-y-auto space-y-1">
          {sessions.length === 0 ? (
            <div className="text-xs text-zinc-500 text-center py-8">
              {dict.chat.noSessions}
            </div>
          ) : (
            sessions.map((cs) => {
              const active = cs.id === activeSessionId;
              return (
                <div
                  key={cs.id}
                  onClick={() => handleSelectSession(cs.id)}
                  className={`group flex items-center justify-between rounded-lg px-2.5 py-2 text-xs cursor-pointer transition-colors ${
                    active
                      ? "bg-sky-500/15 text-sky-300 border border-sky-500/30"
                      : "text-zinc-300 hover:bg-zinc-800/60"
                  }`}
                >
                  <div className="truncate pr-2">
                    <div className="font-medium truncate">{cs.title}</div>
                    <div className="text-[10px] text-zinc-500">
                      {cs.agent_id}
                    </div>
                  </div>
                  <div
                    className="flex items-center gap-1 opacity-70 group-hover:opacity-100"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <button
                      type="button"
                      title={dict.chat.renameSession}
                      onClick={() => handleRenameSession(cs)}
                      className="p-1 hover:text-sky-400 cursor-pointer"
                    >
                      <Edit3 className="h-3 w-3" />
                    </button>
                    <button
                      type="button"
                      title={dict.chat.deleteSession}
                      onClick={() => handleDeleteSession(cs.id)}
                      className="p-1 hover:text-rose-400 cursor-pointer"
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </CardContent>
      </Card>

      {/* Main Chat Panel */}
      <div className="lg:col-span-3 flex flex-col space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h1 className="text-xl font-bold">{dict.chat.title}</h1>
            <p className="text-xs text-zinc-400">{dict.chat.subtitle}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex items-center gap-1.5 text-xs">
              <Sparkles className="h-3.5 w-3.5 text-sky-400" />
              <span className="text-zinc-400">{dict.chat.agentLabel}:</span>
              <select
                value={selectedAgentId}
                onChange={(e) => handleAgentChange(e.target.value)}
                className="rounded-md border border-zinc-800 bg-zinc-900 px-2.5 py-1 text-xs text-zinc-100"
              >
                {agents.length === 0 ? (
                  <option value="default">{dict.chat.defaultAgent}</option>
                ) : (
                  agents.map((ag) => (
                    <option key={ag.id} value={ag.id}>
                      {ag.name}
                    </option>
                  ))
                )}
              </select>
            </div>
            <input
              value={modelOverride}
              onChange={(e) => setModelOverride(e.target.value)}
              onBlur={(e) => handleModelCommit(e.target.value)}
              placeholder={dict.chat.modelPlaceholder}
              className="rounded-md border border-zinc-800 bg-zinc-900 px-2.5 py-1 text-xs text-zinc-200 w-44"
            />
          </div>
        </div>

        <Card className="flex-1 flex flex-col justify-between h-[580px]">
          <CardHeader className="border-b border-zinc-800/80 py-2.5 px-4">
            <CardTitle className="text-xs font-medium text-zinc-300">
              {dict.chat.streamCardTitle}
            </CardTitle>
            <CardDescription className="text-[11px]">
              POST /api/chat (locale={locale}, agent={selectedAgentId})
            </CardDescription>
          </CardHeader>

          <CardContent className="flex-1 py-4 px-4 space-y-4 overflow-y-auto">
            {messages.length === 0 ? (
              <div className="text-sm text-zinc-500 py-16 text-center">
                {dict.chat.placeholder}
              </div>
            ) : (
              messages.map((m, idx) => (
                <div
                  key={idx}
                  className={
                    m.role === "user"
                      ? "ml-auto max-w-xl rounded-lg bg-sky-500/20 border border-sky-500/30 px-4 py-2.5 text-sm space-y-2"
                      : "mr-auto max-w-2xl rounded-lg bg-zinc-800/70 border border-zinc-700/70 px-4 py-2.5 text-sm text-zinc-200 space-y-2"
                  }
                >
                  {/* Attachments on User Message */}
                  {m.attachments && m.attachments.length > 0 && (
                    <div className="flex flex-wrap gap-1.5">
                      {m.attachments.map((att, aIdx) => (
                        <Badge
                          key={aIdx}
                          className="border-sky-500/40 bg-sky-500/10 text-sky-200 gap-1"
                        >
                          <FileText className="h-3 w-3" />
                          {att.name}
                        </Badge>
                      ))}
                    </div>
                  )}

                  {/* Collapsible Thinking / Reasoning Block */}
                  {m.thinking && (
                    <details className="rounded-md border border-zinc-700/80 bg-zinc-900/80 p-2 text-xs text-zinc-300">
                      <summary className="cursor-pointer font-medium inline-flex items-center gap-1.5 text-amber-300">
                        <Brain className="h-3.5 w-3.5" />
                        {dict.chat.thinkingLabel}
                      </summary>
                      <pre className="mt-2 whitespace-pre-wrap font-mono text-[11px] text-zinc-400">
                        {m.thinking}
                      </pre>
                    </details>
                  )}

                  {/* Tool Calls Inspector */}
                  {m.tool_calls && m.tool_calls.length > 0 && (
                    <div className="space-y-1.5">
                      {m.tool_calls.map((tc) => (
                        <details
                          key={tc.id}
                          className="rounded-md border border-zinc-700/80 bg-zinc-950/80 p-2 text-xs"
                        >
                          <summary className="cursor-pointer flex items-center justify-between gap-2">
                            <span className="inline-flex items-center gap-1.5 font-mono text-sky-300">
                              <Wrench className="h-3 w-3" />
                              {tc.name}
                            </span>
                            <Badge
                              className={
                                tc.is_error
                                  ? "border-rose-500/40 text-rose-300"
                                  : tc.output !== undefined
                                  ? "border-emerald-500/40 text-emerald-300"
                                  : "text-amber-300"
                              }
                            >
                              {tc.is_error
                                ? dict.chat.toolError
                                : tc.output !== undefined
                                ? dict.chat.toolDone
                                : dict.chat.toolRunning}
                            </Badge>
                          </summary>
                          <div className="mt-2 space-y-1.5 font-mono text-[11px]">
                            {tc.input && (
                              <div>
                                <div className="text-zinc-500">
                                  {dict.chat.toolInput}:
                                </div>
                                <pre className="overflow-x-auto text-zinc-300">
                                  {tc.input}
                                </pre>
                              </div>
                            )}
                            {tc.output !== undefined && (
                              <div>
                                <div className="text-zinc-500">
                                  {dict.chat.toolOutput}:
                                </div>
                                <pre className="overflow-x-auto text-zinc-300">
                                  {tc.output}
                                </pre>
                              </div>
                            )}
                          </div>
                        </details>
                      ))}
                    </div>
                  )}

                  {/* Message Text */}
                  <div className="whitespace-pre-wrap leading-relaxed">
                    {m.content}
                  </div>

                  {/* Per-turn Token Usage */}
                  {(m.input_tokens || m.output_tokens) ? (
                    <div className="text-[10px] text-zinc-500 pt-1">
                      {dict.chat.tokensLabel}: in={m.input_tokens ?? 0} / out=
                      {m.output_tokens ?? 0}
                    </div>
                  ) : null}
                </div>
              ))
            )}
          </CardContent>

          <div className="border-t border-zinc-800/80 p-3 space-y-2">
            {attachments.length > 0 && (
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-[11px] text-zinc-400">
                  {dict.chat.attachedFiles}:
                </span>
                {attachments.map((att, idx) => (
                  <Badge
                    key={idx}
                    className="gap-1 border-sky-500/40 bg-sky-500/10 text-sky-200"
                  >
                    <FileText className="h-3 w-3" />
                    <span>{att.name}</span>
                    <button
                      type="button"
                      onClick={() =>
                        setAttachments((prev) =>
                          prev.filter((_, i) => i !== idx)
                        )
                      }
                      className="ml-1 hover:text-rose-400 cursor-pointer"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </Badge>
                ))}
              </div>
            )}

            <form onSubmit={handleSend} className="flex items-center gap-2">
              <input
                ref={fileInputRef}
                type="file"
                multiple
                accept=".txt,.md,.json,.csv,.yaml,.yml,.go,.ts,.tsx,.js,.py,.sh,.html,.css,.sql,.xml,.log"
                onChange={handleFilesSelected}
                className="hidden"
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                title={dict.chat.attachFile}
                onClick={() => fileInputRef.current?.click()}
              >
                <Paperclip className="h-4 w-4" />
              </Button>
              <input
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder={dict.chat.placeholder}
                className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 focus:outline-none focus:border-sky-500"
              />
              {streaming ? (
                <Button type="button" variant="outline" onClick={handleStop}>
                  <Square className="h-4 w-4 text-rose-400" />
                  {dict.chat.stop}
                </Button>
              ) : (
                <Button type="submit">
                  <Send className="h-4 w-4" />
                  {dict.chat.send}
                </Button>
              )}
            </form>
          </div>
        </Card>
      </div>
    </div>
  );
}
