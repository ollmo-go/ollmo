"use client";

import { PipelineNode } from "@/lib/api";

// PipelineConfigPanel renders an inline editor for the selected node's data.
// Each node type has its own small form; the parent applies changes back to
// the graph state.
export function PipelineConfigPanel({
  node,
  onChange,
}: {
  node: PipelineNode | null;
  onChange: (id: string, data: Record<string, unknown>) => void;
}) {
  if (!node) {
    return (
      <div className="text-sm text-muted-foreground p-4">
        Select a node to edit its configuration.
      </div>
    );
  }
  return (
    <div className="p-4 space-y-3">
      <div className="text-sm font-medium capitalize">{node.type} node</div>
      {renderEditor(node, (patch) => onChange(node.id, { ...node.data, ...patch }))}
    </div>
  );
}

function renderEditor(
  node: PipelineNode,
  patch: (p: Record<string, unknown>) => void
) {
  switch (node.type) {
    case "source":
      return (
        <Field label="Label">
          <TextInput value={String(node.data.label || "")} onChange={(v) => patch({ label: v })} />
        </Field>
      );
    case "parser":
      return (
        <>
          <Field label="Engine">
            <SelectInput
              value={String(node.data.engine || "auto")}
              options={["auto", "mineru", "text"]}
              onChange={(v) => patch({ engine: v })}
            />
          </Field>
          <Toggle label="OCR" checked={!!node.data.ocr} onChange={(v) => patch({ ocr: v })} />
          <Toggle label="Formula" checked={!!node.data.formula} onChange={(v) => patch({ formula: v })} />
          <Toggle label="Table" checked={!!node.data.table} onChange={(v) => patch({ table: v })} />
        </>
      );
    case "chunker":
      return (
        <>
          <Field label="Strategy">
            <SelectInput
              value={String(node.data.strategy || "paragraph")}
              options={["paragraph", "token", "parent_child", "header"]}
              onChange={(v) => patch({ strategy: v })}
            />
          </Field>
          <Field label="Chunk size">
            <NumberInput value={Number(node.data.size || 500)} onChange={(v) => patch({ size: v })} />
          </Field>
          <Field label="Overlap">
            <NumberInput value={Number(node.data.overlap || 0)} onChange={(v) => patch({ overlap: v })} />
          </Field>
        </>
      );
    case "embedder":
      return (
        <>
          <Field label="Model">
            <div className="px-2 py-1 text-sm text-muted-foreground rounded-md border border-border bg-muted/50">
              {String(node.data.model || "")}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              Pinned at the knowledge base level (determines vector dimension).
            </p>
          </Field>
          <Field label="Batch size">
            <NumberInput
              value={Number(node.data.batch_size || 32)}
              onChange={(v) => patch({ batch_size: v })}
            />
          </Field>
        </>
      );
    case "sink":
      return (
        <Field label="Type">
          <TextInput value={String(node.data.type || "milvus")} onChange={(v) => patch({ type: v })} />
        </Field>
      );
    default:
      return null;
  }
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

function NumberInput({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  return (
    <input
      type="number"
      value={value}
      min={0}
      onChange={(e) => onChange(Number(e.target.value) || 0)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    />
  );
}

function SelectInput({
  value,
  options,
  onChange,
}: {
  value: string;
  options: string[];
  onChange: (v: string) => void;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-md border border-border bg-background px-2 py-1 text-sm"
    >
      {options.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );
}

function Toggle({
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
