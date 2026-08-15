"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import useSWR from "swr";
import Link from "next/link";
import { ArrowLeft, Brain, Check, ChevronDown, Download, FileText, Globe, History, MessageSquareQuote, MoreVertical, Pencil, RefreshCw, Search, Settings2, Tags, Trash2, Upload, BookMarked, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api, Chunk, DocEvent, Document, EmbeddingModel, KnowledgeBase, Paginated, PreviewResult } from "@/lib/api";
import { RetrievalTestDrawer } from "@/components/kb/retrieval-test-drawer";
import { KBEditDrawer } from "@/components/kb/kb-edit-drawer";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";
import { cn, formatSize } from "@/lib/utils";
import { decodeToken, getToken } from "@/lib/auth";

const STATUS_COLOR: Record<string, string> = {
  uploaded: "bg-muted text-muted-foreground",
  queued: "bg-blue-100 text-blue-700",
  parsing: "bg-blue-100 text-blue-700",
  parsed: "bg-amber-100 text-amber-700",
  embedding: "bg-blue-100 text-blue-700",
  ready: "bg-emerald-100 text-emerald-700",
  failed: "bg-red-100 text-red-700",
};

export default function KBDetailPage() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const router = useRouter();
  const t = useTranslations();
  const confirm = useConfirm();

  const { data: kb, mutate: mutateKB } = useSWR<KnowledgeBase>(`kb-${kbId}`, () => api.getKB(kbId));
  const { data: embedData } = useSWR<Paginated<EmbeddingModel>>(`embed-${kbId}`, () => api.listEmbeddings(1, 50));
  const embedNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of embedData?.items ?? []) m.set(p.id, p.model);
    return m;
  }, [embedData]);
  const { data: docsPage, mutate } = useSWR<Paginated<Document>>(
    `docs-${kbId}`,
    () => api.listDocs(kbId, 1, 50),
    {
      refreshInterval: (data) =>
        (data?.items || []).some((d) =>
          ["queued", "parsing", "parsed", "embedding", "uploaded"].includes(d.status)
        )
          ? 15000
          : 0,
    }
  );

  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [uploadIdx, setUploadIdx] = useState(0);
  const [uploadTotal, setUploadTotal] = useState(0);
  const [uploadName, setUploadName] = useState("");
  const [backupOpen, setBackupOpen] = useState(false);
  const [retrievalOpen, setRetrievalOpen] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const dragCounter = useRef(0);
  const [docQuery, setDocQuery] = useState("");
  const [viewChunks, setViewChunks] = useState<{ docId: string; docName: string; chunks: Chunk[] } | null>(null);
  const [loadingChunks, setLoadingChunks] = useState(false);
  const [editingChunkId, setEditingChunkId] = useState<string | null>(null);
  const [editContent, setEditContent] = useState("");
  const [savingChunk, setSavingChunk] = useState(false);
  const importRef = useRef<HTMLInputElement>(null);
  const [isAdmin, setIsAdmin] = useState(false);
  // "..." overflow menu (edit/delete KB) and its edit drawer.
  const [kbMenuOpen, setKbMenuOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  // Document multi-select + batch operations.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [batchBusy, setBatchBusy] = useState(false);
  // Metadata editor (key-value pairs stored as JSON on the document).
  const [metaDoc, setMetaDoc] = useState<Document | null>(null);
  const [metaEntries, setMetaEntries] = useState<{ key: string; value: string }[]>([]);
  const [savingMeta, setSavingMeta] = useState(false);
  // Single-page URL import.
  const [urlOpen, setUrlOpen] = useState(false);
  const [urlValue, setUrlValue] = useState("");
  const [importingUrl, setImportingUrl] = useState(false);
  // Chunking preview.
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewText, setPreviewText] = useState("");
  const [previewStrategy, setPreviewStrategy] = useState("");
  const [previewSize, setPreviewSize] = useState<number | "">("");
  const [previewRes, setPreviewRes] = useState<PreviewResult | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    const token = getToken();
    if (token) {
      const payload = decodeToken(token);
      setIsAdmin(payload?.role === "admin");
    }
  }, []);
  const filteredDocs = docsPage?.items?.filter((d) =>
    d.name.toLowerCase().includes(docQuery.toLowerCase())
  );

  // Subscribe to /documents/events while any document is still in flight
  // (queued / parsing / embedding). The project authenticates with a Bearer
  // token in the Authorization header, so we use fetch streaming (via
  // api.streamDocEvents) instead of the native EventSource, which cannot set
  // custom headers. Reconnects automatically on error with a backoff.
  const hasPendingDocs = useMemo(
    () =>
      (docsPage?.items ?? []).some((d) =>
        ["queued", "parsing", "embedding"].includes(d.status)
      ),
    [docsPage?.items]
  );

  // Bridge the latest docs/mutate/t into the SSE callback without re-
  // subscribing on every status change (which would churn the connection).
  const handleDocEventRef = useRef<(evt: DocEvent) => void>(() => {});
  handleDocEventRef.current = (evt: DocEvent) => {
    if (evt.kb_id && evt.kb_id !== kbId) return;
    mutate(
      (page) => {
        if (!page) return page;
        return {
          ...page,
          items: page.items.map((d) =>
            d.id === evt.doc_id
              ? { ...d, status: evt.status, parse_error: evt.error ?? d.parse_error }
              : d
          ),
        };
      },
      false
    );
    if (evt.status === "failed") {
      const doc = docsPage?.items?.find((d) => d.id === evt.doc_id);
      toast.error(`${t("doc.failed_toast")}: ${doc?.name || evt.doc_id}`);
    }
  };

  useEffect(() => {
    if (!hasPendingDocs) return;
    const controller = new AbortController();
    let stopped = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    async function connect() {
      try {
        const stream = api.streamDocEvents(controller.signal);
        while (true) {
          const { value, done } = await stream.next();
          if (done) break;
          if (value) handleDocEventRef.current(value);
        }
        // Stream closed cleanly; reconnect if still pending and not unmounting.
        if (!stopped) reconnectTimer = setTimeout(connect, 3000);
      } catch (e) {
        if (controller.signal.aborted) return;
        console.warn("[SSE] document events stream error, reconnecting", e);
        if (!stopped) reconnectTimer = setTimeout(connect, 3000);
      }
    }

    connect();
    return () => {
      stopped = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      controller.abort();
    };
  }, [hasPendingDocs]);

  // Polling fallback: event delivery is best-effort (pub/sub), so a lost
  // terminal event would leave the list stuck in a processing state.
  // Re-fetch the list every 15s while anything is still in flight.
  useEffect(() => {
    if (!hasPendingDocs) return;
    const id = setInterval(() => mutate(), 15000);
    return () => clearInterval(id);
  }, [hasPendingDocs, mutate]);

  async function onUpload(files: FileList | null) {
    if (!files || files.length === 0) return;
    // Lazy embedding requirement: a KB created without an embedding model
    // (pure chat) must configure one before documents can be uploaded.
    if (kb && !kb.embedding_model_id) {
      toast.error(t("doc.no_embedding_configured"));
      return;
    }
    const list = Array.from(files).filter((f) =>
      /\.(pdf|docx?|md|txt|pptx|html)$/i.test(f.name)
    );
    if (list.length === 0) {
      toast.error(t("doc.unsupported_type"));
      return;
    }
    setUploading(true);
    setUploadTotal(list.length);
    try {
      for (let i = 0; i < list.length; i++) {
        setUploadIdx(i + 1);
        setUploadName(list[i].name);
        setUploadProgress(0);
        await api.uploadDoc(kbId, list[i], (pct) => setUploadProgress(pct));
      }
      mutate();
      toast.success(t("toast.uploaded"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setUploading(false);
      setUploadProgress(null);
      setUploadTotal(0);
      setUploadName("");
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  const onUploadRef = useRef(onUpload);
  onUploadRef.current = onUpload;

  useEffect(() => {
    function onPaste(e: ClipboardEvent) {
      if (e.clipboardData?.files && e.clipboardData.files.length > 0) {
        e.preventDefault();
        onUploadRef.current(e.clipboardData.files);
      }
    }
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, []);

  function onDragEnter(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current++;
    if (e.dataTransfer.types.includes("Files")) setIsDragging(true);
  }
  function onDragLeave(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current--;
    if (dragCounter.current === 0) setIsDragging(false);
  }
  function onDrop(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current = 0;
    setIsDragging(false);
    if (e.dataTransfer.files?.length) onUpload(e.dataTransfer.files);
  }

  async function showChunks(doc: Document) {
    setLoadingChunks(true);
    setEditingChunkId(null);
    setViewChunks({ docId: doc.id, docName: doc.name, chunks: [] });
    try {
      const res = await api.listChunks(kbId, doc.id);
      setViewChunks({ docId: doc.id, docName: doc.name, chunks: res.items });
    } catch (e) {
      toast.error((e as Error).message);
      setViewChunks(null);
    } finally {
      setLoadingChunks(false);
    }
  }

  function startEdit(chunk: Chunk) {
    setEditingChunkId(chunk.id);
    setEditContent(chunk.content);
  }

  function cancelEdit() {
    setEditingChunkId(null);
    setEditContent("");
  }

  async function saveChunk(chunkId: string) {
    if (!viewChunks) return;
    setSavingChunk(true);
    try {
      const updated = await api.updateChunk(kbId, viewChunks.docId, chunkId, editContent);
      setViewChunks({
        ...viewChunks,
        chunks: viewChunks.chunks.map((c) => (c.id === chunkId ? updated : c)),
      });
      setEditingChunkId(null);
      setEditContent("");
      toast.success(t("toast.chunk_updated"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSavingChunk(false);
    }
  }

  async function deleteChunk(chunkId: string) {
    if (!viewChunks) return;
    const ok = await confirm({
      title: t("doc.delete_chunk_confirm"),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteChunk(kbId, viewChunks.docId, chunkId);
      setViewChunks({
        ...viewChunks,
        chunks: viewChunks.chunks.filter((c) => c.id !== chunkId),
      });
      mutate();
      toast.success(t("toast.chunk_deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function reparse(docId: string) {
    try {
      await api.reparseDoc(kbId, docId);
      mutate();
      toast.success(t("toast.reparsed"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function del(docId: string) {
    const ok = await confirm({
      title: t("doc.delete_confirm"),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteDoc(kbId, docId);
      mutate();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function toggleEnabled(docId: string, enabled: boolean) {
    try {
      await api.setDocEnabled(kbId, docId, enabled);
      mutate(
        (page) => page ? {
          ...page,
          items: page.items.map((d) => d.id === docId ? { ...d, enabled } : d),
        } : page,
        false
      );
    } catch (e) {
      toast.error((e as Error).message);
      mutate();
    }
  }

  // ---- Batch operations ----

  const allSelected = !!filteredDocs && filteredDocs.length > 0 && filteredDocs.every((d) => selected.has(d.id));

  function toggleSelect(docId: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(docId)) next.delete(docId);
      else next.add(docId);
      return next;
    });
  }

  function toggleSelectAll() {
    setSelected((prev) => {
      if (allSelected) return new Set();
      return new Set((filteredDocs ?? []).map((d) => d.id));
    });
  }

  async function batchDelete() {
    const ids = Array.from(selected);
    if (ids.length === 0) return;
    const ok = await confirm({
      title: t("doc.batch_delete_confirm", { count: ids.length }),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    setBatchBusy(true);
    try {
      const res = await api.batchDeleteDocs(kbId, ids);
      setSelected(new Set());
      mutate();
      toast.success(t("toast.batch_deleted", { count: res.deleted }));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  async function batchReparse() {
    const ids = Array.from(selected);
    if (ids.length === 0) return;
    setBatchBusy(true);
    try {
      const res = await api.batchReparseDocs(kbId, ids);
      setSelected(new Set());
      mutate();
      toast.success(t("toast.batch_reparsed", { count: res.queued }));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  // ---- Metadata editor ----

  function openMeta(doc: Document) {
    let entries: { key: string; value: string }[] = [];
    if (doc.metadata) {
      try {
        const parsed = JSON.parse(doc.metadata) as Record<string, string>;
        entries = Object.entries(parsed).map(([key, value]) => ({ key, value: String(value) }));
      } catch {
        entries = [];
      }
    }
    setMetaEntries(entries.length > 0 ? entries : [{ key: "", value: "" }]);
    setMetaDoc(doc);
  }

  async function saveMeta() {
    if (!metaDoc) return;
    const meta: Record<string, string> = {};
    for (const { key, value } of metaEntries) {
      const k = key.trim();
      if (k) meta[k] = value;
    }
    setSavingMeta(true);
    try {
      await api.updateDocMetadata(kbId, metaDoc.id, meta);
      mutate();
      setMetaDoc(null);
      toast.success(t("toast.updated"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSavingMeta(false);
    }
  }

  // ---- URL import ----

  async function doImportURL() {
    const u = urlValue.trim();
    if (!u) return;
    setImportingUrl(true);
    try {
      await api.importURL(kbId, u);
      setUrlOpen(false);
      setUrlValue("");
      mutate();
      toast.success(t("toast.url_imported"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setImportingUrl(false);
    }
  }

  // ---- Chunking preview ----

  async function runPreview() {
    if (!previewText.trim()) return;
    setPreviewLoading(true);
    try {
      const res = await api.previewChunks(kbId, {
        text: previewText,
        strategy: previewStrategy || undefined,
        size: previewSize === "" ? undefined : Number(previewSize),
      });
      setPreviewRes(res);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setPreviewLoading(false);
    }
  }


  async function exportKB() {
    const ok = await confirm({ title: t("backup.export"), description: t("backup.export_confirm"), confirmText: t("backup.export") });
    if (!ok) return;
    try {
      await api.exportKB(kbId);
      toast.success(t("backup.exported"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function importKB(file: File) {
    const ok = await confirm({ title: t("backup.import"), description: t("backup.import_confirm"), confirmText: t("backup.import") });
    if (!ok) return;
    try {
      const res = await api.importKB(kbId, file);
      toast.success(t("backup.imported", { count: res.imported_documents }));
      mutate();
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  // Toggle KB visibility between "private" (owner only) and "team"
  // (all tenant members). Updates the cached KB object optimistically.
  async function setVisibility(visibility: "private" | "team") {
    if (!kb || kb.visibility === visibility) return;
    const prev = kb;
    mutateKB({ ...kb, visibility }, false);
    try {
      await api.setKBVisibility(kbId, visibility);
      toast.success(t("toast.updated"));
    } catch (e) {
      toast.error((e as Error).message);
      mutateKB(prev, false);
    }
  }

  // Delete the whole KB, then return to the list page.
  async function removeKB() {
    if (!kb) return;
    const ok = await confirm({
      title: t("kb.delete_confirm"),
      description: kb.name,
      confirmText: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await api.deleteKB(kbId);
      toast.success(t("toast.deleted"));
      router.push("/dashboard/knowledge-bases");
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  function statusLabel(status: string): string {
    const key = `doc.status_key.${status}`;
    const translated = t(key);
    return translated === key ? status : translated;
  }

  function metaEntriesOf(d: Document): [string, string][] {
    if (!d.metadata) return [];
    try {
      return Object.entries(JSON.parse(d.metadata) as Record<string, string>);
    } catch {
      return [];
    }
  }

  function metaCount(d: Document): number {
    return metaEntriesOf(d).length;
  }

  function metaKeys(d: Document): string[] {
    return metaEntriesOf(d).map(([k]) => k);
  }

  return (
    <div>
      <Link
        href="/dashboard/knowledge-bases"
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4"
      >
        <ArrowLeft className="h-4 w-4 mr-1" /> {t("kb.back")}
      </Link>

      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 mb-6">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-tight truncate">{kb?.name || t("kb.loading")}</h1>
          {kb && (
            <p className="text-sm text-muted-foreground mt-1">
              {embedNameById.get(kb.embedding_model_id) ?? kb.embedding_model_id} · chunk {kb.chunk_size}/{kb.chunk_overlap} · {kb.doc_count} {t("kb.docs")}
            </p>
          )}
          {kb && (
            <div className="flex items-center gap-2 mt-2">
              <span className="text-xs text-muted-foreground">{t("kb.visibility")}:</span>
              <div className="inline-flex rounded-md border overflow-hidden">
                <button
                  onClick={() => setVisibility("private")}
                  className={cn(
                    "px-2.5 py-1 text-xs transition-colors",
                    kb.visibility === "private"
                      ? "bg-primary text-primary-foreground"
                      : "bg-background hover:bg-accent"
                  )}
                  title={t("kb.visibility_private_desc")}
                >
                  {t("kb.private")}
                </button>
                <button
                  onClick={() => setVisibility("team")}
                  className={cn(
                    "px-2.5 py-1 text-xs transition-colors border-l",
                    kb.visibility === "team"
                      ? "bg-primary text-primary-foreground"
                      : "bg-background hover:bg-accent"
                  )}
                  title={t("kb.visibility_team_desc")}
                >
                  {t("kb.team_shared")}
                </button>
              </div>
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href={`/dashboard/knowledge-bases/${kbId}/pipeline`}>
            <Button variant="outline">
              <Settings2 className="h-4 w-4 mr-1" />
              {t("kb.pipeline")}
            </Button>
          </Link>
          <Link href={`/dashboard/knowledge-bases/${kbId}/agent`}>
            <Button variant="outline">
              <Brain className="h-4 w-4 mr-1" />
              {t("kb.agent")}
            </Button>
          </Link>
          <Link href={`/dashboard/knowledge-bases/${kbId}/executions`}>
            <Button variant="outline">
              <History className="h-4 w-4 mr-1" />
              {t("exec.title")}
            </Button>
          </Link>
          <Link href={`/dashboard/knowledge-bases/${kbId}/memories`}>
            <Button variant="outline">
              <BookMarked className="h-4 w-4 mr-1" />
              {t("kb.memories")}
            </Button>
          </Link>
          <Link href={`/dashboard/knowledge-bases/${kbId}/annotations`}>
            <Button variant="outline">
              <MessageSquareQuote className="h-4 w-4 mr-1" />
              {t("annotation.title")}
            </Button>
          </Link>
          {/* Low-frequency actions (edit/delete) collapsed into an overflow menu */}
          <div className="relative">
            <Button
              variant="outline"
              size="icon"
              aria-label={t("common.more")}
              onClick={() => setKbMenuOpen((v) => !v)}
            >
              <MoreVertical className="h-4 w-4" />
            </Button>
            {kbMenuOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setKbMenuOpen(false)} />
                <div className="absolute right-0 top-full mt-1 z-50 w-28 rounded-md border border-border bg-popover shadow-md py-1">
                  <button
                    onClick={() => { setKbMenuOpen(false); setEditOpen(true); }}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-accent transition-colors"
                  >
                    <Pencil className="h-3.5 w-3.5" /> {t("common.edit")}
                  </button>
                  <button
                    onClick={() => { setKbMenuOpen(false); removeKB(); }}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-sm text-destructive hover:bg-accent transition-colors"
                  >
                    <Trash2 className="h-3.5 w-3.5" /> {t("common.delete")}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
        <input
          ref={fileRef}
          type="file"
          multiple
          accept=".pdf,.docx,.doc,.md,.txt,.pptx,.html"
          className="hidden"
          onChange={(e) => onUpload(e.target.files)}
        />
        <input
          ref={importRef}
          type="file"
          accept=".json"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) importKB(f);
            e.target.value = "";
          }}
        />
      </div>

      {uploading && uploadProgress !== null && (
        <div className="mb-4">
          {uploadName && (
            <p className="text-xs text-muted-foreground mb-1 truncate">
              {uploadTotal > 1 ? `(${uploadIdx}/${uploadTotal}) ` : ""}{uploadName} — {uploadProgress}%
            </p>
          )}
          <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full bg-primary transition-all duration-200"
              style={{ width: `${uploadProgress}%` }}
            />
          </div>
        </div>
      )}

      <Card
        onDragEnter={onDragEnter}
        onDragOver={(e) => e.preventDefault()}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
        className={`relative transition-colors ${isDragging ? "border-primary border-2" : ""}`}
      >
        {isDragging && (
          <div className="absolute inset-0 z-50 flex items-center justify-center rounded-lg border-2 border-dashed border-primary bg-primary/5 pointer-events-none">
            <div className="text-center">
              <Upload className="h-10 w-10 mx-auto mb-2 text-primary" />
              <p className="text-sm font-medium text-primary">{t("doc.drop_here")}</p>
            </div>
          </div>
        )}
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">{t("doc.title")}</CardTitle>
            <Input
              value={docQuery}
              onChange={(e) => setDocQuery(e.target.value)}
              placeholder={t("common.search")}
              className="max-w-xs h-8"
            />
          </div>
        </CardHeader>
        <CardContent>
          {docsPage && docsPage.items.length === 0 ? (
            <div
              className="flex flex-col items-center justify-center py-12 px-4 text-center border-2 border-dashed border-muted-foreground/20 rounded-lg cursor-pointer hover:border-primary/40 hover:bg-accent/30 transition-colors"
              onClick={() => fileRef.current?.click()}
            >
              <Upload className="h-10 w-10 mb-3 text-muted-foreground" />
              <p className="text-sm font-medium mb-1">{t("doc.drop_here")}</p>
              <p className="text-xs text-muted-foreground">
                {t("doc.upload_hint")}
              </p>
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between mb-3">
                <p className="text-xs text-muted-foreground">
                  {t("doc.upload_hint")}
                </p>
                <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={() => setPreviewOpen(true)}>
                  <FileText className="h-3.5 w-3.5 mr-1" />
                  {t("doc.preview")}
                </Button>
                <Button size="sm" variant="outline" onClick={() => setUrlOpen(true)} disabled={uploading}>
                  <Globe className="h-3.5 w-3.5 mr-1" />
                  {t("doc.import_url")}
                </Button>
                <Button size="sm" variant="outline" onClick={() => setRetrievalOpen(true)}>
                  <Search className="h-3.5 w-3.5 mr-1" />
                  {t("retrieval_test.title")}
                </Button>
                  <Button size="sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
                    <Upload className="h-3.5 w-3.5 mr-1" />
                    {uploading
                      ? `${t("doc.uploading")} ${uploadIdx}/${uploadTotal}`
                      : t("doc.upload_short")}
                  </Button>
                  {isAdmin && (
                    <div className="relative">
                      <Button size="sm" variant="outline" onClick={() => setBackupOpen(!backupOpen)}>
                        <Download className="h-3.5 w-3.5 mr-1" />
                        {t("backup.title")}
                        <ChevronDown className="h-3 w-3 ml-0.5" />
                      </Button>
                      {backupOpen && (
                        <>
                          <div className="fixed inset-0 z-40" onClick={() => setBackupOpen(false)} />
                          <div className="absolute right-0 top-full mt-1 z-50 w-36 rounded-md border border-border bg-popover shadow-md py-1">
                            <button
                              onClick={() => { setBackupOpen(false); exportKB(); }}
                              className="flex items-center w-full px-3 py-1.5 text-sm hover:bg-accent text-left"
                            >
                              <Download className="h-3.5 w-3.5 mr-2" />
                              {t("backup.export")}
                            </button>
                            <button
                              onClick={() => { setBackupOpen(false); importRef.current?.click(); }}
                              className="flex items-center w-full px-3 py-1.5 text-sm hover:bg-accent text-left"
                            >
                              <Upload className="h-3.5 w-3.5 mr-2" />
                              {t("backup.import")}
                            </button>
                          </div>
                        </>
                      )}
                    </div>
                  )}
                </div>
              </div>
              {selected.size > 0 && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-accent/40 px-3 py-2 mb-3">
                  <span className="text-sm">{t("doc.selected_count", { count: selected.size })}</span>
                  <div className="flex gap-2">
                    <Button size="sm" variant="outline" disabled={batchBusy} onClick={batchReparse}>
                      <RefreshCw className="h-3.5 w-3.5 mr-1" />
                      {t("doc.batch_reparse")}
                    </Button>
                    <Button size="sm" variant="destructive" disabled={batchBusy} onClick={batchDelete}>
                      <Trash2 className="h-3.5 w-3.5 mr-1" />
                      {t("doc.batch_delete")}
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
                      {t("common.cancel")}
                    </Button>
                  </div>
                </div>
              )}
              <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-muted-foreground border-b">
                    <th className="py-2 pr-4 w-8">
                      <input
                        type="checkbox"
                        className="accent-primary"
                        checked={allSelected}
                        onChange={toggleSelectAll}
                        aria-label={t("doc.select_all")}
                      />
                    </th>
                    <th className="py-2 pr-4">{t("doc.name")}</th>
                    <th className="py-2 pr-4">{t("doc.status")}</th>
                    <th className="py-2 pr-4">{t("doc.enabled")}</th>
                    <th className="py-2 pr-4">{t("doc.chunks")}</th>
                    <th className="py-2 pr-4">{t("doc.hits")}</th>
                    <th className="py-2 pr-4">{t("doc.size")}</th>
                    <th className="py-2 pr-4">{t("doc.actions")}</th>
                  </tr>
                </thead>
                <tbody>
                  {!docsPage && Array.from({ length: 4 }).map((_, i) => (
                    <tr key={i} className="border-b last:border-0">
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-4" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-32" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-8" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-8" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-12" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                    </tr>
                  ))}
                  {filteredDocs?.map((d) => (
                    <tr key={d.id} className="border-b last:border-0">
                      <td className="py-2 pr-4">
                        <input
                          type="checkbox"
                          className="accent-primary"
                          checked={selected.has(d.id)}
                          onChange={() => toggleSelect(d.id)}
                          aria-label={d.name}
                        />
                      </td>
                      <td className="py-2 pr-4">
                        <div className="font-medium truncate max-w-xs">{d.name}</div>
                        {d.source_url && (
                          <a
                            href={d.source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs text-muted-foreground hover:text-primary truncate max-w-xs block"
                          >
                            {d.source_url}
                          </a>
                        )}
                        {d.metadata && metaCount(d) > 0 && (
                          <div className="text-xs text-muted-foreground">
                            <Tags className="inline h-3 w-3 mr-0.5" />
                            {metaKeys(d).join(" · ")}
                          </div>
                        )}
                        {d.parse_error && (
                          <div className="text-xs text-destructive truncate max-w-xs">{d.parse_error}</div>
                        )}
                      </td>
                      <td className="py-2 pr-4">
                        <span className={`px-2 py-0.5 rounded text-xs ${STATUS_COLOR[d.status] || ""}`}>
                          {statusLabel(d.status)}
                        </span>
                      </td>
                      <td className="py-2 pr-4">
                        <button
                          onClick={() => toggleEnabled(d.id, !d.enabled)}
                          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${d.enabled ? "bg-primary" : "bg-muted"}`}
                          title={d.enabled ? t("doc.enabled") : t("doc.disabled")}
                          aria-label={d.enabled ? t("doc.enabled") : t("doc.disabled")}
                        >
                          <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${d.enabled ? "translate-x-4" : "translate-x-1"}`} />
                        </button>
                      </td>
                      <td className="py-2 pr-4">{d.chunk_count}</td>
                      <td className="py-2 pr-4 tabular-nums">{d.hit_count ?? 0}</td>
                      <td className="py-2 pr-4">{formatSize(d.size)}</td>
                      <td className="py-2 pr-4">
                        <div className="flex gap-1">
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => openMeta(d)}
                            title={t("doc.metadata")}
                            aria-label={t("doc.metadata")}
                          >
                            <Tags className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => showChunks(d)}
                            title={t("doc.view_chunks")}
                            aria-label={t("doc.view_chunks")}
                          >
                            <FileText className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => reparse(d.id)}
                            title={t("doc.reparse")}
                            aria-label={t("doc.reparse")}
                          >
                            <RefreshCw className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => del(d.id)}
                            title={t("doc.delete")}
                            aria-label={t("doc.delete")}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {viewChunks && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/40"
            onClick={() => setViewChunks(null)}
            aria-hidden="true"
          />
          <div className="relative w-full max-w-2xl max-h-[80vh] flex flex-col rounded-lg border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold truncate">{viewChunks.docName}</h3>
              <Button size="icon" variant="ghost" onClick={() => setViewChunks(null)} aria-label={t("common.close")}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex-1 overflow-auto space-y-3">
              {loadingChunks && <Skeleton className="h-20 w-full" />}
              {!loadingChunks && viewChunks.chunks.length === 0 && (
                <p className="text-sm text-muted-foreground text-center py-8">{t("doc.no_chunks")}</p>
              )}
              {viewChunks.chunks.map((c, i) => (
                <div key={c.id} className="rounded-md border p-3">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs text-muted-foreground">#{i + 1} · {c.token_count || 0} tokens</span>
                    {editingChunkId !== c.id && (
                      <div className="flex gap-1">
                        <Button size="icon" variant="ghost" className="h-6 w-6" onClick={() => startEdit(c)} title={t("common.edit")}>
                          <Pencil className="h-3 w-3" />
                        </Button>
                        <Button size="icon" variant="ghost" className="h-6 w-6" onClick={() => deleteChunk(c.id)} title={t("common.delete")}>
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      </div>
                    )}
                  </div>
                  {editingChunkId === c.id ? (
                    <div className="space-y-2">
                      <textarea
                        className="w-full text-sm rounded-md border border-input bg-background px-3 py-2 resize-y focus:outline-none focus:ring-2 focus:ring-ring"
                        rows={5}
                        value={editContent}
                        onChange={(e) => setEditContent(e.target.value)}
                      />
                      <div className="flex justify-end gap-2">
                        <Button size="sm" variant="outline" onClick={cancelEdit} disabled={savingChunk}>
                          {t("common.cancel")}
                        </Button>
                        <Button size="sm" onClick={() => saveChunk(c.id)} disabled={savingChunk}>
                          {savingChunk ? "..." : (<><Check className="h-3.5 w-3.5 mr-1" /> {t("common.save")}</>)}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <p className="text-sm whitespace-pre-wrap">{c.content}</p>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {metaDoc && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setMetaDoc(null)} aria-hidden="true" />
          <div className="relative w-full max-w-lg max-h-[80vh] flex flex-col rounded-lg border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-1">
              <h3 className="text-lg font-semibold truncate">{t("doc.metadata")}</h3>
              <Button size="icon" variant="ghost" onClick={() => setMetaDoc(null)} aria-label={t("common.close")}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs text-muted-foreground mb-4 truncate">{metaDoc.name}</p>
            <div className="flex-1 overflow-auto space-y-2">
              {metaEntries.map((entry, i) => (
                <div key={i} className="flex items-center gap-2">
                  <Input
                    className="h-8 flex-1"
                    placeholder={t("doc.metadata_key")}
                    value={entry.key}
                    onChange={(e) =>
                      setMetaEntries((prev) => prev.map((en, j) => (j === i ? { ...en, key: e.target.value } : en)))
                    }
                  />
                  <Input
                    className="h-8 flex-1"
                    placeholder={t("doc.metadata_value")}
                    value={entry.value}
                    onChange={(e) =>
                      setMetaEntries((prev) => prev.map((en, j) => (j === i ? { ...en, value: e.target.value } : en)))
                    }
                  />
                  <Button
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8 shrink-0"
                    onClick={() => setMetaEntries((prev) => prev.filter((_, j) => j !== i))}
                    aria-label={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))}
              <Button size="sm" variant="outline" onClick={() => setMetaEntries((prev) => [...prev, { key: "", value: "" }])}>
                {t("doc.metadata_add")}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground mt-3">{t("doc.metadata_hint")}</p>
            <div className="flex justify-end gap-2 mt-4">
              <Button variant="outline" onClick={() => setMetaDoc(null)} disabled={savingMeta}>
                {t("common.cancel")}
              </Button>
              <Button onClick={saveMeta} disabled={savingMeta}>
                {savingMeta ? "..." : t("common.save")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {urlOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => !importingUrl && setUrlOpen(false)} aria-hidden="true" />
          <div className="relative w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">{t("doc.import_url")}</h3>
              <Button size="icon" variant="ghost" onClick={() => setUrlOpen(false)} disabled={importingUrl} aria-label={t("common.close")}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <Input
              autoFocus
              placeholder="https://example.com/article"
              value={urlValue}
              onChange={(e) => setUrlValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !importingUrl) doImportURL();
              }}
            />
            <p className="text-xs text-muted-foreground mt-2">{t("doc.import_url_hint")}</p>
            <div className="flex justify-end gap-2 mt-4">
              <Button variant="outline" onClick={() => setUrlOpen(false)} disabled={importingUrl}>
                {t("common.cancel")}
              </Button>
              <Button onClick={doImportURL} disabled={importingUrl || !urlValue.trim()}>
                {importingUrl ? "..." : t("doc.import_url")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {previewOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setPreviewOpen(false)} aria-hidden="true" />
          <div className="relative w-full max-w-2xl max-h-[80vh] flex flex-col rounded-lg border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">{t("doc.preview")}</h3>
              <Button size="icon" variant="ghost" onClick={() => setPreviewOpen(false)} aria-label={t("common.close")}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex flex-wrap items-center gap-2 mb-3">
              <select
                className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                value={previewStrategy}
                onChange={(e) => setPreviewStrategy(e.target.value)}
              >
                <option value="">{t("doc.preview_strategy_default")}</option>
                <option value="parent_child">parent_child</option>
                <option value="paragraph">paragraph</option>
                <option value="header">header</option>
                <option value="token">token</option>
                <option value="qa">qa (CSV)</option>
              </select>
              <Input
                className="h-8 w-28"
                type="number"
                min={1}
                placeholder={t("doc.preview_size", { size: kb?.chunk_size ?? 0 })}
                value={previewSize}
                onChange={(e) => setPreviewSize(e.target.value === "" ? "" : Number(e.target.value))}
              />
              <Button size="sm" onClick={runPreview} disabled={previewLoading || !previewText.trim()}>
                {previewLoading ? "..." : t("doc.preview_run")}
              </Button>
            </div>
            <textarea
              className="w-full text-sm rounded-md border border-input bg-background px-3 py-2 mb-3 resize-y focus:outline-none focus:ring-2 focus:ring-ring"
              rows={5}
              placeholder={t("doc.preview_placeholder")}
              value={previewText}
              onChange={(e) => setPreviewText(e.target.value)}
            />
            {previewRes && (
              <>
                <p className="text-xs text-muted-foreground mb-2">
                  {t("doc.preview_summary", {
                    total: previewRes.total,
                    tokens: previewRes.token_estimate,
                  })}
                  {previewRes.parent_count ? ` · ${t("doc.preview_parents", { count: previewRes.parent_count })}` : ""}
                </p>
                <div className="flex-1 overflow-auto space-y-2">
                  {previewRes.chunks.map((c) => (
                    <div key={c.index} className="rounded-md border p-3">
                      <span className="text-xs text-muted-foreground">#{c.index + 1} · {c.token_count} tokens</span>
                      <p className="text-sm whitespace-pre-wrap mt-1 line-clamp-6">{c.content}</p>
                    </div>
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {retrievalOpen && (
        <RetrievalTestDrawer kbId={kbId} onClose={() => setRetrievalOpen(false)} />
      )}

      {editOpen && kb && (
        <KBEditDrawer
          kb={kb}
          onClose={() => setEditOpen(false)}
          onSaved={() => {
            setEditOpen(false);
            mutateKB();
          }}
        />
      )}
    </div>
  );
}
