import { measurePerformance } from "./performance.ts";
import { declarationKinds, type Graph, type GraphEdge, type GraphNode } from "./types.ts";

export const factKey = (edge: GraphEdge): string => `${edge.kind}:${edge.from}->${edge.to}`;
const compare = (a: string, b: string): number => a < b ? -1 : a > b ? 1 : 0;
export interface GraphIndex {
  nodes: ReadonlyMap<string, GraphNode>;
  parent: ReadonlyMap<string, string>;
  children: ReadonlyMap<string, readonly GraphNode[]>;
  imports: readonly GraphEdge[];
  importKeys: ReadonlySet<string>;
  outgoing: ReadonlyMap<string, readonly string[]>;
  incoming: ReadonlyMap<string, readonly string[]>;
  counts: { packages: number; files: number; declarations: number };
}

// Loaded snapshots are immutable. A replacement snapshot gets its own index;
// the weak key allows the old snapshot and all its indexes to be collected.
const indexes = new WeakMap<Graph, GraphIndex>();
export function getGraphIndex(graph: Graph): GraphIndex {
  const existing = indexes.get(graph);
  if (existing) return existing;
  const index = measurePerformance("buildGraphIndex", () => {
    const nodes = new Map(graph.nodes.map(node => [node.id, node]));
    const parent = new Map<string, string>();
    const children = new Map<string, GraphNode[]>();
    const imports: GraphEdge[] = [];
    const outgoing = new Map<string, string[]>();
    const incoming = new Map<string, string[]>();
    const append = <T>(map: Map<string, T[]>, id: string, value: T) => {
      const group = map.get(id);
      if (group) group.push(value);
      else map.set(id, [value]);
    };
    for (const edge of graph.edges) {
      if (edge.kind === "CONTAINS") {
        parent.set(edge.to, edge.from);
        const child = nodes.get(edge.to);
        if (child) append(children, edge.from, child);
      }
      if (edge.kind === "IMPORTS") {
        imports.push(edge);
        append(outgoing, edge.from, edge.to);
        append(incoming, edge.to, edge.from);
      }
    }
    for (const group of children.values()) group.sort((a, b) => compare(a.id, b.id));
    for (const group of [...outgoing.values(), ...incoming.values()]) group.sort(compare);
    const counts = { packages: 0, files: 0, declarations: 0 };
    for (const node of graph.nodes) {
      if (node.kind === "PACKAGE") counts.packages++;
      if (node.kind === "FILE") counts.files++;
      if ((declarationKinds as readonly string[]).includes(node.kind)) counts.declarations++;
    }
    return { nodes, parent, children, imports, importKeys: new Set(imports.map(factKey)), outgoing, incoming, counts };
  });
  indexes.set(graph, index);
  return index;
}
