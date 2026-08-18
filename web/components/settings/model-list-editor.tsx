"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, Plus, RefreshCw, Star, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { api, DiscoveredModel } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { CapacityField } from "./capacity-field";
import type { CapacityFieldKey, DraftModel, ProviderKind } from "./_shared";
import { nextKey, parseCapacity, formatCapacity } from "./_shared";

export function ModelListEditor({
  kind,
  models,
  onChange,
  probe,
  testing,
  onTest,
  onMakeDefault,
  disabled,
  title,
}: {
  kind: ProviderKind;
  models: DraftModel[];
  onChange: (models: DraftModel[]) => void;
  probe: { endpoint: string; api_key?: string; provider_id?: string };
  testing?: string | null;
  onTest?: (mid: string) => void;
  onMakeDefault?: (rowId: string, isDefault: boolean) => void;
  disabled?: boolean;
  title?: string;
}) {
  const t = useTranslations();
  const [probing, setProbing] = useState(false);
  const [candidates, setCandidates] = useState<DiscoveredModel[] | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [textBuf, setTextBuf] = useState<Record<string, string>>({});
  const [localTesting, setLocalTesting] = useState<string | null>(null);
  const [testedKeys, setTestedKeys] = useState<Set<string>>(new Set());

  function patch(key: string, next: Partial<DraftModel>) {
    onChange(models.map((m) => (m.key === key ? { ...m, ...next } : m)));
  }

  function toggleExpanded(key: string) {
    setExpanded((cur) => {
      const next = new Set(cur);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  }

  type CF = CapacityFieldKey;

  function fieldText(m: DraftModel, field: CF): string {
    const buf = textBuf[`${m.key}:${field}`];
    if (buf !== undefined) return buf;
    if (field === "context_length") return formatCapacity(m.context_length);
    const v = m[field];
    return v === undefined || v === null || v === 0 ? "" : String(v);
  }

  function setField(m: DraftModel, field: CF, raw: string) {
    setTextBuf((cur) => ({ ...cur, [`${m.key}:${field}`]: raw }));
    if (field === "context_length" || field === "max_tokens") {
      patch(m.key, { [field]: parseCapacity(raw) } as Partial<DraftModel>);
    } else {
      const n = raw.trim() === "" ? undefined : Number(raw);
      patch(m.key, { [field]: n !== undefined && Number.isFinite(n) ? n : undefined } as Partial<DraftModel>);
    }
  }

  function settleField(m: DraftModel, field: CF) {
    setTextBuf((cur) => {
      const next = { ...cur };
      delete next[`${m.key}:${field}`];
      return next;
    });
  }

  async function testRow(m: DraftModel) {
    if (m.rowId) {
      onTest?.(m.rowId);
      return;
    }
    if (!m.model.trim()) {
      toast.error(t("settings.provider_model_id_empty"));
      return;
    }
    if (!probe.endpoint) {
      toast.error(t("settings.fetch_need_endpoint"));
      return;
    }
    setLocalTesting(m.key);
    try {
      await api.providers.probeModel({
        kind,
        endpoint: probe.endpoint,
        api_key: probe.api_key ?? "",
        model: m.model.trim(),
        ...(probe.provider_id ? { provider_id: probe.provider_id } : {}),
      });
      toast.success(`${m.model.trim()} OK`);
      setTestedKeys((cur) => {
        const n = new Set(cur);
        n.add(m.key);
        return n;
      });
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setLocalTesting(null);
    }
  }

  async function fetchModels() {
    setProbing(true);
    try {
      const found = await api.providers.probe({
        endpoint: probe.endpoint,
        ...(probe.api_key ? { api_key: probe.api_key } : {}),
        ...(probe.provider_id ? { provider_id: probe.provider_id } : {}),
      });
      if (found.length === 0) {
        toast.error(t("settings.fetch_models_empty"));
        return;
      }
      const known = new Set(models.map((m) => m.model));
      setCandidates(found);
      setPicked(new Set(found.filter((m) => !known.has(m.id)).map((m) => m.id)));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setProbing(false);
    }
  }

  function adoptPicked() {
    if (!candidates) return;
    const byModel = new Map(models.map((m) => [m.model, m]));
    for (const c of candidates) {
      if (!picked.has(c.id)) continue;
      if (byModel.has(c.id)) continue;
      byModel.set(c.id, {
        key: nextKey(),
        model: c.id,
        name: c.name || "",
        context_length: c.context_length,
      });
    }
    onChange([...byModel.values()]);
    setCandidates(null);
    setPicked(new Set());
  }

  const askable = probe.endpoint.length > 0;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{title ?? t("settings.provider_models_title")}</div>
        <button
          type="button"
          className="text-xs text-muted-foreground hover:text-foreground disabled:opacity-50"
          disabled={disabled || probing || !askable}
          title={!askable ? t("settings.fetch_need_endpoint") : undefined}
          onClick={fetchModels}
        >
          <RefreshCw className={cn("mr-1 inline h-3.5 w-3.5", probing && "animate-spin")} />
          {probing ? t("common.loading") : t("settings.fetch_models")}
        </button>
      </div>

      {models.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("settings.provider_models_empty")}</p>
      )}

      {models.map((m) => {
        const open = expanded.has(m.key);
        return (
          <div key={m.key} className="rounded-md border border-input px-2 py-1.5">
            <div className="flex items-center gap-2">
              {m.rowId ? (
                <span className="h-9 min-w-0 flex-1 truncate rounded-md border border-input bg-muted/40 px-3 pt-2 font-mono text-xs leading-5">
                  {m.model}
                </span>
              ) : (
                <Input
                  className="min-w-0 flex-1 font-mono text-xs"
                  placeholder={t("settings.provider_model_id")}
                  value={m.model}
                  disabled={disabled}
                  onChange={(e) => patch(m.key, { model: e.target.value })}
                />
              )}
              <Input
                className="min-w-0 flex-1 text-xs"
                placeholder={t("settings.provider_model_name")}
                value={m.name}
                disabled={disabled}
                onChange={(e) => patch(m.key, { name: e.target.value })}
              />
              <button
                type="button"
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
                aria-label={t("settings.model_capacity")}
                aria-expanded={open}
                title={t("settings.model_capacity")}
                disabled={disabled}
                onClick={() => toggleExpanded(m.key)}
              >
                {open ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </button>
              {(m.rowId || testedKeys.has(m.key)) && (
                <button
                  type="button"
                  className="shrink-0 text-muted-foreground hover:text-primary disabled:opacity-50"
                  title={m.is_default ? t("settings.unset_default") : t("settings.set_default")}
                  disabled={disabled}
                  onClick={() => {
                    if (m.rowId && onMakeDefault) {
                      onMakeDefault(m.rowId, !!m.is_default);
                    } else {
                      onChange(
                        models.map((row) => ({
                          ...row,
                          is_default: row.key === m.key ? !row.is_default : false,
                        }))
                      );
                    }
                  }}
                >
                  <Star className={cn("h-3.5 w-3.5", m.is_default && "fill-primary text-primary")} />
                </button>
              )}
              {(m.rowId ? onTest : true) && (
                <button
                  type="button"
                  className="shrink-0 text-muted-foreground hover:text-primary disabled:opacity-50"
                  title={t("settings.test")}
                  disabled={disabled || testing === m.rowId || localTesting === m.key}
                  onClick={() => testRow(m)}
                >
                  <Zap
                    className={cn(
                      "h-3.5 w-3.5",
                      (testing === m.rowId || localTesting === m.key) && "animate-pulse"
                    )}
                  />
                </button>
              )}
              <button
                type="button"
                className="shrink-0 text-muted-foreground hover:text-destructive disabled:opacity-50"
                title={t("common.delete")}
                disabled={disabled}
                onClick={() => onChange(models.filter((x) => x.key !== m.key))}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
            {open && (
              <div className="mt-2 grid grid-cols-2 gap-2 border-t pt-2 sm:grid-cols-4">
                <CapacityField
                  m={m}
                  field="context_length"
                  fieldText={fieldText}
                  setField={setField}
                  settleField={settleField}
                  placeholder="128K"
                  disabled={disabled}
                  t={t}
                />
                {kind === "llm" && (
                  <>
                    <CapacityField
                      m={m}
                      field="max_tokens"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="2048"
                      disabled={disabled}
                      t={t}
                    />
                    <CapacityField
                      m={m}
                      field="input_price"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="0"
                      disabled={disabled}
                      t={t}
                    />
                    <CapacityField
                      m={m}
                      field="output_price"
                      fieldText={fieldText}
                      setField={setField}
                      settleField={settleField}
                      placeholder="0"
                      disabled={disabled}
                      t={t}
                    />
                  </>
                )}
              </div>
            )}
          </div>
        );
      })}

      <Button
        variant="ghost"
        size="sm"
        className="text-xs text-muted-foreground"
        disabled={disabled}
        onClick={() => onChange([...models, { key: nextKey(), model: "", name: "" }])}
      >
        <Plus className="h-3.5 w-3.5 mr-1" />
        {t("settings.provider_add_model")}
      </Button>

      {candidates !== null && (
        <Dialog
          onClose={() => {
            setCandidates(null);
            setPicked(new Set());
          }}
          title={t("settings.fetch_dialog_title")}
          description={t("settings.fetch_dialog_desc")}
          footer={
            <>
              <Button
                variant="outline"
                onClick={() => {
                  setCandidates(null);
                  setPicked(new Set());
                }}
              >
                {t("common.cancel")}
              </Button>
              <Button onClick={adoptPicked} disabled={picked.size === 0}>
                {t("settings.fetch_adopt")}
              </Button>
            </>
          }
        >
          <ul className="space-y-1">
            {candidates.map((c) => {
              const key = `${c.id}`;
              const checked = picked.has(key);
              return (
                <li key={c.id}>
                  <label className="flex cursor-pointer items-center justify-between rounded-md border border-input px-3 py-2 text-sm hover:bg-muted">
                    <span className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() =>
                          setPicked((s) => {
                            const n = new Set(s);
                            if (!n.delete(key)) n.add(key);
                            return n;
                          })
                        }
                      />
                      <span className="font-mono text-xs">{c.id}</span>
                      {c.note === "free" && (
                        <span className="rounded bg-emerald-500/10 px-1.5 py-px text-[10px] text-emerald-600">
                          {t("settings.free_badge")}
                        </span>
                      )}
                    </span>
                    {!!c.context_length && (
                      <span className="text-xs text-muted-foreground">
                        {Math.floor(c.context_length / 1024)}K ctx
                      </span>
                    )}
                  </label>
                </li>
              );
            })}
          </ul>
        </Dialog>
      )}
    </div>
  );
}
