"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Save } from "lucide-react";
import ReactFlow, {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  type Edge,
  type Node,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
} from "reactflow";
import useSWR from "swr";
import "reactflow/dist/style.css";

import { api, Pipeline, PipelineDefinition, PipelineEdge, PipelineNode } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { pipelineNodeTypes } from "@/components/pipeline/pipeline-nodes";
import { PipelineConfigPanel } from "@/components/pipeline/pipeline-config-panel";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

// Converts the API graph shape (PipelineNode/Edge) into React Flow's Node/Edge
// so the canvas can render and edit it. Node.data is preserved verbatim so the
// config panel can read/write typed fields without translation.
function toFlow(def: PipelineDefinition): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = def.nodes.map((n: PipelineNode) => ({
    id: n.id,
    type: n.type,
    position: n.position,
    data: n.data,
  }));
  const edges: Edge[] = def.edges.map((e: PipelineEdge) => ({
    id: e.id,
    source: e.source,
    target: e.target,
  }));
  return { nodes, edges };
}

// Strips React Flow internal fields (selected, width, height, …) so the saved
// payload matches what the backend Validate() expects.
function fromFlow(nodes: Node[], edges: Edge[]): PipelineDefinition {
  return {
    nodes: nodes.map((n) => ({
      id: n.id,
      type: n.type || "source",
      position: n.position,
      data: n.data as Record<string, unknown>,
    })),
    edges: edges.map((e) => ({ id: e.id, source: e.source, target: e.target })),
  };
}

export default function PipelinePage() {
  return (
    <ReactFlowProvider>
      <PipelineCanvas />
    </ReactFlowProvider>
  );
}

function PipelineCanvas() {
  const params = useParams<{ id: string }>();
  const kbId = params.id;
  const t = useTranslations();

  const { data: pipeline, mutate } = useSWR<Pipeline>(
    `pipeline-${kbId}`,
    () => api.getPipeline(kbId)
  );

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);

  // Seed the canvas once when the pipeline definition arrives.
  useEffect(() => {
    if (!pipeline || loaded) return;
    const { nodes: fn, edges: fe } = toFlow(pipeline.definition);
    setNodes(fn);
    setEdges(fe);
    setLoaded(true);
  }, [pipeline, loaded, setNodes, setEdges]);

  const selectedNode = nodes.find((n) => n.id === selectedId) || null;

  const onNodeDataChange = useCallback(
    (id: string, data: Record<string, unknown>) => {
      setNodes((nds) =>
        nds.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...data } } : n))
      );
    },
    [setNodes]
  );

  async function save() {
    setSaving(true);
    try {
      await api.savePipeline(kbId, fromFlow(nodes, edges));
      mutate();
      toast.success(t("toast.saved"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <Link
        href={`/dashboard/knowledge-bases/${kbId}`}
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4"
      >
        <ArrowLeft className="h-4 w-4 mr-1" /> {t("kb.back_kb")}
      </Link>

      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("pipeline.title")}</h1>
          {pipeline && (
            <p className="text-sm text-muted-foreground mt-1">
              {t("pipeline.version")} {pipeline.version} · {t("pipeline.saved_label")} {new Date(pipeline.updated_at).toLocaleString()}
            </p>
          )}
        </div>
        <Button onClick={save} disabled={saving || !loaded}>
          <Save className="h-4 w-4 mr-1" />
          {saving ? t("pipeline.saving") : t("pipeline.save_button")}
        </Button>
      </div>

      <div className="flex gap-4">
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card>
            <CardContent className="p-0" style={{ overflow: "hidden" }}>
              <div style={{ width: "100%", height: 560, position: "relative" }}>
                <ReactFlow
                  nodes={nodes}
                  edges={edges}
                  onNodesChange={onNodesChange}
                  onEdgesChange={onEdgesChange}
                  onNodeClick={(_, n) => setSelectedId(n.id)}
                  onPaneClick={() => setSelectedId(null)}
                  nodeTypes={pipelineNodeTypes}
                  nodesDraggable
                  nodesConnectable={false}
                  defaultEdgeOptions={{ markerEnd: { type: MarkerType.ArrowClosed } }}
                  fitView
                  proOptions={{ hideAttribution: true }}
                >
                  <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
                  <Controls showInteractive={false} />
                </ReactFlow>
              </div>
            </CardContent>
          </Card>
        </div>

        <div style={{ width: 280, flexShrink: 0 }}>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">{t("pipeline.config")}</CardTitle>
            </CardHeader>
            <CardContent>
              <PipelineConfigPanel node={selectedNode as PipelineNode | null} onChange={onNodeDataChange} />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
