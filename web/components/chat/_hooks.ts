"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export function getConvIdFromURL(): string {
  if (typeof window === "undefined") return "";
  const params = new URLSearchParams(window.location.search);
  return params.get("c") || "";
}

export function parseFollowUps(raw?: string): string[] {
  if (!raw) return [];
  try {
    const v = JSON.parse(raw);
    return Array.isArray(v) ? v.filter((q) => typeof q === "string" && q.trim()) : [];
  } catch {
    return [];
  }
}

export function useThrottledText(delay = 50) {
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

export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(id);
  }, [value, delay]);
  return debounced;
}
