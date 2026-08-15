"use client";

import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { Brain, ChevronLeft, Download, LogOut, MessageSquare, PanelLeft, Pin, Plus, Send, Settings, Square, Trash2, Pencil, User, LogIn } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useConfirm, usePrompt } from "@/components/ui/confirm";
import {
  api,
  Citation,
  Conversation,
  KnowledgeBase,
  Message,
  Paginated,
  ReplyStats,
  UserProfile,
} from "@/lib/api";
import { useTranslations } from "next-intl";
import { consumeChatStream } from "@/lib/stream";
import { logout } from "@/lib/auth";
import { useSiteSettings } from "@/lib/use-site-settings";
import { ProfileDialog } from "@/components/features/profile-dialog";
import { MessageBubble } from "@/components/chat/message-bubble";

function getConvIdFromURL(): string {
  if (typeof window === "undefined") return "";
  const params = new URLSearchParams(window.location.search);
  return params.get("c") || "";
}

export function ChatApp({ authed, profile, isAdmin }: { authed: boolean; profile?: UserProfile | null; isAdmin?: boolean }) {
  const [selectedKb, setSelectedKb] = useState<string>("");
  const [selectedConv, setSelectedConv] = useState<string>("");
  const [draft, setDraft] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [greeting, setGreeting] = useState("");
  const [suggestedQuestions, setSuggestedQuestions] = useState<string[]>([]);
  const t = useTranslations();
  const { settings } = useSiteSettings();
  // Registration is allowed only when the setting is explicitly "true",
  // matching the backend check. Missing/other values hide the entry.
  const allowRegistration = settings?.allow_registration === "true";

  const [messages, setMessages] = useState<Message[]>([]);
  const [pendingCitations, setPendingCitations] = useState<Citation[]>([]);
  const [streamedText, setStreamedText] = useState("");
  const [streamedThinking, setStreamedThinking] = useState("");
  const [summarizing, setSummarizing] = useState(false);
  const [showList, setShowList] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const confirm = useConfirm();
  const prompt = usePrompt();

  const scrollRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const { data: kbs } = useSWR(authed ? "kb-list" : null, () => api.listKBs(1, 50));
  const [convQuery, setConvQuery] = useState("");
  const { data: convs, mutate: mutateConvs } = useSWR<Paginated<Conversation>>(
    authed ? (convQuery ? ["conv-search", convQuery] : "conv-list") : null,
    () => api.listConversations(1, 50, convQuery || undefined)
  );
  const filteredConvs = convs?.items?.filter((c) => c.kb_id === selectedKb);

  useEffect(() => {
    if (!selectedKb && kbs?.items?.length) {
      setSelectedKb(kbs.items[0].id);
    }
  }, [kbs, selectedKb]);

  // Auto-focus the input when landing on the home page (logged in, no
  // conversation selected) and after clicking "+ New conversation".
  useEffect(() => {
    if (authed && selectedKb && !selectedConv && !streaming) {
      textareaRef.current?.focus();
    }
  }, [authed, selectedKb, selectedConv, streaming]);

  useEffect(() => {
    if (!authed) return;
    const id = getConvIdFromURL();
    if (id) setSelectedConv(id);
  }, [authed]);

  useEffect(() => {
    if (!selectedConv || !convs?.items?.length || selectedKb) return;
    const conv = convs.items.find((c) => c.id === selectedConv);
    if (conv) setSelectedKb(conv.kb_id);
  }, [convs, selectedConv, selectedKb]);

  const { data: agentData } = useSWR(
    authed && selectedKb && !selectedConv ? `agent-${selectedKb}` : null,
    () => api.getAgent(selectedKb!)
  );
  const { data: msgQuota, mutate: mutateQuota } = useSWR(
    authed ? "msg-quota" : null,
    () => api.getMessageQuota()
  );
  const quotaExceeded = msgQuota !== undefined && msgQuota.quota >= 0 && msgQuota.remaining <= 0;
  useEffect(() => {
    if (!selectedKb || selectedConv) {
      setGreeting("");
      setSuggestedQuestions([]);
      return;
    }
    const def = agentData?.definition;
    // Opening message: prefer definition-level, fall back to legacy start node.
    let msg = def?.opening_message;
    if (!msg) {
      const startNode = def?.nodes?.find((n) => n.type === "start");
      const v = startNode?.data?.opening_message;
      if (typeof v === "string") msg = v;
    }
    setGreeting(typeof msg === "string" ? msg : "");
    setSuggestedQuestions(Array.isArray(def?.suggested_questions) ? def!.suggested_questions : []);
  }, [agentData, selectedKb, selectedConv]);

  const skipFetchRef = useRef(false);

  useEffect(() => {
    if (!selectedConv) {
      setMessages([]);
      return;
    }
    if (skipFetchRef.current) {
      skipFetchRef.current = false;
      return;
    }
    api.listMessages(selectedConv).then((r) => setMessages(r.items)).catch(() => setMessages([]));
  }, [selectedConv]);

  useEffect(() => {
    if (!scrollRef.current) return;
    requestAnimationFrame(() => {
      if (scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    });
  }, [messages, streamedText, streamedThinking, pendingCitations]);

  function selectConv(id: string) {
    setSelectedConv(id);
    const url = id ? "/?c=" + id : "/";
    window.history.replaceState(null, "", url);
  }

  function startNewChat() {
    setSelectedConv("");
    setMessages([]);
    setDraft("");
    window.history.replaceState(null, "", "/");
  }

  function stop() {
    abortRef.current?.abort();
    abortRef.current = null;
  }

  async function del(id: string) {
    const ok = await confirm({
      title: t("chat.delete_confirm"),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteConversation(id);
      if (selectedConv === id) selectConv("");
      mutateConvs();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function rename(id: string, oldTitle: string) {
    const title = await prompt({
      title: t("conv.rename"),
      defaultValue: oldTitle,
      confirmText: t("common.confirm"),
    });
    if (!title || title === oldTitle) return;
    try {
      await api.renameConversation(id, title);
      mutateConvs();
      toast.success(t("conv.renamed"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function togglePin(id: string, pinned: boolean) {
    try {
      await api.pinConversation(id, !pinned);
      mutateConvs();
      toast.success(!pinned ? t("conv.pinned") : t("conv.unpinned"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function exportConv(id: string) {
    try {
      await api.exportConversation(id);
      toast.success(t("conv.exported"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function summarize() {
    if (!selectedConv) return;
    setSummarizing(true);
    try {
      await api.summarizeConversation(selectedConv);
      toast.success(t("chat.memory_saved"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSummarizing(false);
    }
  }

  async function handleLogout() {
    const ok = await confirm({
      title: t("nav.sign_out"),
      description: t("nav.sign_out_confirm"),
      confirmText: t("nav.sign_out"),
    });
    if (!ok) return;
    logout();
  }

  async function send(overrideText?: string) {
    const text = (overrideText ?? draft).trim();
    if (!text || streaming) return;
    if (!selectedKb) {
      toast.error(t("chat.pick_kb_first"));
      return;
    }
    if (quotaExceeded) {
      toast.error(t("chat.quota_exceeded"));
      return;
    }
    if (!overrideText) {
      setDraft("");
      if (textareaRef.current) textareaRef.current.style.height = "auto";
    }

    let convId = selectedConv;
    if (!convId) {
      try {
        const title = text.length > 30 ? text.slice(0, 30) + "…" : text;
        const conv = await api.createConversation(selectedKb, { title });
        convId = conv.id;
        const r = await api.listMessages(convId);
        setMessages(r.items);
        skipFetchRef.current = true;
        selectConv(convId);
        mutateConvs();
      } catch (e) {
        toast.error((e as Error).message);
        return;
      }
    }

    const userMsg: Message = {
      id: crypto.randomUUID(),
      tenant_id: "",
      conversation_id: convId,
      role: "user",
      content: text,
      created_at: new Date().toISOString(),
    };
    setMessages((m) => [...m, userMsg]);
    setStreamedText("");
    setStreamedThinking("");
    setPendingCitations([]);
    setStreaming(true);

    const controller = new AbortController();
    abortRef.current = controller;

    let acc = "";
    let thinkAcc = "";
    let cits: Citation[] = [];
    let doneMsgId = "";
    let streamError = "";
    let doneStats: ReplyStats | undefined;
    let annReply = false;

    try {
      const stream = api.streamChat(convId, { message: text }, controller.signal);
      const { error } = await consumeChatStream(stream, {
        onCitations: (c) => { cits = c; setPendingCitations(c); },
        onToken: (tk, annotation) => {
          acc += tk;
          setStreamedText(acc);
          if (annotation) annReply = true;
        },
        onThinking: (tk) => { thinkAcc += tk; setStreamedThinking(thinkAcc); },
        onDone: (id, stats) => { doneMsgId = id; doneStats = stats; },
        onWarning: (w) => { acc += `⚠️ ${w}\n\n`; setStreamedText(acc); },
      });
      streamError = error ?? "";
    } finally {
      const content = streamError
        ? (acc ? `${acc}\n\n⚠️ ${streamError}` : `⚠️ ${streamError}`)
        : acc;
      if (content) {
        setMessages((m) => [
          ...m,
          {
            id: doneMsgId || crypto.randomUUID(),
            tenant_id: "",
            conversation_id: convId,
            role: "assistant",
            content,
            reasoning: thinkAcc || undefined,
            citations: cits.length ? JSON.stringify(cits) : "",
            retrieve_ms: doneStats?.retrieve_ms,
            generate_ms: doneStats?.generate_ms,
            total_ms: doneStats?.total_ms,
            prompt_tokens: doneStats?.prompt_tokens,
            completion_tokens: doneStats?.completion_tokens,
            total_tokens: doneStats?.total_tokens,
            annotation: annReply || undefined,
            created_at: new Date().toISOString(),
          },
        ]);
      }
      if (streamError) toast.error(streamError);
      setStreamedText("");
      setStreamedThinking("");
      setPendingCitations([]);
      setStreaming(false);
      abortRef.current = null;
      mutateQuota();
    }
  }

  const canSend = !!selectedKb && !streaming && draft.trim().length > 0;

  const chatInput = (
    <div className="rounded-2xl border bg-background shadow-sm">
      <div className="flex items-center gap-2 px-3 pt-2">
        <select
          className="h-8 rounded-md border border-input bg-background px-2 text-sm cursor-pointer"
          value={selectedKb}
          onChange={(e) => {
            setSelectedKb(e.target.value);
            if (selectedConv) selectConv("");
          }}
        >
          <option value="">{t("chat.select_kb")}</option>
          {kbs?.items?.map((k) => (
            <option key={k.id} value={k.id}>{k.name}</option>
          ))}
        </select>
      </div>
      <textarea
        ref={textareaRef}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          const ta = e.target;
          ta.style.height = "auto";
          ta.style.height = Math.min(ta.scrollHeight, 200) + "px";
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
          }
        }}
        placeholder={selectedKb ? t("chat.ask_anything") : t("chat.pick_kb_first")}
        disabled={!selectedKb || streaming}
        rows={1}
        className="w-full resize-none bg-transparent px-4 py-3 text-sm outline-none max-h-48"
      />
      <div className="flex items-center justify-between p-2">
        <span className="px-2 text-xs text-muted-foreground">
          {msgQuota && msgQuota.quota >= 0
            ? t("chat.quota_remaining", { used: Math.max(0, msgQuota.quota - msgQuota.remaining), total: msgQuota.quota })
            : ""}
        </span>
        {streaming ? (
          <Button size="icon" variant="outline" onClick={stop} aria-label={t("chat.stop")}>
            <Square className="h-4 w-4" />
          </Button>
        ) : (
          <Button size="icon" onClick={() => send()} disabled={!canSend || quotaExceeded} aria-label={t("chat.send")}>
            <Send className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );

  // Not authed: show welcome screen with login prompt.
  if (!authed) {
    return (
      <div className="flex flex-col h-full">
        <div className="flex-1 flex flex-col items-center justify-center max-w-3xl mx-auto w-full">
          <div className="text-center space-y-2 mb-8">
            <h2 className="text-2xl font-medium text-foreground">{t("chat.welcome_title")}</h2>
            <p className="text-sm text-muted-foreground">{t("chat.welcome_desc")}</p>
          </div>
          <div className="w-full">
            <div className="rounded-2xl border bg-background shadow-sm">
              <textarea
                disabled
                placeholder={t("chat.login_to_start")}
                rows={1}
                className="w-full resize-none bg-transparent px-4 py-3 text-sm outline-none max-h-48"
              />
              <div className="flex justify-end p-2">
                <Button size="icon" disabled aria-label={t("auth.sign_in")}>
                  <Send className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
          <div className="flex gap-2 mt-4">
            <Link href="/login">
              <Button>
                <LogIn className="h-4 w-4 mr-1" />
                {t("auth.sign_in")}
              </Button>
            </Link>
            {allowRegistration && (
              <Link href="/register">
                <Button variant="outline">{t("auth.create_tenant")}</Button>
              </Link>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex gap-4 h-full relative">
      {showList && (
        <div
          className="fixed inset-0 z-40 bg-black/40 md:hidden"
          onClick={() => setShowList(false)}
          aria-hidden="true"
        />
      )}
      <Card
        className={`w-64 flex-shrink-0 h-full ${
          collapsed
            ? "hidden"
            : showList
              ? "fixed left-0 top-0 z-50 h-full md:static md:z-auto flex rounded-none md:rounded-lg"
              : "hidden md:flex"
        }`}
      >
        <CardContent className="p-3 space-y-2 h-full flex flex-col w-full">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">{t("chat.conversations")}</span>
            <div className="flex items-center gap-1">
              <Button size="icon" variant="ghost" onClick={startNewChat} title={t("chat.new_conversation")} aria-label={t("chat.new_conversation")}>
                <Plus className="h-4 w-4" />
              </Button>
              <button
                onClick={() => setCollapsed(true)}
                className="hidden md:flex text-muted-foreground hover:text-foreground p-1.5"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
            </div>
          </div>
          <Input
            value={convQuery}
            onChange={(e) => setConvQuery(e.target.value)}
            placeholder={t("common.search")}
            className="h-8 text-sm"
            autoComplete="off"
            disabled={profileOpen}
          />
          <div className="flex-1 overflow-auto space-y-1">
            {!convs && Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
            {filteredConvs?.map((c) => (
              <div
                key={c.id}
                className={`group flex items-center justify-between rounded-md px-2 py-1.5 text-sm cursor-pointer ${
                  selectedConv === c.id ? "bg-accent" : "hover:bg-accent/50"
                }`}
                onClick={() => { selectConv(c.id); setShowList(false); }}
              >
                <div className="flex items-center gap-1 min-w-0">
                  {c.pinned && <Pin className="h-3 w-3 shrink-0 text-primary" />}
                  <span className="truncate">{c.title}</span>
                </div>
                <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 shrink-0">
                  <button
                    className="text-muted-foreground hover:text-primary p-0.5"
                    onClick={(e) => { e.stopPropagation(); togglePin(c.id, c.pinned); }}
                    title={c.pinned ? t("conv.unpin") : t("conv.pin")}
                  >
                    <Pin className="h-3.5 w-3.5" />
                  </button>
                  <button
                    className="text-muted-foreground hover:text-foreground p-0.5"
                    onClick={(e) => { e.stopPropagation(); rename(c.id, c.title); }}
                    title={t("conv.rename")}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button
                    className="text-muted-foreground hover:text-foreground p-0.5"
                    onClick={(e) => { e.stopPropagation(); exportConv(c.id); }}
                    title={t("conv.export")}
                  >
                    <Download className="h-3.5 w-3.5" />
                  </button>
                  <button
                    className="text-muted-foreground hover:text-destructive p-0.5"
                    onClick={(e) => { e.stopPropagation(); del(c.id); }}
                    aria-label={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))}
            {convs && filteredConvs?.length === 0 && (
              <p className="text-xs text-muted-foreground px-2 py-2">
                {t("chat.no_conversations")}
              </p>
            )}
          </div>
          <div className="border-t pt-2 space-y-1">
            {profile?.tenant_name && (
              <p className="px-2 pb-1 text-xs text-muted-foreground truncate">
                {t("nav.tenant")}: {profile.tenant_name}
              </p>
            )}
            {isAdmin && (
              <Link
                href="/dashboard"
                className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/60"
              >
                <Settings className="h-4 w-4 shrink-0" />
                {t("nav.admin")}
              </Link>
            )}
            <button
              onClick={() => setProfileOpen(true)}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/60 w-full"
            >
              <User className="h-4 w-4 shrink-0" />
              <span className="truncate">{profile?.name || t("nav.profile")}</span>
            </button>
            <button
              onClick={handleLogout}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent/60 w-full"
            >
              <LogOut className="h-4 w-4 shrink-0" />
              {t("nav.sign_out")}
            </button>
          </div>
        </CardContent>
      </Card>

      <div className="flex-1 flex flex-col min-w-0">
        <div className="flex items-center justify-between mb-3 gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setShowList(true);
              setCollapsed(false);
            }}
            className={collapsed ? "" : "md:hidden"}
            aria-label={t("chat.conversations")}
          >
            <PanelLeft className="h-4 w-4" />
          </Button>
          {selectedConv && (
            <Button
              variant="outline"
              size="sm"
              onClick={summarize}
              disabled={summarizing || messages.length < 2}
              className="ml-auto"
            >
              <Brain className="h-3.5 w-3.5 mr-1" />
              {summarizing ? t("chat.summarizing") : t("chat.summarize")}
            </Button>
          )}
        </div>
        {selectedConv ? (
          <>
            <div ref={scrollRef} className="flex-1 overflow-auto">
              <div className="max-w-3xl mx-auto space-y-4 pr-2">
                {messages.map((m) => (
                  <MessageBubble key={m.id} message={m} kbId={selectedKb} onEditSend={(text) => send(text)} />
                ))}
                {streaming && (
                  <MessageBubble
                    message={{
                      id: "streaming",
                      tenant_id: "",
                      conversation_id: selectedConv,
                      role: "assistant",
                      content: streamedText,
                      reasoning: streamedThinking || undefined,
                      citations: pendingCitations.length ? JSON.stringify(pendingCitations) : "",
                      created_at: new Date().toISOString(),
                    }}
                    streaming
                    isThinking={!!streamedThinking && !streamedText}
                    kbId={selectedKb}
                  />
                )}
              </div>
            </div>
            <div className="pt-3 max-w-3xl mx-auto w-full">
              {chatInput}
              <p className="pt-2 text-center text-xs text-muted-foreground/60 select-none">
                {t("chat.ai_disclaimer")}
              </p>
            </div>
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center max-w-3xl mx-auto w-full">
            {greeting ? (
              <div className="w-full mb-4">
                <MessageBubble
                  message={{
                    id: "greeting",
                    tenant_id: "",
                    conversation_id: "",
                    role: "assistant",
                    content: greeting,
                    created_at: new Date().toISOString(),
                  }}
                  kbId={selectedKb}
                />
              </div>
            ) : (
              <div className="text-center space-y-2 mb-8">
                <h2 className="text-2xl font-medium">{t("chat.welcome_title")}</h2>
                <p className="text-sm text-muted-foreground">{t("chat.welcome_desc")}</p>
              </div>
            )}
            {suggestedQuestions.length > 0 && (
              <div className="w-full flex flex-wrap gap-2 mb-4">
                {suggestedQuestions.map((q) => (
                  <button
                    key={q}
                    onClick={() => send(q)}
                    disabled={streaming || quotaExceeded}
                    className="rounded-full border border-border bg-background px-4 py-2 text-sm text-foreground hover:bg-accent hover:border-primary/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {q}
                  </button>
                ))}
              </div>
            )}
            <div className="w-full">{chatInput}</div>
          </div>
        )}
      </div>
      <ProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
    </div>
  );
}
