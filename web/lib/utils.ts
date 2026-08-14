import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

import type { Citation } from "@/lib/api";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Format an ISO timestamp as a localized string. Falls back to the raw
 *  string if the input is not a valid date. */
export function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

/** Format a byte count as a human-readable size string. */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

/** Format a millisecond duration as a compact label. */
export function formatMs(ms?: number): string {
  if (!ms || ms <= 0) return "0ms";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

/** Format a retrieval/rerank score: ≤1 as percentage, >1 as decimal. */
export function formatScore(score: number): string {
  if (score <= 0) return "";
  if (score <= 1) return `${(score * 100).toFixed(1)}%`;
  return score.toFixed(2);
}

/** Parse a JSON-encoded citation array, returning [] on any error. */
export function safeParseCitations(s: string): Citation[] {
  try {
    const v = JSON.parse(s);
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

/** Deduplicate citations by document, keeping a hit count and the top score
 *  so the UI can show "doc_name (count)" instead of repeated entries. */
export function dedupeByDoc(cits: Citation[]): (Citation & { count: number; topScore: number })[] {
  const map = new Map<string, Citation & { count: number; topScore: number }>();
  for (const c of cits) {
    const key = c.doc_id || c.doc_name;
    const ex = map.get(key);
    const score = typeof c.score === "number" ? c.score : 0;
    if (ex) {
      ex.count++;
      if (score > ex.topScore) ex.topScore = score;
    } else {
      map.set(key, { ...c, count: 1, topScore: score });
    }
  }
  return Array.from(map.values());
}
