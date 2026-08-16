"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { Brain, ChevronLeft, Download, LogOut, MessageSquare, PanelLeft, Pin, Plus, Send, Settings, Square, Trash2, Pencil, User, LogIn } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useConfirm, usePrompt } from "@/components/ui/confirm";
import { Logo } from "@/components/brand/logo";
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
import { DocumentViewer } from "@/components/chat/document-viewer";

function getConvIdFromURL(): string {
  if (typeof window === "undefined") return "";
  const params = new URLSearchParams(window.location.search);
  return params.get("c") || "";
}

// parseFollowUps decodes a message's stored follow-up suggestions (JSON
// string array). Returns [] on missing/malformed data.
function parseFollowUps(raw?: string): string[] {
  if (!raw) return [];
  try {
    const v = JSON.parse(raw);
    return Array.isArray(v) ? v.filter((q) => typeof q === "string" && q.trim()) : [];
  } catch {
    return [];
  }
}

// useThrottledText buffers streaming token updates: tokens arrive faster
// than the screen can paint, so schedule() holds the latest value and flushes
// at most every `delay` ms; finish() cancels the timer and applies a final
// value immediately (pass "" to reset).
function useThrottledText(delay = 50) {
  const [text, setText] = useState("");
  const ref = useRef({ pending: "", timer: null as number | null });
  const schedule = useCallback(
    (v: string) => {
      const f = ref.current;
      f.pending = v;
      if (f.timer !== null) return;
      f.timer = window.setTimeout(() => {
        f.timer = null;
        setText(f.pending);
      }, delay);
    },
    [delay]
  );
  const finish = useCallback((v: string) => {
    const f = ref.current;
    if (f.timer !== null) {
      clearTimeout(f.timer);
      f.timer = null;
    }
    f.pending = v;
    setText(v);
  }, []);
  return [text, schedule, finish] as const;
}

// useDebouncedValue delays propagating fast-changing state (search input):
// the conversation list keys SWR per query, so without a debounce every
// keystroke fires a request.
function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(id);
  }, [value, delay]);
  return debounced;
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
  const [streamedText, scheduleStreamText, finishStreamText] = useThrottledText();
  const [streamedThinking, scheduleStreamThink, finishStreamThink] = useThrottledText();
  const [summarizing, setSummarizing] = useState(false);
  const [showList, setShowList] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  // Document viewer side panel: opened by clicking a citation chip. The
  // full-width chat stays visible next to it on desktop (side-by-side review).
  const [viewerCitation, setViewerCitation] = useState<Citation | null>(null);
  // Escape closes the panel; clicking the backdrop does too.
  useEffect(() => {
    if (!viewerCitation) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setViewerCitation(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [viewerCitation]);
  const confirm = useConfirm();
  const prompt = usePrompt();

  const scrollRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  // Mirror of the messages state for collision checks outside setMessages
  // updaters (the server-assigned message id must be unique in the list).
  const messagesRef = useRef<Message[]>([]);
  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const { data: kbs } = useSWR(authed ? "kb-list" : null, () => api.listKBs(1, 50));
  const [convQuery, setConvQuery] = useState("");
  const debouncedConvQuery = useDebouncedValue(convQuery);
  const { data: convs, mutate: mutateConvs } = useSWR<Paginated<Conversation>>(
    authed ? (debouncedConvQuery ? ["conv-search", debouncedConvQuery] : "conv-list") : null,
    () => api.listConversations(1, 50, debouncedConvQuery || undefined)
  );
  const filteredConvs = convs?.items?.filter((c) => c.kb_id === selectedKb);

  // DeepSeek-style grouping: pinned conversations form their own group on
  // top; the rest are bucketed by last-activity date (today / yesterday /
  // previous 7 days / previous 30 days / older). Search results stay flat.
  const convGroups = useMemo(() => {
    const items = filteredConvs ?? [];
    const now = new Date();
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const dayDiff = (d: Date) =>
      Math.floor((todayStart - new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()) / 86400000);
    const out: { key: string; items: Conversation[] }[] = [];
    const pinned = items.filter((c) => c.pinned);
    const rest = items.filter((c) => !c.pinned);
    if (pinned.length) out.push({ key: "pinned", items: pinned });
    const buckets: Record<string, Conversation[]> = { today: [], yesterday: [], week: [], month: [], older: [] };
    for (const c of rest) {
      const dd = dayDiff(new Date(c.updated_at));
      const k = dd <= 0 ? "today" : dd === 1 ? "yesterday" : dd < 8 ? "week" : dd < 31 ? "month" : "older";
      buckets[k].push(c);
    }
    for (const [key, arr] of Object.entries(buckets)) if (arr.length) out.push({ key, items: arr });
    return out;
  }, [filteredConvs]);

  const GROUP_LABELS: Record<string, string> = {
    pinned: t("conv.group_pinned"),
    today: t("conv.group_today"),
    yesterday: t("conv.group_yesterday"),
    week: t("conv.group_week"),
    month: t("conv.group_month"),
    older: t("conv.group_older"),
  };

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
  // True while this client is sending its own turn (send -> stream done).
  // The subscription stream receives the same events back; they are skipped
  // so the turn is not rendered twice.
  const selfStreamingRef = useRef(false);

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

  // Observer stream: follow the selected conversation so a turn streamed
  // from another tab/device renders here live. Self-sent turns are skipped
  // (the send() POST response already carries the same events). On done the
  // persisted messages are refetched so the final row (id, citations,
  // follow-ups) replaces the streamed approximation.
  useEffect(() => {
    if (!authed || !selectedConv) return;
    const controller = new AbortController();
    const skip = () => selfStreamingRef.current;
    (async () => {
      let acc = "";
      let thinkAcc = "";
      await consumeChatStream(api.subscribeConversation(selectedConv, controller.signal), {
        onUser: (msgId, text) => {
          if (skip()) return;
          acc = "";
          thinkAcc = "";
          setMessages((m) => [
            ...m,
            {
              id: msgId,
              tenant_id: "",
              conversation_id: selectedConv,
              role: "user",
              content: text,
              created_at: new Date().toISOString(),
            },
          ]);
          finishStreamText("");
          finishStreamThink("");
          setPendingCitations([]);
          setStreaming(true);
        },
        onCitations: (c) => {
          if (skip()) return;
          setPendingCitations(c);
        },
        onToken: (tk) => {
          if (skip()) return;
          acc += tk;
          scheduleStreamText(acc);
        },
        onThinking: (tk) => {
          if (skip()) return;
          thinkAcc += tk;
          scheduleStreamThink(thinkAcc);
        },
        onDone: () => {
          if (skip()) return;
          setStreaming(false);
          finishStreamText("");
          finishStreamThink("");
          setPendingCitations([]);
          api.listMessages(selectedConv).then((r) => setMessages(r.items)).catch(() => {});
          mutateConvs();
        },
        onFollowUps: () => {
          if (skip()) return;
          api.listMessages(selectedConv).then((r) => setMessages(r.items)).catch(() => {});
        },
      }).then((res) => {
        // A remote turn failed mid-stream (error event): end the local
        // rendering state; the partial reply is persisted server-side, so
        // refetch shows what was produced.
        if (res.error && !skip()) {
          setStreaming(false);
          finishStreamText("");
          finishStreamThink("");
          setPendingCitations([]);
          api.listMessages(selectedConv).then((r) => setMessages(r.items)).catch(() => {});
        }
      });
    })();
    return () => {
      controller.abort();
      if (!selfStreamingRef.current) setStreaming(false);
    };
  }, [authed, selectedConv, mutateConvs]);

  useEffect(() => {
    if (!scrollRef.current) return;
    requestAnimationFrame(() => {
      if (scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    });
  }, [messages, streamedText, streamedThinking, pendingCitations]);

  // Measure the floating input bar so the scroll area reserves exactly its
  // height (plus breathing room) as bottom padding. This lets the last
  // message scroll up past the input bar instead of hard-stopping at its
  // top edge, while auto-scroll keeps the tail just above the input.
  const inputBarRef = useRef<HTMLDivElement>(null);
  const [inputBarH, setInputBarH] = useState(220);
  useEffect(() => {
    let ro: ResizeObserver | null = null;
    const raf = requestAnimationFrame(() => {
      const el = inputBarRef.current;
      if (!el) return;
      ro = new ResizeObserver(() => {
        if (inputBarRef.current) setInputBarH(inputBarRef.current.offsetHeight);
      });
      ro.observe(el);
      setInputBarH(el.offsetHeight);
    });
    return () => {
      cancelAnimationFrame(raf);
      ro?.disconnect();
    };
  }, [selectedConv]);

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
        // A just-created conversation has no messages — skip the fetch.
        setMessages([]);
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
    finishStreamText("");
    finishStreamThink("");
    setPendingCitations([]);
    setStreaming(true);
    selfStreamingRef.current = true;

    const controller = new AbortController();
    abortRef.current = controller;

    let acc = "";
    let thinkAcc = "";
    let cits: Citation[] = [];
    let doneMsgId = "";
    // The id actually used for the appended assistant row. Guards against a
    // server that reuses a message id already present in the list.
    let doneFinalId = "";
    let streamError = "";
    let doneStats: ReplyStats | undefined;
    let annReply = false;
    let finalized = false;

    // finalize ends the streaming UI (final message in, cursor out) at the
    // done EVENT, not at stream close: the server keeps the stream open
    // after done to generate follow-up chips (an extra LLM call, up to
    // 15s), which must not keep the blinking cursor alive. The error/abort
    // path in the finally block reuses it.
    const finalize = (errMsg?: string) => {
      if (finalized) return;
      finalized = true;
      const content = errMsg
        ? (acc ? `${acc}\n\n⚠️ ${errMsg}` : `⚠️ ${errMsg}`)
        : acc;
      if (content) {
        doneFinalId =
          doneMsgId && !messagesRef.current.some((x) => x.id === doneMsgId)
            ? doneMsgId
            : crypto.randomUUID();
        setMessages((m) => [
          ...m,
          {
            id: doneFinalId,
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
      if (errMsg) toast.error(errMsg);
      finishStreamText("");
      finishStreamThink("");
      setPendingCitations([]);
      setStreaming(false);
    };

    try {
      const stream = api.streamChat(convId, { message: text }, controller.signal);
      const { error } = await consumeChatStream(stream, {
        onCitations: (c) => { cits = c; setPendingCitations(c); },
        onToken: (tk, annotation) => {
          acc += tk;
          scheduleStreamText(acc);
          if (annotation) annReply = true;
        },
        onThinking: (tk) => { thinkAcc += tk; scheduleStreamThink(thinkAcc); },
        onDone: (id, stats) => { doneMsgId = id; doneStats = stats; finalize(); },
        onWarning: (w) => { acc += `⚠️ ${w}\n\n`; scheduleStreamText(acc); },
        // Chips arrive after done (while the stream is still open); patch
        // them onto the already-finalized message row.
        onFollowUps: (qs) => {
          const b = JSON.stringify(qs);
          setMessages((m) => m.map((x) => (x.id === doneFinalId ? { ...x, follow_ups: b } : x)));
        },
      });
      streamError = error ?? "";
    } finally {
      finalize(streamError);
      selfStreamingRef.current = false;
      abortRef.current = null;
      mutateQuota();
    }
  }

  const canSend = !!selectedKb && !streaming && draft.trim().length > 0;

  // Index of the last assistant message; follow-up chips render only under it
  // so history stays clean and chips always track the latest reply.
  let lastAssistantIdx = -1;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === "assistant") {
      lastAssistantIdx = i;
      break;
    }
  }

  // Vote feedback on an assistant message. Clicking the active icon clears
  // the vote; optimistic update with rollback on failure.
  // Stable across renders so memoized MessageBubbles skip re-render while a
  // stream updates other state.
  const vote = useCallback(async (m: Message, v: "up" | "down") => {
    const next = m.vote === v ? "" : v;
    setMessages((ms) => ms.map((x) => (x.id === m.id ? { ...x, vote: next || undefined } : x)));
    try {
      await api.voteMessage(m.conversation_id, m.id, next);
    } catch (e: any) {
      setMessages((ms) => ms.map((x) => (x.id === m.id ? { ...x, vote: m.vote } : x)));
      toast.error(e?.message || "Vote failed");
    }
  }, []);

  // Stable wrapper around send (which is recreated every render): memoized
  // user bubbles compare props by reference and would otherwise re-render on
  // every parent state change.
  const sendRef = useRef(send);
  sendRef.current = send;
  const editSend = useCallback((text: string) => sendRef.current(text), []);

  const chatInput = (
    <div className="rounded-2xl border bg-background shadow-sm">
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
        className="w-full resize-none bg-transparent px-4 pt-3 pb-1 text-sm outline-none max-h-48"
      />
      <div className="flex items-center justify-between p-2">
        <span className="px-2 text-xs text-muted-foreground">
          {msgQuota && msgQuota.quota >= 0
            ? t("chat.quota_remaining", { used: Math.max(0, msgQuota.quota - msgQuota.remaining), total: msgQuota.quota })
            : ""}
        </span>
        <div className="flex items-center gap-2">
          <select
            className="h-8 max-w-44 rounded-md border border-input bg-background px-2 text-sm cursor-pointer"
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
          {streaming ? (
            <Button size="icon" className="h-8 w-8" variant="outline" onClick={stop} aria-label={t("chat.stop")}>
              <Square className="h-4 w-4" />
            </Button>
          ) : (
            <Button size="icon" className="h-8 w-8" onClick={() => send()} disabled={!canSend || quotaExceeded} aria-label={t("chat.send")}>
              <Send className="h-4 w-4" />
            </Button>
          )}
        </div>
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

  function renderRow(c: Conversation) {
    return (
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
            {convQuery.trim() !== ""
              ? filteredConvs?.map((c) => renderRow(c))
              : convGroups.map((g) => (
                  <div key={g.key}>
                    <p className="px-2 pb-1 pt-2 text-xs font-medium text-muted-foreground/80">
                      {GROUP_LABELS[g.key]}
                    </p>
                    {g.items.map((c) => renderRow(c))}
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

      <div className="flex-1 flex flex-col min-w-0 relative">
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
            {/* Scroll area extends to the container bottom; the input bar
                floats above it. Reserved bottom padding equals the measured
                input bar height, so the last message can scroll up past the
                input instead of stopping at its top edge. */}
            <div ref={scrollRef} className="flex-1 overflow-auto">
              <div
                className="max-w-3xl mx-auto space-y-4 pr-2"
                style={{ paddingBottom: inputBarH + 24 }}
              >
                {messages.map((m, i) => {
                  const chips = !streaming && m.role === "assistant" && i === lastAssistantIdx ? parseFollowUps(m.follow_ups) : [];
                  return (
                    <div key={m.id}>
                      <MessageBubble
                        message={m}
                        kbId={selectedKb}
                        onEditSend={m.role === "user" ? editSend : undefined}
                        onVote={m.role === "assistant" && m.id !== "greeting" ? vote : undefined}
                        onCitation={setViewerCitation}
                      />
                      {chips.length > 0 && (
                        <div className="flex flex-wrap gap-2 mt-1">
                          {chips.map((q) => (
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
                    </div>
                  );
                })}
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
                    onCitation={setViewerCitation}
                  />
                )}
              </div>
            </div>
            {/* Floating input bar with a soft gradient above it */}
            <div ref={inputBarRef} className="absolute bottom-0 left-0 right-0 z-10">
              <div className="h-10 bg-gradient-to-t from-background to-transparent pointer-events-none" />
              <div className="bg-background pb-1">
                <div className="max-w-3xl mx-auto w-full">
                  {chatInput}
                  <p className="pt-2 text-center text-xs text-muted-foreground/60 select-none">
                    {t("chat.ai_disclaimer")}
                  </p>
                </div>
              </div>
            </div>
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center max-w-3xl mx-auto w-full">
            {/* Empty state, DeepSeek-style: centered logo + one sentence. The
                headline is the knowledge base's opening message (Agent
                canvas); fall back to the site description, then the default
                welcome copy. */}
            <div className="mb-12 flex flex-col items-center gap-8 text-center">
              <div className="flex items-center gap-2">
                <Logo showName={false} size="h-6 w-6" />
                {kbs?.items?.find((k) => k.id === selectedKb)?.name && (
                  <span className="text-2xl font-bold">
                    {kbs.items.find((k) => k.id === selectedKb)!.name}
                  </span>
                )}
              </div>
              <h2 className="max-w-xl text-base font-medium tracking-tight md:text-lg">
                {greeting || t("chat.welcome_title")}
              </h2>
              {!greeting && (
                <p className="max-w-lg text-sm text-muted-foreground">
                  {settings?.site_description || t("chat.welcome_desc")}
                </p>
              )}
            </div>
            {suggestedQuestions.length > 0 && (
              <div className="mb-6 flex w-full flex-wrap justify-center gap-2">
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
      {viewerCitation && selectedKb && (
        <>
          {/* Backdrop over the chat area: clicking outside the panel closes it. */}
          <div
            className="fixed inset-0 z-30 bg-black/20"
            onClick={() => setViewerCitation(null)}
          />
          <div className="fixed inset-y-0 right-0 z-40 flex w-full flex-col border-l border-border bg-background shadow-2xl sm:w-[48%] lg:w-[44%]">
            <DocumentViewer
              kbId={selectedKb}
              citation={viewerCitation}
              onClose={() => setViewerCitation(null)}
              embed
            />
          </div>
        </>
      )}
      <ProfileDialog open={profileOpen} onOpenChange={setProfileOpen} />
    </div>
  );
}
