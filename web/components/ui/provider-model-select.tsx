"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

// Single model row shape used by the selector. The API types (LLMModel,
// EmbeddingModel, RerankModel) all satisfy this.
interface ModelRow {
  id: string;
  name: string;
  model: string;
  provider_id?: string;
  status?: string;
}

// Provider card shape for grouping.
interface ProviderRow {
  id: string;
  name: string;
}

function groupByProvider(
  rows: ModelRow[],
  providerName: Map<string, string>
): [string, ModelRow[]][] {
  const map = new Map<string, ModelRow[]>();
  for (const m of rows) {
    const pid = m.provider_id || "";
    const name = pid ? providerName.get(pid) || "" : "";
    if (!map.has(name)) map.set(name, []);
    map.get(name)!.push(m);
  }
  return Array.from(map.entries()).sort(([a], [b]) => {
    if (!a) return 1;
    if (!b) return -1;
    return a.localeCompare(b);
  });
}

function ModelRowButton({
  m,
  selected,
  onPick,
}: {
  m: ModelRow;
  selected: boolean;
  onPick: (id: string) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onPick(m.id)}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-accent",
        selected && "bg-accent text-primary"
      )}
    >
      <span className="flex-1 truncate">{m.name || m.model}</span>
      {selected && <Check className="h-4 w-4 shrink-0" />}
    </button>
  );
}

// Two-level model picker rendered as a FastGPT-style popup: the left column
// lists providers (plus a "default" and an "all" entry), the right column
// lists the models of the active provider. Clicking a model commits it and
// closes; the "default" row commits the tenant default ("") directly.
// `value` is the model row id.
export function ProviderModelSelect({
  models,
  providers,
  value,
  onChange,
  defaultLabel,
  providerAllLabel,
  otherLabel,
  className,
  disabled,
}: {
  models: ModelRow[];
  providers: ProviderRow[];
  value: string;
  onChange: (v: string) => void;
  defaultLabel: string;
  providerAllLabel: string;
  otherLabel?: string;
  className?: string;
  disabled?: boolean;
}) {
  // Active rows, plus the currently selected one even if it was deactivated
  // so an existing selection (e.g. a KB's pinned embedding model) still
  // renders after its provider card was disabled.
  const active = useMemo(
    () => models.filter((m) => m.status === "active" || !m.status || m.id === value),
    [models, value]
  );

  const providerName = useMemo(
    () => new Map(providers.map((p) => [p.id, p.name])),
    [providers]
  );

  const current = active.find((m) => m.id === value);

  // Left-column providers that actually own models, sorted by name
  // (unnamed/other last).
  const providerEntries = useMemo(() => {
    const seen = new Map<string, string>();
    for (const m of active) {
      const pid = m.provider_id || "";
      const name = pid ? providerName.get(pid) || "" : "";
      if (!seen.has(pid)) seen.set(pid, name);
    }
    const list = Array.from(seen.entries())
      .filter(([pid]) => pid !== "")
      .sort((a, b) => (a[1] || "").localeCompare(b[1] || ""));
    if (seen.has("")) list.push(["", ""]);
    return list;
  }, [active, providerName]);

  const [open, setOpen] = useState(false);
  // Active left entry: "default" | "all" | provider id.
  const [left, setLeft] = useState<string>("all");
  const [pos, setPos] = useState({ top: 0, left: 0 });
  const btnRef = useRef<HTMLButtonElement>(null);
  const popRef = useRef<HTMLDivElement>(null);

  // Anchor the popup under the trigger with fixed coordinates so it is never
  // clipped by an ancestor with overflow (drawers, panels).
  useEffect(() => {
    if (!open || !btnRef.current) return;
    const rect = btnRef.current.getBoundingClientRect();
    const popWidth = 336;
    const left = Math.max(8, Math.min(rect.left, window.innerWidth - popWidth - 8));
    setPos({ top: rect.bottom + 4, left });
  }, [open]);

  // Close on outside click, Escape, scroll or resize.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (
        !popRef.current?.contains(e.target as Node) &&
        !btnRef.current?.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    const onReposition = () => setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", onReposition, true);
    window.addEventListener("resize", onReposition);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", onReposition, true);
      window.removeEventListener("resize", onReposition);
    };
  }, [open]);

  const onPickLeft = (key: string) => {
    setLeft(key);
    // The default row is a leaf: selecting it commits "" and closes.
    if (key === "default") {
      onChange("");
      setOpen(false);
    }
  };

  const onPickModel = (id: string) => {
    onChange(id);
    setOpen(false);
  };

  // Models shown in the right column for the active left entry.
  const rightModels = useMemo(() => {
    if (left === "default") return [];
    const scoped =
      left === "all" ? active : active.filter((m) => (m.provider_id || "") === left);
    return [...scoped].sort((a, b) => (a.name || a.model).localeCompare(b.name || b.model));
  }, [active, left]);

  const displayProvider = current?.provider_id
    ? providerName.get(current.provider_id) || ""
    : "";
  const label = current
    ? displayProvider
      ? `${displayProvider} · ${current.name || current.model}`
      : current.name || current.model
    : defaultLabel;

  const triggerCls =
    className ||
    "w-full rounded-md border border-input bg-background px-3 py-2 text-sm";

  return (
    <>
      <button
        type="button"
        ref={btnRef}
        disabled={disabled}
        onClick={() => !disabled && setOpen((v) => !v)}
        className={cn(
          triggerCls,
          "flex items-center justify-between gap-2 text-left",
          disabled && "cursor-not-allowed opacity-50"
        )}
      >
        <span className="truncate">{label}</span>
        <ChevronDown
          className={cn("h-4 w-4 shrink-0 text-muted-foreground", open && "rotate-180")}
        />
      </button>

      {open && (
        <div
          ref={popRef}
          style={{ position: "fixed", top: pos.top, left: pos.left, zIndex: 60 }}
          className="flex w-[336px] max-w-[90vw] overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-lg"
        >
          {/* Left: default / all / providers */}
          <div
            className="w-36 shrink-0 overflow-y-auto border-r border-border py-1"
            style={{ maxHeight: 320 }}
          >
            <button
              type="button"
              onClick={() => onPickLeft("default")}
              className={cn(
                "flex w-full items-center px-3 py-1.5 text-left text-sm hover:bg-accent",
                left === "default" && "bg-accent text-primary"
              )}
            >
              <span className="truncate">{defaultLabel}</span>
            </button>
            <button
              type="button"
              onClick={() => onPickLeft("all")}
              className={cn(
                "flex w-full items-center px-3 py-1.5 text-left text-sm hover:bg-accent",
                left === "all" && "bg-accent text-primary"
              )}
            >
              <span className="truncate">{providerAllLabel}</span>
            </button>
            {providerEntries.map(([pid, name]) => (
              <button
                key={pid || "_"}
                type="button"
                onClick={() => onPickLeft(pid)}
                className={cn(
                  "flex w-full items-center px-3 py-1.5 text-left text-sm hover:bg-accent",
                  left === pid && "bg-accent text-primary"
                )}
              >
                <span className="truncate">{name || otherLabel || "Other"}</span>
              </button>
            ))}
          </div>

          {/* Right: models of the active provider */}
          <div className="flex-1 overflow-y-auto py-1" style={{ maxHeight: 320 }}>
            {left === "default" ? (
              <div className="px-3 py-2 text-xs text-muted-foreground">{defaultLabel}</div>
            ) : left === "all" ? (
              groupByProvider(rightModels, providerName).map(([pname, items]) => (
                <div key={pname || "_"}>
                  {pname && (
                    <div className="px-3 pt-1.5 pb-0.5 text-xs font-medium text-muted-foreground">
                      {pname}
                    </div>
                  )}
                  {items.map((m) => (
                    <ModelRowButton
                      key={m.id}
                      m={m}
                      selected={m.id === value}
                      onPick={onPickModel}
                    />
                  ))}
                </div>
              ))
            ) : (
              rightModels.map((m) => (
                <ModelRowButton key={m.id} m={m} selected={m.id === value} onPick={onPickModel} />
              ))
            )}
          </div>
        </div>
      )}
    </>
  );
}
