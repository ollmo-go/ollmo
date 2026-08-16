"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ChevronLeft, ChevronRight, FileText, Loader2, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { api, fetchOriginal, Citation, DocContent } from "@/lib/api";
import { mdComponents } from "@/lib/markdown";
import { cn } from "@/lib/utils";

type PageAnchor = { start: number; end: number; page: number };

type Mode = "pdf" | "markdown";

// normalizeText strips markdown artifacts and whitespace so the cited passage
// can be matched against the PDF text layer, which has neither.
function normalizeText(s: string): string {
  return s
    .replace(/[#*_`>~\-|[\]()]/g, " ")
    .replace(/\s+/g, "");
}

function parseAnchors(raw?: string): PageAnchor[] {
  if (!raw) return [];
  try {
    const v = JSON.parse(raw);
    return Array.isArray(v) ? v.filter((a) => typeof a?.start === "number" && typeof a?.end === "number" && typeof a?.page === "number") : [];
  } catch {
    return [];
  }
}

// splitPages cuts the parsed content into per-page segments using the anchor
// blocks. Boundaries sit at the first anchor of each new page.
function splitPages(content: string, anchors: PageAnchor[]): { page: number; text: string }[] {
  if (anchors.length === 0 || content.length === 0) return [{ page: 0, text: content }];
  const sorted = [...anchors].sort((a, b) => a.start - b.start);
  const boundaries: { offset: number; page: number }[] = [];
  let lastPage = -1;
  for (const a of sorted) {
    if (a.page !== lastPage) {
      boundaries.push({ offset: a.start, page: a.page });
      lastPage = a.page;
    }
  }
  if (boundaries.length === 0) return [{ page: 0, text: content }];
  const segments: { page: number; text: string }[] = [];
  for (let i = 0; i < boundaries.length; i++) {
    const start = boundaries[i].offset;
    const end = i + 1 < boundaries.length ? boundaries[i + 1].offset : content.length;
    if (end > start) segments.push({ page: boundaries[i].page + 1, text: content.slice(start, end) });
  }
  return segments;
}

/**
 * DocumentViewer renders a cited source document: the original file via
 * PDF.js when it is a PDF, the parsed markdown otherwise — with page
 * navigation and the cited passage highlighted in place.
 *
 * embed=true renders bare content for the chat side panel; otherwise the
 * viewer draws its own full-screen modal (mobile + agent test drawer).
 */
export function DocumentViewer({
  kbId,
  citation,
  onClose,
  embed,
}: {
  kbId: string;
  citation: Citation;
  onClose: () => void;
  embed?: boolean;
}) {
  const t = useTranslations();
  const [view, setView] = useState<DocContent | null>(null);
  const [error, setError] = useState("");
  const [mode, setMode] = useState<Mode>("markdown");
  const [pdfReady, setPdfReady] = useState(false);
  const [pdfFailed, setPdfFailed] = useState(false);

  const anchors = useMemo(() => parseAnchors(view?.page_anchors), [view?.page_anchors]);
  const segments = useMemo(() => splitPages(view?.content ?? "", anchors), [view?.content, anchors]);

  useEffect(() => {
    let cancelled = false;
    setError("");
    api
      .getDocContent(kbId, citation.doc_id, citation.chunk_id)
      .then((res) => {
        if (cancelled) return;
        setView(res);
        const isPdf = (res.mime_type || "").toLowerCase().includes("pdf") || citation.doc_name.toLowerCase().endsWith(".pdf");
        if (isPdf) {
          setMode("pdf");
          setPdfReady(true);
        }
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "load failed");
      });
    return () => {
      cancelled = true;
    };
  }, [kbId, citation.doc_id, citation.chunk_id, citation.doc_name]);

  const body = (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium">{view?.name || citation.doc_name || ""}</span>
          {citation.page_numbers && (
            <span className="shrink-0 text-xs text-muted-foreground">p.{citation.page_numbers}</span>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {pdfReady && (
            <div className="flex rounded-md border p-0.5">
              <button
                onClick={() => setMode("pdf")}
                className={cn("rounded px-2 py-0.5 text-xs", mode === "pdf" ? "bg-primary text-primary-foreground" : "hover:bg-accent")}
              >
                {t("viewer.mode_original")}
              </button>
              <button
                onClick={() => setMode("markdown")}
                className={cn("rounded px-2 py-0.5 text-xs", mode === "markdown" ? "bg-primary text-primary-foreground" : "hover:bg-accent")}
              >
                {t("viewer.mode_parsed")}
              </button>
            </div>
          )}
          <button onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground" aria-label={t("common.close")}>
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden bg-muted/20">
        {error && <div className="p-4 text-sm text-destructive">{error}</div>}
        {!error && !view && (
          <div className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> {t("viewer.loading")}
          </div>
        )}
        {!error && view && mode === "pdf" && pdfReady && (
          <PdfPane
            kbId={kbId}
            docId={citation.doc_id}
            citation={citation}
            onFailed={() => {
              setPdfFailed(true);
              setMode("markdown");
            }}
            t={t}
          />
        )}
        {!error && view && (mode === "markdown" || (mode === "pdf" && pdfFailed)) && (
          <MarkdownPane
            view={view}
            citation={citation}
            segments={segments}
            t={t}
          />
        )}
      </div>
    </div>
  );

  if (embed) {
    return <div className="flex h-full flex-col bg-background">{body}</div>;
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="flex h-[85vh] w-[92vw] max-w-4xl flex-col overflow-hidden rounded-lg border bg-background shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {body}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// PDF pane: PDF.js rendering with page navigation and in-place highlight of
// the cited passage (text-layer matching). Falls back gracefully when the
// passage cannot be located (e.g. scanned pages without a text layer).
// ---------------------------------------------------------------------------

type PdfPaneT = (key: string, values?: Record<string, string | number>) => string;

function PdfPane({
  kbId,
  docId,
  citation,
  onFailed,
  t,
}: {
  kbId: string;
  docId: string;
  citation: Citation;
  onFailed: () => void;
  t: PdfPaneT;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const pdfjsRef = useRef<typeof import("pdfjs-dist") | null>(null);
  const docRef = useRef<import("pdfjs-dist").PDFDocumentProxy | null>(null);
  // Serializes renders: a new render cancels the previous task, and a
  // monotonic sequence guards against a superseded render writing state after
  // a newer one finished. pdf.js forbids two concurrent render() calls on the
  // same canvas.
  const renderSeq = useRef(0);
  const renderTaskRef = useRef<import("pdfjs-dist").RenderTask | null>(null);
  const [page, setPage] = useState(1);
  const [pageCount, setPageCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [miss, setMiss] = useState(false);
  const [rects, setRects] = useState<{ x: number; y: number; w: number; h: number }[]>([]);
  const [viewportCss, setViewportCss] = useState<{ w: number; h: number } | null>(null);

  const pageRef = useRef(page);
  pageRef.current = page;

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const pdfjs = await import("pdfjs-dist");
        pdfjsRef.current = pdfjs;
        pdfjs.GlobalWorkerOptions.workerSrc = new URL(
          "pdfjs-dist/build/pdf.worker.min.mjs",
          import.meta.url
        ).toString();
        const buf = await fetchOriginal(kbId, docId);
        const pdfDoc = await pdfjs.getDocument({ data: new Uint8Array(buf) }).promise;
        if (cancelled) return;
        docRef.current = pdfDoc;
        setPageCount(pdfDoc.numPages);
        const startPage = pickStartPage(citation.page_numbers, pdfDoc.numPages);
        setPage(startPage);
        await renderPage(pdfjs, pdfDoc, startPage);
      } catch {
        if (!cancelled) onFailed();
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
      docRef.current?.destroy();
      docRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kbId, docId]);

  // Re-render at a fitted scale when the panel width changes (e.g. viewport
  // resize), so the page always fits without horizontal scrolling. Only
  // width changes trigger a re-render (height changes come from our own
  // canvas resizing and must not loop back).
  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    let lastWidth = 0;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width ?? 0;
      if (Math.abs(w - lastWidth) < 1) return;
      lastWidth = w;
      const pdfjs = pdfjsRef.current;
      const doc = docRef.current;
      if (pdfjs && doc) renderPage(pdfjs, doc, pageRef.current);
    });
    ro.observe(el);
    return () => ro.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function renderPage(
    pdfjs: typeof import("pdfjs-dist"),
    pdfDoc: import("pdfjs-dist").PDFDocumentProxy,
    pageNum: number
  ) {
    // Supersede any in-flight render: pdf.js forbids concurrent render()
    // calls on the same canvas.
    const id = ++renderSeq.current;
    renderTaskRef.current?.cancel();
    renderTaskRef.current = null;
    let task: import("pdfjs-dist").RenderTask | null = null;
    try {
      const pageObj = await pdfDoc.getPage(pageNum);
      if (id !== renderSeq.current) return; // superseded while loading the page
      // Fit the page to the pane width (with sensible bounds) so the viewer
      // never produces a horizontal scrollbar.
      const baseWidth = pageObj.getViewport({ scale: 1 }).width;
      const available = (wrapRef.current?.clientWidth ?? 600) - 48;
      const scale = Math.min(1.6, Math.max(0.6, available / baseWidth));
      const viewport = pageObj.getViewport({ scale });
      const canvas = canvasRef.current;
      if (!canvas) return;
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = Math.floor(viewport.width * dpr);
      canvas.height = Math.floor(viewport.height * dpr);
      canvas.style.width = `${viewport.width}px`;
      canvas.style.height = `${viewport.height}px`;
      setViewportCss({ w: viewport.width, h: viewport.height });
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      task = pageObj.render({ canvasContext: ctx, viewport });
      renderTaskRef.current = task;
      await task.promise;
      if (id !== renderSeq.current) return; // superseded during the render

    // Locate the cited passage in the page text layer.
    const target = normalizeText(citation.content || "").slice(0, 60);
    const tc = await pageObj.getTextContent();
    const items = (tc.items as { str: string; transform: number[] }[]).filter((it) => typeof it.str === "string" && it.str.trim());
    const found: { x: number; y: number; w: number; h: number }[] = [];
    if (target.length >= 4 && items.length > 0) {
      const normItems = items.map((it) => normalizeText(it.str));
      let acc = "";
      let start = -1;
      for (let i = 0; i < normItems.length; i++) {
        acc += normItems[i];
        while (acc.length > target.length * 3 && i - (start < 0 ? 0 : start) > 1) {
          // keep a bounded window: drop from the earliest item
          acc = acc.slice(normItems[i].length);
          break;
        }
        const idx = acc.indexOf(target);
        if (idx >= 0) {
          // find the item span that covers [idx, idx+target.length)
          const endIdx = idx + target.length;
          let pos = 0;
          let first = 0;
          for (let j = 0; j <= i; j++) {
            const len = normItems[j].length;
            if (pos + len > idx && pos <= idx) first = j;
            if (pos >= endIdx) break;
            pos += len;
          }
          for (let j = first; j <= i; j++) {
            const m = pdfjs.Util.transform(viewport.transform, items[j].transform);
            found.push({ x: m[0], y: m[1], w: m[2] - m[0], h: m[3] - m[1] });
          }
          break;
        }
        if (start < 0) start = i;
      }
    }
      setRects(found);
      setMiss(found.length === 0);
    } catch (e) {
      // A cancelled render throws RenderingCancelledException; only surface
      // real failures.
      if ((e as { name?: string })?.name === "RenderingCancelledException") return;
      throw e;
    } finally {
      if (renderTaskRef.current === task) renderTaskRef.current = null;
    }
  }

  const goto = useCallback(
    async (p: number) => {
      if (p < 1 || p > pageCount) return;
      setPage(p);
      setLoading(true);
      try {
        const pdfjs = pdfjsRef.current;
        const doc = docRef.current;
        if (!pdfjs || !doc) return;
        await renderPage(pdfjs, doc, p);
      } catch {
        /* keep previous page */
      } finally {
        setLoading(false);
      }
    },
    [pageCount]
  );

  return (
    <div ref={wrapRef} className="flex flex-col items-center p-4">
      <div className="mb-3 flex items-center gap-2 text-sm text-muted-foreground">
        <button
          onClick={() => goto(page - 1)}
          disabled={page <= 1 || loading}
          className="rounded border p-1 hover:bg-accent disabled:opacity-40"
          aria-label={t("viewer.prev_page")}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <span className="tabular-nums">
          {page} / {pageCount || "…"}
        </span>
        <button
          onClick={() => goto(page + 1)}
          disabled={page >= pageCount || loading}
          className="rounded border p-1 hover:bg-accent disabled:opacity-40"
          aria-label={t("viewer.next_page")}
        >
          <ChevronRight className="h-4 w-4" />
        </button>
        {miss && (
          <span className="ml-2 rounded bg-amber-500/10 px-2 py-0.5 text-xs text-amber-600 dark:text-amber-400">
            {t("viewer.page_level_only")}
          </span>
        )}
      </div>
      {loading && <Loader2 className="mb-3 h-5 w-5 animate-spin text-muted-foreground" />}
      <div className="relative overflow-hidden rounded border bg-white shadow">
        <canvas ref={canvasRef} />
        {viewportCss && !loading && (
          <div className="pointer-events-none absolute inset-0">
            {rects.map((r, i) => (
              <div
                key={i}
                className="absolute rounded-sm bg-yellow-300/40 ring-1 ring-yellow-500/70"
                style={{ left: r.x, top: r.y, width: r.w, height: r.h }}
              />
            ))}
          </div>
        )}
      </div>
      {miss && citation.content && (
        <div className="mt-3 w-full max-w-2xl rounded border-l-4 border-primary bg-primary/5 p-3">
          <p className="mb-1 text-xs font-medium text-primary">{t("chat.cited_passage")}</p>
          <div className="whitespace-pre-wrap text-sm">{citation.content}</div>
        </div>
      )}
    </div>
  );
}

function pickStartPage(pages: string | undefined, pageCount: number): number {
  if (!pages) return 1;
  const m = pages.match(/\d+/);
  if (!m) return 1;
  const p = parseInt(m[0], 10);
  return Math.min(Math.max(p, 1), pageCount);
}

// ---------------------------------------------------------------------------
// Markdown pane: parsed content split into pages (when anchors exist) with
// the cited passage wrapped in <mark> and scrolled into view.
// ---------------------------------------------------------------------------

function MarkdownPane({
  view,
  citation,
  segments,
  t,
}: {
  view: DocContent;
  citation: Citation;
  segments: { page: number; text: string }[];
  t: PdfPaneT;
}) {
  const hitRef = useRef<HTMLDivElement>(null);
  const multiPage = segments.length > 1;
  const offset = view.anchor_offset ?? -1;
  const cited = citation.content || "";

  // Locate which segment holds the anchor and its relative range.
  let hitSeg = -1;
  let relStart = -1;
  let relEnd = -1;
  if (offset >= 0 && cited) {
    let base = 0;
    for (let i = 0; i < segments.length; i++) {
      const len = segments[i].text.length;
      if (offset >= base && offset < base + len) {
        hitSeg = i;
        relStart = offset - base;
        relEnd = Math.min(relStart + cited.length, len);
        break;
      }
      base += len;
    }
  }

  useEffect(() => {
    if (hitRef.current) {
      hitRef.current.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  }, [hitSeg]);

  return (
    <div className="mx-auto max-w-3xl p-4">
      {hitSeg < 0 && cited && (
        <div className="mb-4 rounded border-l-4 border-primary bg-primary/5 p-3">
          <p className="mb-1 text-xs font-medium text-primary">{t("chat.cited_passage")}</p>
          <div className="whitespace-pre-wrap text-sm">{cited}</div>
        </div>
      )}
      {segments.map((seg, i) => {
        const isHit = i === hitSeg;
        let inner: React.ReactNode;
        if (isHit) {
          const before = seg.text.slice(0, relStart);
          const hit = seg.text.slice(relStart, relEnd);
          const after = seg.text.slice(relEnd);
          inner = (
            <>
              {before && (
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
                  {before}
                </ReactMarkdown>
              )}
              <div ref={hitRef}>
                <mark className="rounded bg-yellow-300/60 px-0.5 text-inherit dark:bg-yellow-500/40">
                  <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
                    {hit}
                  </ReactMarkdown>
                </mark>
              </div>
              {after && (
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
                  {after}
                </ReactMarkdown>
              )}
            </>
          );
        } else {
          inner = (
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>
              {seg.text}
            </ReactMarkdown>
          );
        }
        return (
          <section key={i} className="mb-6">
            {multiPage && (
              <div className="mb-2 flex items-center gap-2 border-b pb-1">
                <span className="rounded bg-accent px-2 py-0.5 text-xs font-medium text-muted-foreground">
                  {t("viewer.page")} {seg.page} {t("viewer.page_suffix")}
                </span>
              </div>
            )}
            {inner}
          </section>
        );
      })}
    </div>
  );
}
