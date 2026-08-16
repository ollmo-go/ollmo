"use client";

import { useState } from "react";
import useSWR from "swr";
import { Ban, Check, Copy, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { api, APIKey, APIKeyWithSecret } from "@/lib/api";
import { useTranslations } from "next-intl";
import { useConfirm } from "@/components/ui/confirm";
import { formatTime } from "@/lib/utils";
import { STATUS_BADGE } from "@/lib/ui-colors";

export default function ApiKeysPage() {
  const t = useTranslations();
  const confirm = useConfirm();
  const { data, mutate } = useSWR<APIKey[]>("api-keys", () => api.listApiKeys());

  // Create dialog has two phases: "form" collects input, "result" shows the
  // full key once after creation (it is never retrievable again).
  const [dialogOpen, setDialogOpen] = useState(false);
  const [phase, setPhase] = useState<"form" | "result">("form");
  const [name, setName] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [creating, setCreating] = useState(false);
  const [createdKey, setCreatedKey] = useState<APIKeyWithSecret | null>(null);
  const [copied, setCopied] = useState(false);

  function openCreate() {
    setPhase("form");
    setName("");
    setExpiresAt("");
    setCreatedKey(null);
    setCopied(false);
    setDialogOpen(true);
  }

  async function create() {
    if (!name.trim()) {
      toast.error(t("settings.apiKeys.name_required"));
      return;
    }
    setCreating(true);
    try {
      // Send empty string as undefined so the backend treats it as no expiry.
      const key = await api.createApiKey(
        name.trim(),
        expiresAt ? new Date(expiresAt).toISOString() : undefined
      );
      setCreatedKey(key);
      setPhase("result");
      mutate();
      toast.success(t("settings.apiKeys.created"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setCreating(false);
    }
  }

  function closeDialog() {
    setDialogOpen(false);
    setCreatedKey(null);
  }

  function copyKey() {
    if (!createdKey) return;
    navigator.clipboard.writeText(createdKey.full_key);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  async function revoke(k: APIKey) {
    const ok = await confirm({
      title: t("settings.apiKeys.revoke_confirm"),
      destructive: true,
      confirmText: t("settings.apiKeys.revoke"),
    });
    if (!ok) return;
    try {
      await api.revokeApiKey(k.id);
      mutate();
      toast.success(t("settings.apiKeys.revoked_toast"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function del(k: APIKey) {
    const ok = await confirm({
      title: t("settings.apiKeys.delete_confirm"),
      destructive: true,
      confirmText: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.deleteApiKey(k.id);
      mutate();
      toast.success(t("toast.deleted"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("settings.apiKeys.title")}
        </h1>
      </div>

      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold tracking-tight">{t("settings.apiKeys.title")}</h2>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" /> {t("settings.apiKeys.create")}
        </Button>
      </div>

      <Card>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-muted-foreground border-b">
                  <th className="py-2 pr-4">{t("settings.apiKeys.name")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.key_prefix")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.status")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.last_used")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.created_at")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.expires")}</th>
                  <th className="py-2 pr-4">{t("settings.apiKeys.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {!data &&
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b last:border-0">
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-28" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-14" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="py-2 pr-4"><Skeleton className="h-4 w-16" /></td>
                    </tr>
                  ))}
                {data?.map((k) => (
                  <tr key={k.id} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-medium">{k.name}</td>
                    <td className="py-2 pr-4">
                      <code className="text-xs bg-muted px-1.5 py-0.5 rounded">
                        {k.key_prefix}…
                      </code>
                    </td>
                    <td className="py-2 pr-4">
                      <span
                        className={`px-2 py-0.5 rounded text-xs ${
                          k.status === "active"
                            ? STATUS_BADGE.ok
                            : "bg-muted text-muted-foreground"
                        }`}
                      >
                        {t(`settings.apiKeys.${k.status}`)}
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">
                      {k.last_used_at ? formatTime(k.last_used_at) : t("settings.apiKeys.never")}
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">
                      {formatTime(k.created_at)}
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">
                      {k.expires_at ? formatTime(k.expires_at) : t("settings.apiKeys.no_expiry")}
                    </td>
                    <td className="py-2 pr-4">
                      <div className="flex gap-1">
                        {k.status === "active" && (
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => revoke(k)}
                            title={t("settings.apiKeys.revoke")}
                            aria-label={t("settings.apiKeys.revoke")}
                          >
                            <Ban className="h-4 w-4" />
                          </Button>
                        )}
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => del(k)}
                          title={t("settings.apiKeys.delete")}
                          aria-label={t("settings.apiKeys.delete")}
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
          {data && data.length === 0 && (
            <p className="text-sm text-muted-foreground py-4">
              {t("settings.apiKeys.empty")}
            </p>
          )}
        </CardContent>
      </Card>

      {dialogOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/40"
            onClick={closeDialog}
            aria-hidden="true"
          />
          <div className="relative w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
            {phase === "form" ? (
              <>
                <h3 className="text-lg font-semibold mb-4">
                  {t("settings.apiKeys.create_title")}
                </h3>
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label>{t("settings.apiKeys.name")}</Label>
                    <Input
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder={t("settings.apiKeys.name_placeholder")}
                      autoFocus
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>{t("settings.apiKeys.expires_at")}</Label>
                    <Input
                      type="date"
                      value={expiresAt}
                      onChange={(e) => setExpiresAt(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      {t("settings.apiKeys.expires_at_hint")}
                    </p>
                  </div>
                </div>
                <div className="mt-6 flex justify-end gap-2">
                  <Button variant="outline" onClick={closeDialog}>
                    {t("common.cancel")}
                  </Button>
                  <Button onClick={create} disabled={creating}>
                    {creating ? "..." : t("settings.apiKeys.create_button")}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <h3 className="text-lg font-semibold mb-2">
                  {t("settings.apiKeys.key_created")}
                </h3>
                <p className="text-sm text-amber-700 bg-amber-50 border border-amber-200 rounded-md px-3 py-2 mb-4">
                  {t("settings.apiKeys.key_warning")}
                </p>
                <div className="space-y-1">
                  <Label>{t("settings.apiKeys.full_key")}</Label>
                  <div className="flex gap-2">
                    <Input
                      readOnly
                      value={createdKey?.full_key ?? ""}
                      className="font-mono text-xs"
                      onFocus={(e) => e.target.select()}
                    />
                    <Button variant="outline" onClick={copyKey} className="shrink-0">
                      {copied ? (
                        <><Check className="h-4 w-4 mr-1" /> {t("settings.apiKeys.copied")}</>
                      ) : (
                        <><Copy className="h-4 w-4 mr-1" /> {t("settings.apiKeys.copy")}</>
                      )}
                    </Button>
                  </div>
                </div>
                <div className="mt-6 flex justify-end">
                  <Button onClick={closeDialog}>{t("settings.apiKeys.done")}</Button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

