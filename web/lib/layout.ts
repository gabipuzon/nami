import dagre from "@dagrejs/dagre";
import type { VisibleGraph } from "./presentation.ts";

export interface PositionedNode {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

export function layoutVisibleGraph(graph: VisibleGraph, cardHeights: ReadonlyMap<string, number> = new Map()): PositionedNode[] {
  const layout = new dagre.graphlib.Graph();
  layout.setGraph({ rankdir: "LR", nodesep: 18, ranksep: 52, marginx: 48, marginy: 48 });
  layout.setDefaultEdgeLabel(() => ({}));
  for (const node of graph.nodes) {
    const width = 278;
    const height = cardHeights.get(node.id) ?? 72;
    layout.setNode(node.id, { width, height });
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
