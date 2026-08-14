"use client";

import { memo } from "react";
import { Handle, Position } from "reactflow";
import { Database, FileInput, Scissors, Sparkles, Layers } from "lucide-react";

interface NodeData {
  [key: string]: unknown;
}

function nodeShell(label: string, icon: React.ReactNode, subtitle: string, selected: boolean) {
  return (
    <div
      className={`rounded-lg border bg-card px-4 py-3 shadow-sm w-52 transition-colors ${
        selected ? "border-primary ring-2 ring-primary/30" : "border-border"
      }`}
    >
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !bg-muted-foreground" />
      <div className="flex items-center gap-2 mb-1">
        <span className="text-primary">{icon}</span>
        <span className="font-medium text-sm">{label}</span>
      </div>
      <div className="text-xs text-muted-foreground truncate">{subtitle}</div>
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !bg-muted-foreground" />
    </div>
  );
}

export const SourceNode = memo(({ data, selected }: { data: NodeData; selected?: boolean }) =>
  nodeShell("File Source", <FileInput className="h-4 w-4" />, "Upload from MinIO", !!selected)
);

export const ParserNode = memo(({ data, selected }: { data: NodeData; selected?: boolean }) => {
  const ocr = data.ocr ? "OCR" : null;
  const table = data.table ? "Table" : null;
  const formula = data.formula ? "Formula" : null;
  const feats = [ocr, table, formula].filter(Boolean).join(" · ") || "no extras";
  return nodeShell(
    "Parser",
    <Layers className="h-4 w-4" />,
    `${String(data.engine || "auto")} · ${feats}`,
    !!selected
  );
});

export const ChunkerNode = memo(({ data, selected }: { data: NodeData; selected?: boolean }) => {
  const sub = `${data.strategy || "paragraph"} · ${data.size || 500}/${data.overlap || 0}`;
  return nodeShell("Chunker", <Scissors className="h-4 w-4" />, sub, !!selected);
});

export const EmbedderNode = memo(({ data, selected }: { data: NodeData; selected?: boolean }) =>
  nodeShell(
    "Embedder",
    <Sparkles className="h-4 w-4" />,
    `${String(data.model || "bge-large-zh-v1.5")} · batch ${Number(data.batch_size || 32)}`,
    !!selected
  )
);

export const SinkNode = memo(({ data, selected }: { data: NodeData; selected?: boolean }) =>
  nodeShell("Vector Store", <Database className="h-4 w-4" />, String(data.type || "milvus"), !!selected)
);

// Maps the backend node type to a React Flow node component. Registered in the
// canvas page via nodeTypes.
export const pipelineNodeTypes = {
  source: SourceNode,
  parser: ParserNode,
  chunker: ChunkerNode,
  embedder: EmbedderNode,
  sink: SinkNode,
};
