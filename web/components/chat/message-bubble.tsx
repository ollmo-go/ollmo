"use client";

import { memo, useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Brain, Check, Copy, FileText, Pencil, Send, ThumbsDown, ThumbsUp, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { api, Citation, Message } from "@/lib/api";
import { dedupeByDoc, formatMs, formatScore, safeParseCitations } from "@/lib/utils";
import { mdComponents } from "@/lib/markdown";
import { DocumentViewer } from "@/components/chat/document-viewer";

// normalizeAnnotationContent converts single newlines to double newlines
// so that ReactMarkdown renders proper paragraph breaks. Annotation answers
// are plain text curated by users, who naturally press Enter for line breaks
// — but markdown ignores single \n within a paragraph. This function splits
// on existing \n\n (preserving real paragraphs), then promotes any leftover
// single \n to \n\n, and rejoins.
function normalizeAnnotationContent(content: string): string {
	let normalized = content.replace(/\r\n?/g, "\n");
	return normalized
		.split("\n\n")
		.map((part) => part.replace(/\n/g, "\n\n"))
		.join("\n\n");
}

// MessageBubble renders a single chat message (user or assistant) with
// thinking panel, citations, copy/edit actions, and retrieval stats. Shared
// by the foreground ChatApp and the agent test drawer so the interaction
// surface stays identical. onEditSend is optional: when omitted the edit
// button is hidden (used by the test drawer, which is single-turn).
// Memoized: during streaming, every throttled text update re-renders the
// parent, and only the streaming bubble should re-render — historical
// bubbles skip it when their props (stable callbacks, same message refs) are
// unchanged.
export const MessageBubble = memo(function MessageBubble({
  message,
  streaming,
  isThinking,
  kbId,
  onEditSend,
  onVote,
  onCitation,
}: {
  message: Message;
  streaming?: boolean;
  isThinking?: boolean;
  kbId?: string;
  onEditSend?: (text: string) => void;
  // Vote feedback (persisted messages only). Receives the full message so
  // the parent callback can stay referentially stable across renders.
  onVote?: (message: Message, vote: "up" | "down") => void;
  // Citation click handler. When provided the parent (chat side panel) opens
  // the document viewer; otherwise the bubble falls back to its own modal
  // (agent test drawer / standalone contexts).
  onCitation?: (citation: Citation) => void;
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
  const citations: Citation[] = useMemo(
    () => (message.citations ? safeParseCitations(message.citations) : []),
    [message.citations]
  );
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
        {!isUser && message.annotation && (
          <span className="inline-flex items-center rounded px-1.5 py-px mb-1.5 text-[10px] font-medium bg-primary/10 border border-primary/20 text-primary">
            {t("chat.annotation_reply")}
          </span>
        )}
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
                {message.annotation ? normalizeAnnotationContent(message.content) : message.content}
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
                  onClick={() => (onCitation ? onCitation(c) : setViewDoc(c))}
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
            {onVote && (
              <>
                <button
                  onClick={() => onVote(message, "up")}
                  aria-label={t("chat.vote_up")}
                  title={t("chat.vote_up")}
                  className={`hover:text-foreground flex items-center gap-1 ${message.vote === "up" ? "text-green-600 dark:text-green-400" : ""}`}
                >
                  <ThumbsUp className="h-3 w-3" />
                </button>
                <button
                  onClick={() => onVote(message, "down")}
                  aria-label={t("chat.vote_down")}
                  title={t("chat.vote_down")}
                  className={`hover:text-foreground flex items-center gap-1 ${message.vote === "down" ? "text-red-600 dark:text-red-400" : ""}`}
                >
                  <ThumbsDown className="h-3 w-3" />
                </button>
              </>
            )}
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
      {viewDoc && kbId && !onCitation && (
        <DocumentViewer kbId={kbId} citation={viewDoc} onClose={() => setViewDoc(null)} />
      )}
    </div>
  );
});
