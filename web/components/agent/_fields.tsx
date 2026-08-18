"use client";

import type React from "react";

export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

export function TextArea({
  value,
  onChange,
  rows = 4,
  textareaRef,
}: {
  value: string;
  onChange: (v: string) => void;
  rows?: number;
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>;
}) {
  return (
    <textarea
      ref={textareaRef}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      rows={rows}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm resize-y"
    />
  );
}

export function NumberInput({
  value,
  onChange,
  step,
}: {
  value: number;
  onChange: (v: number) => void;
  step?: number;
}) {
  return (
    <input
      type="number"
      value={value}
      step={step || 1}
      onChange={(e) => onChange(Number(e.target.value) || 0)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

export function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between cursor-pointer">
      <span className="text-xs text-muted-foreground">{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4"
      />
    </label>
  );
}
