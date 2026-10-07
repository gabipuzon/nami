import dagre from "@dagrejs/dagre";
import type { VisibleGraph } from "./presentation.ts";

export interface PositionedNode {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

export function layoutVisibleGraph(graph: VisibleGraph): PositionedNode[] {
  const layout = new dagre.graphlib.Graph();
  layout.setGraph({ rankdir: "LR", nodesep: 36, ranksep: 42, marginx: 32, marginy: 32 });
  layout.setDefaultEdgeLabel(() => ({}));
  for (const node of graph.nodes) {
    layout.setNode(node.id, { width: 304, height: 82 });
  }
  for (const edge of graph.edges) layout.setEdge(edge.source, edge.target);
  dagre.layout(layout);
  return graph.nodes.map((node) => {
    const point = layout.node(node.id);
    return {
      id: node.id,
      x: point.x - point.width / 2,
      y: point.y - point.height / 2,
      width: point.width,
      height: point.height,
    };
  });
}
