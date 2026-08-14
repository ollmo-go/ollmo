"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useTranslations } from "next-intl";

interface ConfirmOptions {
  title: string;
  description?: string;
  confirmText?: string;
  cancelText?: string;
  destructive?: boolean;
}

interface PromptOptions {
  title: string;
  description?: string;
  defaultValue?: string;
  confirmText?: string;
  cancelText?: string;
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;
type PromptFn = (opts: PromptOptions) => Promise<string | null>;

const ConfirmContext = createContext<ConfirmFn | null>(null);
const PromptContext = createContext<PromptFn | null>(null);

export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext);
  if (!fn) throw new Error("useConfirm must be used within ConfirmProvider");
  return fn;
}

export function usePrompt(): PromptFn {
  const fn = useContext(PromptContext);
  if (!fn) throw new Error("usePrompt must be used within ConfirmProvider");
  return fn;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const t = useTranslations();
  const [open, setOpen] = useState(false);
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolverRef = useRef<((v: boolean) => void) | null>(null);

  // Prompt state
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptOpts, setPromptOpts] = useState<PromptOptions | null>(null);
  const [promptValue, setPromptValue] = useState("");
  const promptResolverRef = useRef<((v: string | null) => void) | null>(null);
  const promptInputRef = useRef<HTMLInputElement>(null);

  const confirm = useCallback<ConfirmFn>((opts) => {
    setOptions(opts);
    setOpen(true);
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, []);

  function close(value: boolean) {
    setOpen(false);
    resolverRef.current?.(value);
    resolverRef.current = null;
  }

  const prompt = useCallback<PromptFn>((opts) => {
    setPromptOpts(opts);
    setPromptValue(opts.defaultValue || "");
    setPromptOpen(true);
    return new Promise<string | null>((resolve) => {
      promptResolverRef.current = resolve;
    });
  }, []);

  function closePrompt(value: string | null) {
    setPromptOpen(false);
    promptResolverRef.current?.(value);
    promptResolverRef.current = null;
  }

  useEffect(() => {
    if (promptOpen) {
      requestAnimationFrame(() => promptInputRef.current?.focus());
    }
  }, [promptOpen]);

  return (
    <ConfirmContext.Provider value={confirm}>
      <PromptContext.Provider value={prompt}>
        {children}
        {open && options && (
          <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div
              className="absolute inset-0 bg-black/40"
              onClick={() => close(false)}
              aria-hidden="true"
            />
            <div className="relative w-full max-w-sm rounded-lg border bg-background p-6 shadow-lg">
              <h3 className="text-lg font-semibold">{options.title}</h3>
              {options.description && (
                <p className="mt-2 text-sm text-muted-foreground">
                  {options.description}
                </p>
              )}
              <div className="mt-6 flex justify-end gap-2">
                <Button variant="outline" onClick={() => close(false)}>
                  {options.cancelText || t("common.cancel")}
                </Button>
                <Button
                  variant={options.destructive ? "destructive" : "default"}
                  onClick={() => close(true)}
                >
                  {options.confirmText || t("common.confirm")}
                </Button>
              </div>
            </div>
          </div>
        )}
        {promptOpen && promptOpts && (
          <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div
              className="absolute inset-0 bg-black/40"
              onClick={() => closePrompt(null)}
              aria-hidden="true"
            />
            <div className="relative w-full max-w-sm rounded-lg border bg-background p-6 shadow-lg">
              <h3 className="text-lg font-semibold">{promptOpts.title}</h3>
              {promptOpts.description && (
                <p className="mt-2 text-sm text-muted-foreground">
                  {promptOpts.description}
                </p>
              )}
              <Input
                ref={promptInputRef}
                className="mt-4"
                value={promptValue}
                onChange={(e) => setPromptValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") closePrompt(promptValue);
                  if (e.key === "Escape") closePrompt(null);
                }}
              />
              <div className="mt-6 flex justify-end gap-2">
                <Button variant="outline" onClick={() => closePrompt(null)}>
                  {promptOpts.cancelText || t("common.cancel")}
                </Button>
                <Button onClick={() => closePrompt(promptValue)}>
                  {promptOpts.confirmText || t("common.confirm")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </PromptContext.Provider>
    </ConfirmContext.Provider>
  );
}
