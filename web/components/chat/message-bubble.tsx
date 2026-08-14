"use client";

import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Brain, Check, Copy, FileText, Pencil, Send, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { api, Citation, Message } from "@/lib/api";
import { dedupeByDoc, formatMs, formatScore, safeParseCitations } from "@/lib/utils";
import { mdComponents } from "@/lib/markdown";

// MessageBubble renders a single chat message (user or assistant) with
// thinking panel, citations, copy/edit actions, and retrieval stats. Shared
// by the foreground ChatApp and the agent test drawer so the interaction
// surface stays identical. onEditSend is optional: when omitted the edit
// button is hidden (used by the test drawer, which is single-turn).
export function MessageBubble({
  message,
  streaming,
  isThinking,
  kbId,
  onEditSend,
}: {
  message: Message;
  streaming?: boolean;
  isThinking?: boolean;
  kbId?: string;
  onEditSend?: (text: string) => void;
}) {
  const isUser = message.role === "user";
  const [copied, setCopied] = useState(false);
  const [viewDoc, setViewDoc] = useState<Citation | null>(null);
  const [showThinking, setShowThinking] = useState(true);
  const [editing, setEditing] = useState(false);
  const [editText, setEditText] = useState("");
  const reasoningRef = useRef<HTMLDivElement>(null);

  // Auto-scroll the reasoning panel as thinking tokens stream in. The panel
  // has its own max-h-60 overflow-auto, so the outer container's auto-scroll
  // doesn't reach it — we need to scroll this inner div directly.
  useEffect(() => {
    if (streaming && isThinking && reasoningRef.current) {
      reasoningRef.current.scrollTop = reasoningRef.current.scrollHeight;
    }
  }, [message.reasoning, streaming, isThinking]);
  const citations: Citation[] = message.citations
    ? safeParseCitations(message.citations)
    : [];
  const t = useTranslations();
  const hasStats =
    !!(message.total_ms || message.retrieve_ms || message.generate_ms || message.total_tokens);
  const hasReasoning = !!message.reasoning;

  useEffect(() => {
    if (streaming && isThinking) setShowThinking(true);
  }, [streaming, isThinking, hasReasoning]);

  function copy() {
    navigator.clipboard.writeText(message.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"} group relative`}>
      <div
        className={`${
          isUser && editing
            ? "w-full rounded-2xl border bg-background shadow-sm text-foreground"
            : `max-w-[80%] rounded-lg px-4 py-2 ${
              isUser ? "bg-primary text-primary-foreground" : "bg-muted"
            }`
        }`}
      >
        {hasReasoning && !isUser && (
          <div className="mb-2 border-b border-border/40 pb-2">
            <button
              onClick={() => setShowThinking((v) => !v)}
              className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
            >
              <Brain className={`h-3 w-3 ${isThinking ? "animate-pulse text-primary" : ""}`} />
              <span>{isThinking ? t("chat.thinking") : t("chat.thought_process")}</span>
              <span className="text-[10px] opacity-60">{showThinking ? "▾" : "▸"}</span>
            </button>
            {showThinking && (
              <div ref={reasoningRef} className="mt-1.5 text-xs text-muted-foreground/80 whitespace-pre-wrap leading-relaxed max-h-60 overflow-auto">
                {message.reasoning}
                {isThinking && <span className="inline-block w-1 h-3 ml-0.5 bg-current animate-pulse align-middle" />}
              </div>
            )}
          </div>
        )}
        <div className="text-sm">
          {isUser ? (
            editing ? (
              <div>
                <textarea
                  value={editText}
                  onChange={(e) => setEditText(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      if (editText.trim() && onEditSend) onEditSend(editText.trim());
                      setEditing(false);
                    }
                  }}
                  className="w-full resize-none bg-transparent px-4 py-3 text-sm outline-none max-h-48"
                  rows={3}
                  autoFocus
                />
                <div className="flex justify-end gap-2 p-2">
                  <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
                    {t("common.cancel")}
                  </Button>
                  <Button size="sm" onClick={() => { if (editText.trim() && onEditSend) onEditSend(editText.trim()); setEditing(false); }}>
                    <Send className="h-3 w-3 mr-1" />
                    {t("chat.send")}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="whitespace-pre-wrap">{message.content}</div>
            )
          ) : (
            <div className={`chat-markdown ${streaming && message.content ? "streaming" : ""}`}>
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
                {message.content}
              </ReactMarkdown>
            </div>
          )}
          {streaming && !message.content && !hasReasoning && (
            <span className="inline-flex items-center gap-1 py-1.5" aria-label={t("chat.thinking")}>
              <span className="h-1.5 w-1.5 rounded-full bg-current opacity-40 animate-bounce" style={{ animationDelay: "0ms" }} />
              <span className="h-1.5 w-1.5 rounded-full bg-current opacity-40 animate-bounce" style={{ animationDelay: "150ms" }} />
              <span className="h-1.5 w-1.5 rounded-full bg-current opacity-40 animate-bounce" style={{ animationDelay: "300ms" }} />
            </span>
          )}
        </div>
        {!isUser && citations.length > 0 && (
          <div className="mt-2 pt-2 border-t border-border/40 text-xs space-y-0.5">
            <p className="font-medium opacity-70">{t("chat.sources")}</p>
            {dedupeByDoc(citations).map((c, i) => {
              const scoreLabel = formatScore(c.topScore);
              return (
                <button
                  key={i}
                  onClick={() => setViewDoc(c)}
                  className="opacity-80 hover:opacity-100 hover:underline cursor-pointer flex items-center gap-1.5 text-left"
                >
                  <span>
                    {c.doc_name || c.doc_id.slice(0, 8)}
                    {c.count > 1 && <span className="opacity-60"> ({c.count})</span>}
                  </span>
                  {c.page_numbers && (
                    <span className="opacity-60">p.{c.page_numbers}</span>
                  )}
                  {scoreLabel && (
                    <span
                      className="inline-flex items-center rounded px-1 py-px text-[10px] font-medium bg-background/60 border border-border/50 text-muted-foreground"
                      title="retrieval score"
                    >
                      {scoreLabel}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        )}
        {!isUser && !streaming && message.content && (
          <div className="mt-1 flex items-center gap-3 text-[10px] opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground">
            <button
              onClick={copy}
              aria-label={t("chat.copy")}
              className="hover:text-foreground flex items-center gap-1"
            >
              {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            </button>
            {hasStats && (
              <span className="flex items-center gap-2 select-none">
                {(message.retrieve_ms ?? 0) > 0 && (
                  <span>{t("chat.stats_retrieve")} {formatMs(message.retrieve_ms)}</span>
                )}
                {(message.generate_ms ?? 0) > 0 && (
                  <span>{t("chat.stats_generate")} {formatMs(message.generate_ms)}</span>
                )}
                {(message.total_ms ?? 0) > 0 && (
                  <span>{t("chat.stats_total")} {formatMs(message.total_ms)}</span>
                )}
                {(message.total_tokens ?? 0) > 0 && (
                  <span
                    title={`${t("chat.stats_prompt")}: ${message.prompt_tokens ?? 0} / ${t("chat.stats_completion")}: ${message.completion_tokens ?? 0}`}
                  >
                    {t("chat.stats_tokens")} {message.total_tokens}
                  </span>
                )}
              </span>
            )}
          </div>
        )}
      </div>
      {isUser && !streaming && message.content && !editing && (
        <div className="absolute right-0 -bottom-5 flex items-center gap-3 text-[10px] opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground">
          <button
            onClick={copy}
            aria-label={t("chat.copy")}
            className="hover:text-foreground flex items-center gap-1"
          >
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          </button>
          {onEditSend && (
            <button
              onClick={() => { setEditText(message.content); setEditing(true); }}
              aria-label={t("common.edit")}
              className="hover:text-foreground flex items-center gap-1"
            >
              <Pencil className="h-3 w-3" />
            </button>
          )}
        </div>
      )}
      {viewDoc && kbId && (
        <DocViewerModal kbId={kbId} citation={viewDoc} onClose={() => setViewDoc(null)} />
      )}
    </div>
  );
}

// DocViewerModal loads a cited document's full content and shows the cited
// passage alongside it. Opened by clicking a citation in MessageBubble.
export function DocViewerModal({ kbId, citation, onClose }: {
  kbId: string;
  citation: Citation;
  onClose: () => void;
}) {
  const [content, setContent] = useState("");
  const [docName, setDocName] = useState(citation.doc_name || "");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const t = useTranslations();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    api.getDocContent(kbId, citation.doc_id)
      .then((res) => {
        if (cancelled) return;
        setDocName(res.name || citation.doc_name || "");
        setContent(res.content || "");
      })
      .catch((e: any) => {
        if (cancelled) return;
        setError(e?.message || "Failed to load document");
      })
      .finally(() => !cancelled && setLoading(false));
    return () => { cancelled = true; };
  }, [kbId, citation.doc_id]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="bg-background border rounded-lg shadow-xl w-[90vw] max-w-3xl h-[80vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <div className="flex items-center gap-2 min-w-0">
            <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
            <span className="font-medium truncate">{docName}</span>
            {citation.page_numbers && (
              <span className="text-xs text-muted-foreground shrink-0">p.{citation.page_numbers}</span>
            )}
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground shrink-0">
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 overflow-auto p-4 space-y-4">
          {loading && (
            <div className="space-y-2">
              <div className="h-4 bg-muted rounded animate-pulse" />
              <div className="h-4 bg-muted rounded animate-pulse w-3/4" />
              <div className="h-4 bg-muted rounded animate-pulse w-1/2" />
            </div>
          )}
          {error && (
            <div className="text-sm text-destructive">{error}</div>
          )}
          {!loading && !error && (
            <>
              {citation.content && (
                <div className="border-l-4 border-primary bg-primary/5 rounded-r p-3">
                  <p className="text-xs font-medium text-primary mb-1">{t("chat.cited_passage")}</p>
                  <div className="text-sm whitespace-pre-wrap">{citation.content}</div>
                </div>
              )}
              {content ? (
                <div className="chat-markdown text-sm">
                  <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
                    {content}
                  </ReactMarkdown>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">{t("chat.no_content")}</p>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
