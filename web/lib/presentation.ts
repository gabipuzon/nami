import { declarationKinds, type DeclarationKind, type Graph, type GraphEdge, type GraphNode, type PackageProjection } from "./types.ts";

export interface VisibleNode extends GraphNode {
  parentId?: string;
  childCount: number;
  expanded: boolean;
}

export interface VisibleEdge {
  id: string;
  kind: GraphEdge["kind"];
  source: string;
  target: string;
  projectionEdge?: GraphEdge;
  evidence: GraphEdge[];
}

export interface VisibleGraph {
  nodes: VisibleNode[];
  edges: VisibleEdge[];
}

export interface PresentationInput {
  canonicalGraph: Graph;
  packageProjection: PackageProjection;
  expandedPackages: ReadonlySet<string>;
  expandedFiles: ReadonlySet<string>;
  visibleDeclarationKinds: ReadonlySet<DeclarationKind>;
}

export interface RevealState {
  expandedPackages: Set<string>;
  expandedFiles: Set<string>;
  visibleDeclarationKinds: Set<DeclarationKind>;
}

const compare = (a: string, b: string): number => a < b ? -1 : a > b ? 1 : 0;
const edgeKey = (edge: GraphEdge): string => `${edge.kind}:${edge.from}->${edge.to}`;

export function buildVisibleGraph(input: PresentationInput): VisibleGraph {
  const { canonicalGraph, packageProjection, expandedPackages, expandedFiles, visibleDeclarationKinds } = input;
  const canonicalNodes = new Map(canonicalGraph.nodes.map((node) => [node.id, node]));
  const children = new Map<string, GraphNode[]>();
  const canonicalImports = new Set(canonicalGraph.edges.filter((edge) => edge.kind === "IMPORTS").map(edgeKey));
  for (const edge of canonicalGraph.edges) {
    if (edge.kind !== "CONTAINS") continue;
    const child = canonicalNodes.get(edge.to);
    if (child) children.set(edge.from, [...(children.get(edge.from) ?? []), child]);
  }
  for (const group of children.values()) group.sort((a, b) => compare(a.id, b.id));

  const nodes: VisibleNode[] = [];
  const edges: VisibleEdge[] = [];
  const visibleIDs = new Set<string>();
  const addNode = (node: GraphNode, parentId?: string) => {
    if (visibleIDs.has(node.id)) return;
    visibleIDs.add(node.id);
    nodes.push({
      ...node,
      parentId,
      childCount: (children.get(node.id) ?? []).length,
      expanded: node.kind === "PACKAGE" ? expandedPackages.has(node.id) :
        node.kind === "FILE" ? expandedFiles.has(node.id) : false,
    });
    if (parentId) {
      edges.push({ id: `CONTAINS:${parentId}->${node.id}`, kind: "CONTAINS", source: parentId, target: node.id, evidence: [] });
    }
  };

  for (const pkg of [...packageProjection.graph.nodes].sort((a, b) => compare(a.id, b.id))) {
    addNode(pkg);
    if (!expandedPackages.has(pkg.id)) continue;
    for (const file of children.get(pkg.id) ?? []) {
      if (file.kind !== "FILE") continue;
      addNode(file, pkg.id);
      if (!expandedFiles.has(file.id)) continue;
      for (const declaration of children.get(file.id) ?? []) {
        if (isDeclarationKind(declaration.kind) && visibleDeclarationKinds.has(declaration.kind)) {
          addNode(declaration, file.id);
        }
      }
    }
  }

  const evidenceByEdge = new Map(packageProjection.evidence.map((item) => [edgeKey(item.edge), item.sources]));
  for (const edge of [...packageProjection.graph.edges].sort((a, b) => compare(edgeKey(a), edgeKey(b)))) {
    if (edge.kind !== "IMPORTS" || !visibleIDs.has(edge.from) || !visibleIDs.has(edge.to)) continue;
    const evidence = evidenceByEdge.get(edgeKey(edge)) ?? [];
    if (!expandedPackages.has(edge.from)) {
      edges.push({ id: edgeKey(edge), kind: "IMPORTS", source: edge.from, target: edge.to, projectionEdge: edge, evidence });
      continue;
    }
    for (const source of [...evidence].sort((a, b) => compare(edgeKey(a), edgeKey(b)))) {
      if (source.kind !== "IMPORTS" || !visibleIDs.has(source.from) || source.to !== edge.to || !canonicalImports.has(edgeKey(source))) continue;
      edges.push({ id: edgeKey(source), kind: "IMPORTS", source: source.from, target: source.to, projectionEdge: edge, evidence: [source] });
    }
  }

  nodes.sort((a, b) => compare(a.id, b.id));
  edges.sort((a, b) => compare(a.id, b.id));
  return { nodes, edges };
}

export function isDeclarationKind(kind: GraphNode["kind"]): kind is DeclarationKind {
  return (declarationKinds as readonly string[]).includes(kind);
}

export function revealNode(graph: Graph, id: string, state: RevealState): RevealState {
  const nodes = new Map(graph.nodes.map((node) => [node.id, node]));
  const parent = new Map(graph.edges.filter((edge) => edge.kind === "CONTAINS").map((edge) => [edge.to, edge.from]));
  const expandedPackages = new Set(state.expandedPackages);
  const expandedFiles = new Set(state.expandedFiles);
  const visibleDeclarationKinds = new Set(state.visibleDeclarationKinds);
  const node = nodes.get(id);
  if (!node) return { expandedPackages, expandedFiles, visibleDeclarationKinds };
  if (isDeclarationKind(node.kind)) visibleDeclarationKinds.add(node.kind);
  let ancestor = parent.get(id);
  while (ancestor) {
    const kind = nodes.get(ancestor)?.kind;
    if (kind === "PACKAGE") expandedPackages.add(ancestor);
    if (kind === "FILE") expandedFiles.add(ancestor);
    ancestor = parent.get(ancestor);
  }
  return { expandedPackages, expandedFiles, visibleDeclarationKinds };
}

export function searchNodes(graph: Graph, query: string): GraphNode[] {
  const term = query.trim().toLocaleLowerCase();
  if (!term) return [];
  return graph.nodes
    .filter((node) => node.kind !== "MODULE" && [node.name, node.path, node.id].some((field) => field.toLocaleLowerCase().includes(term)))
    .sort((a, b) => compare(a.name, b.name) || compare(a.id, b.id))
    .slice(0, 40);
}
