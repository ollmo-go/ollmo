"use client";

import { useState } from "react";
import useSWR from "swr";
import { Pencil, Plus, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Drawer } from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Paginated } from "@/lib/api";
import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import { useConfirm } from "@/components/ui/confirm";

export interface BaseModel {
  id: string;
  name: string;
  endpoint: string;
  model: string;
  api_key?: string;
  is_default: boolean;
  last_tested_at: string | null;
  last_test_status: string;
}

export type FieldType = "text" | "number" | "password" | "checkbox" | "select";

export interface ModelField {
  key: string;
  labelKey: string;
  type: FieldType;
  options?: string[];
  onOptionChange?: (option: string, form: Record<string, unknown>) => Record<string, unknown>;
  parse?: (v: string) => unknown;
}

export interface ModelSectionConfig<T extends BaseModel> {
  swrKey: string;
  listFn: () => Promise<Paginated<T>>;
  createFn: (body: Partial<T>) => Promise<T>;
  updateFn: (id: string, body: Record<string, unknown>) => Promise<T>;
  deleteFn: (id: string) => Promise<void>;
  testFn: (id: string) => Promise<{ reply: string }>;

  sectionTitleKey: string;
  namespace: string;

  defaultForm: Record<string, unknown>;
  toEditForm: (p: T) => Record<string, unknown>;
  buildCreateBody: (form: Record<string, unknown>) => Partial<T>;
  buildUpdateBody: (form: Record<string, unknown>) => Record<string, unknown>;

  fields: ModelField[];
  renderSecondary: (p: T, t: (key: string) => string) => string;
  formatTestReply?: (reply: string, t: (key: string) => string) => string;
}

export function ModelSection<T extends BaseModel>({
  config,
  first = false,
}: {
  config: ModelSectionConfig<T>;
  first?: boolean;
}) {
  const { data, mutate } = useSWR<Paginated<T>>(config.swrKey, config.listFn);
  const t = useTranslations();
  const confirm = useConfirm();
  const ns = config.namespace;

  const [mode, setMode] = useState<"create" | "edit" | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, unknown>>(config.defaultForm);
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<Record<string, string>>({});

  const isCreate = mode === "create";

  function openCreate() {
    setMode("create");
    setEditId(null);
    setForm({ ...config.defaultForm });
  }

  function openEdit(p: T) {
    setMode("edit");
    setEditId(p.id);
    setForm(config.toEditForm(p));
  }

  async function save() {
    if (!form.name || !form.endpoint || !form.model) {
      toast.error(t(`${ns}.required_fields`));
      return;
    }
    setSaving(true);
    try {
      if (mode === "create") {
        await config.createFn(config.buildCreateBody(form));
      } else if (mode === "edit" && editId) {
        await config.updateFn(editId, config.buildUpdateBody(form));
      }
      setMode(null);
      mutate();
      toast.success(t("toast.saved"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function test(id: string) {
    setTestResult({ ...testResult, [id]: t(`${ns}.testing`) });
    try {
      const r = await config.testFn(id);
      const label = config.formatTestReply
        ? config.formatTestReply(r.reply, t)
        : r.reply || t(`${ns}.empty_reply`);
      setTestResult({ ...testResult, [id]: label });
    } catch (e) {
      setTestResult({ ...testResult, [id]: "error: " + (e as Error).message });
    } finally {
      mutate();
    }
  }

  async function del(id: string) {
    const ok = await confirm({
      title: t(`${ns}.delete_confirm`),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await config.deleteFn(id);
      mutate();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <>
      <div className={cn("flex items-center justify-between mb-4", !first && "mt-8")}>
        <h2 className="text-lg font-semibold tracking-tight">{t(config.sectionTitleKey)}</h2>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" /> {t(`${ns}.new`)}
        </Button>
      </div>

      <div className="space-y-3">
        {!data &&
          Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <Skeleton className="h-4 w-32 mb-2" />
                <Skeleton className="h-3 w-48" />
              </CardContent>
            </Card>
          ))}
        {data?.items?.map((p) => (
          <Card key={p.id}>
            <CardContent className="p-4 flex items-center justify-between gap-3">
              <div className="space-y-0.5 min-w-0">
                <div className="font-medium flex items-center gap-2">
                  {p.name}
                  {p.is_default && (
                    <span className="px-1.5 py-0.5 rounded text-xs bg-emerald-100 text-emerald-700">
                      {t(`${ns}.default`)}
                    </span>
                  )}
                </div>
                <div className="text-xs text-muted-foreground truncate">
                  {config.renderSecondary(p, t)}
                </div>
                {testResult[p.id] && (
                  <div className="text-xs text-muted-foreground">
                    {t(`${ns}.test`)}: {testResult[p.id]}
                  </div>
                )}
                {p.last_tested_at && (
                  <div
                    className={cn(
                      "text-xs",
                      p.last_test_status === "success" ? "text-emerald-600" : "text-red-500"
                    )}
                  >
                    {t("common.last_test")}:{" "}
                    {p.last_test_status === "success"
                      ? t("common.test_success")
                      : t("common.test_failed")}{" "}
                    · {new Date(p.last_tested_at).toLocaleString()}
                  </div>
                )}
              </div>
              <div className="flex gap-1 shrink-0">
                <Button size="sm" variant="outline" onClick={() => test(p.id)}>
                  <Zap className="h-3.5 w-3.5 mr-1" /> {t(`${ns}.test`)}
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => openEdit(p)}
                  title={t(`${ns}.edit`)}
                  aria-label={t(`${ns}.edit`)}
                >
                  <Pencil className="h-4 w-4" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => del(p.id)}
                  aria-label={t(`${ns}.delete`)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
        {data && data.items.length === 0 && (
          <p className="text-sm text-muted-foreground">{t(`${ns}.empty`)}</p>
        )}
      </div>

      {mode && (
        <Drawer
          title={isCreate ? t(`${ns}.new_title`) : t(`${ns}.edit_title`)}
          onClose={() => setMode(null)}
          onSave={save}
          saving={saving}
        >
          <div className="grid grid-cols-2 gap-4">
            {config.fields.map((f) => (
              <FieldInput
                key={f.key}
                field={f}
                form={form}
                setForm={setForm}
                isCreate={isCreate}
                t={t}
              />
            ))}
          </div>
        </Drawer>
      )}
    </>
  );
}

function FieldInput({
  field,
  form,
  setForm,
  isCreate,
  t,
}: {
  field: ModelField;
  form: Record<string, unknown>;
  setForm: (f: Record<string, unknown>) => void;
  isCreate: boolean;
  t: ReturnType<typeof useTranslations>;
}) {
  const label = t(field.labelKey);

  if (field.type === "checkbox") {
    return (
      <div className="flex items-end col-span-2">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={!!form[field.key]}
            onChange={(e) => setForm({ ...form, [field.key]: e.target.checked })}
          />
          {label}
        </label>
      </div>
    );
  }

  if (field.type === "select") {
    return (
      <div className="space-y-1">
        <Label>{label}</Label>
        <select
          className="w-full h-10 rounded-md border border-input bg-background px-3 text-sm"
          value={String(form[field.key] ?? "")}
          onChange={(e) => {
            const option = e.target.value;
            const updates = field.onOptionChange
              ? field.onOptionChange(option, form)
              : { [field.key]: option };
            setForm({ ...form, ...updates });
          }}
        >
          {field.options?.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      </div>
    );
  }

  if (field.type === "password") {
    return (
      <div className="space-y-1 col-span-2">
        <Label>{label}</Label>
        <Input
          type="password"
          value={String(form[field.key] ?? "")}
          onChange={(e) => setForm({ ...form, [field.key]: e.target.value })}
          placeholder={isCreate ? "" : t(`${field.labelKey}_hint`)}
        />
      </div>
    );
  }

  // text or number
  return (
    <div className="space-y-1">
      <Label>{label}</Label>
      <Input
        value={String(form[field.key] ?? "")}
        onChange={(e) => {
          const v = e.target.value;
          setForm({ ...form, [field.key]: field.parse ? field.parse(v) : v });
        }}
      />
    </div>
  );
}
