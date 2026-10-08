import { declarationKinds, type Graph, type GraphNode } from "./types.ts";

export interface SourceStats {
  imports?: number;
  exports?: number;
}

export interface NodePresentation {
  displayKind: string;
  displayName: string;
  rowName: string;
  cardName: string;
  secondaryLabel?: string;
  childLabel: { singular: string; plural: string };
  rowGroupLabel: string;
  expansion?: "container" | "source";
  primaryCard: boolean;
  explorerSection?: "packages" | "modules";
  rowMark?: string;
  sourceStats: SourceStats;
}

const goDeclarations = new Set<string>(declarationKinds.filter((kind) => kind !== "CLASS"));
const pythonDeclarations = new Set(["CLASS", "FUNCTION", "METHOD"]);

// Missing language is a legacy snapshot: retain its canonical labels and
// package cards, but never infer Python semantics from a .py suffix.
export function presentationFor(node: GraphNode, parent?: GraphNode, counts?: SourceStats): NodePresentation {
  const legacy = node.language === undefined || node.language === "";
  const go = node.language === "go";
  const python = node.language === "python";
  const supported = (go || legacy) && (node.kind === "MODULE" || node.kind === "PACKAGE" || node.kind === "FILE" || goDeclarations.has(node.kind)) ||
    python && (node.kind === "PACKAGE" || node.kind === "FILE" || pythonDeclarations.has(node.kind));
  const container = supported && node.kind === "PACKAGE";
  const source = supported && node.kind === "FILE";
  const pythonModule = python && source;
  const stem = node.name.replace(/\.py$/, "");
  const moduleName = parent?.language === "python" && parent.kind === "PACKAGE" ?
    node.name === "__init__.py" ? parent.name : `${parent.name}.${stem}` : stem;
  const displayName = pythonModule ? moduleName : node.name;
  return {
    displayKind: pythonModule ? "module" : node.kind.toLowerCase(),
    displayName,
    rowName: pythonModule ? stem : container ? python ? node.name.split(".").at(-1)! : node.path : node.name,
    cardName: pythonModule ? moduleName : container ? python ? node.name : node.path : node.name,
    secondaryLabel: container ? `package ${node.name}` : pythonModule ? "module" : undefined,
    childLabel: container ? python ? { singular: "module", plural: "modules" } : { singular: "file", plural: "files" } :
      source ? { singular: "declaration", plural: "declarations" } : { singular: "package", plural: "packages" },
    rowGroupLabel: container ? python ? "Modules" : "Files" : pythonModule ? "Module" : "Files",
    expansion: container ? "container" : source ? "source" : undefined,
    primaryCard: container || pythonModule && !parent,
    explorerSection: container ? "packages" : pythonModule && !parent ? "modules" : undefined,
    rowMark: source ? pythonModule ? "M" : "F" : container ? undefined : "·",
    sourceStats: { imports: source ? node.import_count : counts?.imports, exports: python ? undefined : source ? node.export_count : counts?.exports },
  };
}

export function sourceStatsTitle(stats: SourceStats): string {
  return `${stats.imports ?? "unknown"} imports, ${stats.exports ?? "unknown"} exports`;
}

export function buildNodePresentations(graph: Graph, counts: ReadonlyMap<string, SourceStats> = new Map()): Map<string, NodePresentation> {
  const nodes = new Map(graph.nodes.map((node) => [node.id, node]));
  const parent = new Map(graph.edges.filter((edge) => edge.kind === "CONTAINS").map((edge) => [edge.to, nodes.get(edge.from)]));
  return new Map(graph.nodes.map((node) => [node.id, presentationFor(node, parent.get(node.id), counts.get(node.id))]));
}
