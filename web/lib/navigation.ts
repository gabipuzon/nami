import { getGraphIndex } from "./graphIndex.ts";
import type { Graph, GraphNode, Neighborhood } from "./types.ts";
import type { VisibleGraph } from "./presentation.ts";

export const EXPLORER_ROW_HEIGHT = 32;
export interface ExplorerPages { [id: string]: { next: number; total: number } }
export interface ExplorerPagination {
  pages: ExplorerPages;
  expanded: ReadonlySet<string>;
  pending: ReadonlySet<string>;
}
type ExplorerAction =
  | { type: "start" | "settled" | "collapse" | "expand"; id: string }
  | { type: "loaded"; id: string; next: number; total: number };

export function explorerPagination(state: ExplorerPagination, action: ExplorerAction): ExplorerPagination {
  const pending = new Set(state.pending);
  switch (action.type) {
    case "start":
      pending.add(action.id);
      return { ...state, pending };
    case "settled":
      pending.delete(action.id);
      return { ...state, pending };
    case "collapse": {
      const expanded = new Set(state.expanded);
      pending.delete(action.id);
      expanded.delete(action.id);
      return { ...state, pending, expanded };
    }
    case "expand": {
      const expanded = new Set(state.expanded);
      expanded.add(action.id);
      return { ...state, expanded };
    }
    case "loaded": {
      const expanded = new Set(state.expanded);
      pending.delete(action.id);
      expanded.add(action.id);
      return { pending, expanded, pages: { ...state.pages, [action.id]: { next: action.next, total: action.total } } };
    }
  }
}

export type ExplorerRow =
  | { kind: "node"; key: string; node: GraphNode; depth: number; expandable: boolean }
  | { kind: "more"; key: string; parent: GraphNode; depth: number; loaded: number; total: number };
export function explorerWindow(rowCount:number, scrollTop:number) {
  const first=Math.min(Math.max(0,rowCount-1),Math.max(0,Math.floor(scrollTop/EXPLORER_ROW_HEIGHT)-2));
  const last=Math.min(rowCount,first+24);
  return {first,last,before:first*EXPLORER_ROW_HEIGHT,after:Math.max(0,rowCount-last)*EXPLORER_ROW_HEIGHT};
}
export function flattenExplorer(graph: Graph, expanded: ReadonlySet<string>, kinds: ReadonlySet<string>, childCounts?: Record<string, number>, pages: ExplorerPages = {}): ExplorerRow[] {
  const { children, parent, nodes } = getGraphIndex(graph);
  const rows: ExplorerRow[] = [];
  const roots = graph.nodes.filter(n => (n.kind === "PACKAGE" && nodes.get(parent.get(n.id) ?? "")?.kind !== "PACKAGE") || (n.kind === "FILE" && !parent.has(n.id))).sort((a,b) => a.path < b.path ? -1 : a.path > b.path ? 1 : a.id < b.id ? -1 : 1);
  const visit = (node: GraphNode, depth: number) => {
    const kids = (children.get(node.id) ?? []).filter(n => n.kind === "FILE" || n.kind === "PACKAGE" || kinds.has(n.kind));
    rows.push({ kind: "node", key: `node:${node.id}`, node, depth, expandable: kids.length > 0 || (childCounts?.[node.id] ?? 0) > 0 });
    if (expanded.has(node.id)) {
      kids.forEach(n => visit(n, depth+1));
      const page=pages[node.id];
      if (page && page.next<page.total) rows.push({kind:"more",key:`more:${node.id}`,parent:node,depth:depth+1,loaded:page.next,total:page.total});
    }
  };
  roots.forEach(n => visit(n,0));
  return rows;
}

export function neighborhoodGraph(result: Neighborhood): VisibleGraph {
  return { nodes: result.graph.nodes.map(n => ({ ...n, childCount: 0, expanded: false })),
    edges: result.graph.edges.map(edge => ({ id: JSON.stringify([edge.kind, edge.from, edge.to]), kind: edge.kind, source: edge.from, target: edge.to, projectionEdge: result.scope === "package" ? edge : undefined,
      evidence: result.evidence.find(e => e.edge.from === edge.from && e.edge.to === edge.to)?.sources ?? [edge] })) };
}

export interface FocusEntry {
  result: Neighborhood;
  positions: Record<string, { x: number; y: number }>;
  viewport?: { x: number; y: number; zoom: number };
}
export function pushFocus(history: readonly FocusEntry[], current: FocusEntry | null): FocusEntry[] {
  return current ? [...history, current] : [...history];
}
export function backFocus(history: readonly FocusEntry[]): { current: FocusEntry | null; history: FocusEntry[] } {
  return { current: history.at(-1) ?? null, history: history.slice(0,-1) };
}

// Abort is best effort: a completed response still has to match the generation
// and snapshot before it is allowed to replace the last successful view.
export class RequestGeneration {
  private generation = 0;
  next(): number { return ++this.generation; }
  accepts(generation: number, snapshot: string, expected: string): boolean { return generation === this.generation && snapshot === expected; }
}
