"use client";

import { useState } from "react";
import useSWR from "swr";
import { Search, Loader2, ToggleLeft, ToggleRight, ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Drawer } from "@/components/ui/drawer";
import { api, Paginated, ProviderCard, RerankModel, SearchResult, SearchHit } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { ProviderModelSelect } from "@/components/ui/provider-model-select";

export function RetrievalTestDrawer({ kbId, onClose }: { kbId: string; onClose: () => void }) {
  const t = useTranslations();
  const { data: reranksData } = useSWR<Paginated<RerankModel>>("rerank-list", () =>
    api.listReranks(1, 50)
  );
  const reranks = reranksData?.items ?? [];
  const { data: providerCards } = useSWR<ProviderCard[]>("providers", () =>
    api.providers.list()
  );
  const providers = providerCards ?? [];

  const [query, setQuery] = useState("");
  const [topK, setTopK] = useState(10);
  const [rerank, setRerank] = useState(false);
  const [rerankModelId, setRerankModelId] = useState("");
  const [vectorWeight, setVectorWeight] = useState(0.5);
  const [debugMode, setDebugMode] = useState(true);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<SearchResult | null>(null);
  const [error, setError] = useState("");

  async function handleSearch() {
    if (!query.trim()) return;
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const body = {
        query: query.trim(),
        top_k: topK,
        rerank: rerank,
        rerank_model_id: rerank ? rerankModelId || undefined : undefined,
        vector_weight: vectorWeight,
      };
      const res = debugMode
        ? await api.searchDebug(kbId, body)
        : await api.search(kbId, body);
      setResult(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSearch();
    }
  }

  return (
    <Drawer title={t("retrieval_test.title")} onClose={onClose} width="max-w-2xl">
      <div className="space-y-4">
        {/* Query input */}
        <div className="space-y-3">
          <div className="flex gap-2">
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t("retrieval_test.query_placeholder")}
              className="flex-1"
            />
            <Button onClick={handleSearch} disabled={loading || !query.trim()}>
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Search className="h-4 w-4" />
              )}
            </Button>
          </div>

          <div className="flex flex-wrap items-center gap-4 text-sm">
            <label className="flex items-center gap-2">
              <span className="text-muted-foreground">{t("retrieval_test.top_k")}</span>
              <Input
                type="number"
                min={1}
                max={50}
                value={topK}
                onChange={(e) => setTopK(Number(e.target.value) || 10)}
                className="w-20"
              />
            </label>

            <button
              onClick={() => setRerank(!rerank)}
              className="flex items-center gap-1 text-muted-foreground hover:text-foreground"
            >
              {rerank ? (
                <ToggleRight className="h-5 w-5 text-primary" />
              ) : (
                <ToggleLeft className="h-5 w-5" />
              )}
              {t("retrieval_test.rerank")}
            </button>

            {rerank && (
              <div className="w-64">
                <ProviderModelSelect
                  models={reranks}
                  providers={providers}
                  value={rerankModelId}
                  onChange={setRerankModelId}
                  defaultLabel={t("retrieval_test.rerank_default")}
                  providerAllLabel={t("common.provider_all")}
                  otherLabel={t("common.provider_other")}
                />
              </div>
            )}

            <button
              onClick={() => setDebugMode(!debugMode)}
              className="flex items-center gap-1 text-muted-foreground hover:text-foreground"
            >
              {debugMode ? (
                <ToggleRight className="h-5 w-5 text-primary" />
              ) : (
                <ToggleLeft className="h-5 w-5" />
              )}
              {t("retrieval_test.debug_mode")}
            </button>
          </div>

          <div className="flex items-center gap-3 text-sm">
            <span className="text-muted-foreground whitespace-nowrap">
              {t("retrieval_test.vector_weight")}
            </span>
            <span className="text-xs text-muted-foreground whitespace-nowrap">
              {t("retrieval_test.keyword_weight")}
            </span>
            <input
              type="range"
              min={0}
              max={1}
              step={0.1}
              value={vectorWeight}
              onChange={(e) => setVectorWeight(Number(e.target.value))}
              className="flex-1 min-w-[140px] accent-primary"
              aria-label={t("retrieval_test.vector_weight")}
            />
            <span className="text-xs text-muted-foreground whitespace-nowrap">
              {t("retrieval_test.semantic_weight")}
            </span>
            <span className="font-mono text-xs text-primary w-8 text-right">
              {vectorWeight.toFixed(1)}
            </span>
          </div>
        </div>

        {error && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        {result && (
          <ResultView result={result} debug={debugMode} />
        )}
      </div>
    </Drawer>
  );
}

function ResultView({ result, debug }: { result: SearchResult; debug: boolean }) {
  const t = useTranslations();

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2 text-xs">
        <Badge color="blue">
          {result.hits.length} {t("retrieval_test.hits")}
        </Badge>
        {result.sparse && (
          <Badge color="green">{t("retrieval_test.sparse_on")}</Badge>
        )}
        {result.rerank && (
          <Badge color="purple">{t("retrieval_test.rerank_on")}</Badge>
        )}
        {result.graph_context && (
          <Badge color="amber">{t("retrieval_test.graph_on")}</Badge>
        )}
      </div>

      {result.hits.length === 0 && (
        <p className="text-center text-muted-foreground py-8">
          {t("retrieval_test.no_hits")}
        </p>
      )}

      <div className="space-y-2">
        {result.hits.map((hit, i) => (
          <HitCard
            key={hit.chunk_id}
            hit={hit}
            rank={i + 1}
            debug={debug}
            denseScore={result.debug_dense_hits?.find((d) => d.chunk_id === hit.chunk_id)?.score}
            sparseScore={result.debug_sparse_hits?.find((d) => d.chunk_id === hit.chunk_id)?.score}
            fusedScore={result.debug_fused_scores?.[hit.chunk_id]}
          />
        ))}
      </div>

      {debug && result.graph_context && (
        <div className="rounded-lg border p-3">
          <p className="text-xs font-medium mb-2">{t("retrieval_test.graph_context")}</p>
          <pre className="text-xs whitespace-pre-wrap text-muted-foreground">
            {result.graph_context}
          </pre>
        </div>
      )}
    </div>
  );
}

function HitCard({
  hit,
  rank,
  debug,
  denseScore,
  sparseScore,
  fusedScore,
}: {
  hit: SearchHit;
  rank: number;
  debug: boolean;
  denseScore?: number;
  sparseScore?: number;
  fusedScore?: number;
}) {
  const t = useTranslations();
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="rounded-lg border p-3">
      <div className="flex items-start gap-3">
        <span className="flex-shrink-0 w-6 h-6 rounded-full bg-primary/10 text-primary text-xs flex items-center justify-center font-medium">
          {rank}
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-sm font-medium truncate">{hit.doc_name}</span>
            {hit.page_numbers && (
              <span className="text-xs text-muted-foreground">
                {t("retrieval_test.page")} {hit.page_numbers}
              </span>
            )}
          </div>
          <p
            className={cn(
              "text-sm text-muted-foreground",
              !expanded && "line-clamp-3"
            )}
          >
            {hit.content}
          </p>
          {hit.content.length > 200 && (
            <button
              onClick={() => setExpanded(!expanded)}
              className="text-xs text-primary hover:underline mt-1 flex items-center gap-1"
            >
              {expanded ? (
                <ChevronDown className="h-3 w-3" />
              ) : (
                <ChevronRight className="h-3 w-3" />
              )}
              {expanded ? t("retrieval_test.collapse") : t("retrieval_test.expand")}
            </button>
          )}
          {debug && (
            <div className="flex flex-wrap gap-2 mt-2 text-xs">
              <ScoreBadge label={t("retrieval_test.score_final")} value={hit.score} color="blue" />
              {fusedScore !== undefined && (
                <ScoreBadge label="RRF" value={fusedScore} color="green" />
              )}
              {denseScore !== undefined && (
                <ScoreBadge label={t("retrieval_test.score_dense")} value={denseScore} color="purple" />
              )}
              {sparseScore !== undefined && (
                <ScoreBadge label={t("retrieval_test.score_sparse")} value={sparseScore} color="amber" />
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function Badge({ children, color }: { children: React.ReactNode; color: "blue" | "green" | "purple" | "amber" }) {
  const colors = {
    blue: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
    green: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400",
    purple: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
    amber: "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400",
  };
  return (
    <span className={cn("px-2 py-0.5 rounded-full text-xs font-medium", colors[color])}>
      {children}
    </span>
  );
}

function ScoreBadge({ label, value, color }: { label: string; value: number; color: "blue" | "green" | "purple" | "amber" }) {
  const colors = {
    blue: "bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400",
    green: "bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-400",
    purple: "bg-purple-50 text-purple-600 dark:bg-purple-900/20 dark:text-purple-400",
    amber: "bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-400",
  };
  return (
    <span className={cn("px-1.5 py-0.5 rounded font-mono", colors[color])}>
      {label}: {value.toFixed(4)}
    </span>
  );
}
