import { measurePerformance } from "./performance.ts";
import { getGraphIndex, factKey as edgeKey } from "./graphIndex.ts";
import { buildNodePresentations, presentationFor } from "./languagePresentation.ts";
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
  supportingCount?: number;
}

export interface VisibleGraph {
  nodes: VisibleNode[];
  edges: VisibleEdge[];
}

export interface CardImportLine {
  id: string;
  canonicalEdgeID: string;
  source: string;
  sourceHandle?: string;
  target: string;
  targetHandle?: string;
  importingFileID?: string;
  supplyingFileID?: string;
}

export type ExportUseIndex = ReadonlyMap<string, ReadonlyMap<string, readonly string[]>>;

export interface PackageSourceCounts {
  imports?: number;
  exports?: number;
}

export function packageSourceCounts(graph: Graph, knownFiles?: Record<string,number>): Map<string, PackageSourceCounts> {
  return measurePerformance("packageSourceCounts", () => {
    const nodes = getGraphIndex(graph).nodes;
    const totals = new Map(graph.nodes.filter((node) => node.kind === "PACKAGE").map((node) => [node.id, { imports: 0, exports: 0 } as PackageSourceCounts]));
    for (const edge of graph.edges) {
      if (edge.kind !== "CONTAINS") continue;
      const file = nodes.get(edge.to);
      const total = totals.get(edge.from);
      if (file?.kind !== "FILE" || !total) continue;
      total.imports = total.imports === undefined || file.import_count === undefined ? undefined : total.imports + file.import_count;
      total.exports = total.exports === undefined || file.export_count === undefined ? undefined : total.exports + file.export_count;
    }
    if (knownFiles) for (const [id,total] of totals) {
      const loaded = (getGraphIndex(graph).children.get(id) ?? []).filter(n => n.kind === "FILE").length;
      if (loaded < (knownFiles[id] ?? loaded)) {total.imports=undefined;total.exports=undefined;}
    }
    return totals;
  });
}

export function fileDependencyRoles(graph: Graph): { importing: Set<string>; supplying: Set<string> } {
  return measurePerformance("fileDependencyRoles", () => {
    const fileIDs = new Set(graph.nodes.filter((node) => node.kind === "FILE").map((node) => node.id));
    const importing = new Set<string>();
    const supplying = new Set<string>();
    for (const edge of graph.edges) {
      if (edge.kind === "IMPORTS") { importing.add(edge.from); if (fileIDs.has(edge.to)) supplying.add(edge.to); }
      if (edge.kind === "USES_EXPORT") supplying.add(edge.to);
    }
    return { importing, supplying };
  });
}

export function directlyConnectedPackages(graph: VisibleGraph, packageID: string): Set<string> {
  const connected = new Set([packageID]);
  for (const edge of graph.edges) {
    if (edge.kind !== "IMPORTS") continue;
    if (edge.source === packageID) connected.add(edge.target);
    if (edge.target === packageID) connected.add(edge.source);
  }
  return connected;
}

export function buildExportUseIndex(graph: Graph): ExportUseIndex {
  return measurePerformance("buildExportUseIndex", () => {
    const parent = getGraphIndex(graph).parent;
    const found = new Map<string, Map<string, Set<string>>>();
    for (const edge of graph.edges) {
      if (edge.kind !== "USES_EXPORT") continue;
      const packageID = parent.get(edge.to);
      if (!packageID) continue;
      let packages = found.get(edge.from);
      if (!packages) { packages = new Map(); found.set(edge.from, packages); }
      let files = packages.get(packageID);
      if (!files) { files = new Set(); packages.set(packageID, files); }
      files.add(edge.to);
    }
    return new Map([...found].map(([importer, packages]) => [importer,
      new Map([...packages].map(([packageID, files]) => [packageID, [...files].sort(compare)]))]));
  });
}

// Visual direction is dependency -> importer; saved IMPORTS edges remain file -> package.
export function importCardLines(edge: VisibleEdge, nodes: ReadonlyMap<string, VisibleNode>, exportUses: ExportUseIndex, canonicalNodes: ReadonlyMap<string, GraphNode> = nodes): CardImportLine[] {
  return measurePerformance("importCardLines", () => {
    const importingPackage = nodes.get(edge.source)?.kind === "FILE" ? nodes.get(edge.source)?.parentId ?? edge.source : edge.source;
    const dependencyExpanded = nodes.get(edge.target)?.expanded ?? false;
    const importerExpanded = nodes.get(importingPackage)?.expanded ?? false;
    const importers = [...new Set(edge.evidence.length ? edge.evidence.map((source) => source.from) : [edge.source])].sort(compare);
    const providersByImporter = new Map<string, string[]>();
    for (const fact of edge.evidence) {
      if (canonicalNodes.get(fact.to)?.kind !== "FILE") continue;
      const providers = providersByImporter.get(fact.from);
      if (providers) providers.push(fact.to);
      else providersByImporter.set(fact.from, [fact.to]);
    }
    const directProviders = (importer: string) => providersByImporter.get(importer) ?? [];
    if (!dependencyExpanded && !importers.some((importer) => directProviders(importer).length > 0)) {
      return [{ id: edge.id, canonicalEdgeID: edge.id, source: edge.target, target: importingPackage,
        targetHandle: importerExpanded && importers.length === 1 ? importers[0] : undefined,
        importingFileID: importers.length === 1 && edge.evidence.length ? importers[0] : undefined }];
    }
    const lines: CardImportLine[] = [];
    for (const importer of importers) {
      const moduleProviders = directProviders(importer);
      const providers = [...new Set(moduleProviders.length ? moduleProviders : exportUses.get(importer)?.get(edge.target) ?? [])].sort(compare);
      for (const provider of providers.length ? providers : [undefined]) {
        lines.push({
          // Encode the tuple so separators inside canonical IDs cannot collide.
          id: JSON.stringify([edge.id, importer, provider ?? null]),
          canonicalEdgeID: edge.id,
          source: edge.target,
          sourceHandle: dependencyExpanded ? provider : undefined,
          target: importingPackage,
          targetHandle: importerExpanded ? importer : undefined,
          importingFileID: importer,
          supplyingFileID: provider,
        });
      }
    }
    return lines;
  });
}

// Keep every evidence-backed identity in Flow state, but only paint the last
// line on an identical route. This is the line the old overlapping SVGs exposed
// to mouse clicks. Handles distinguish routes when cards are expanded.
export function occludedCardLineIDs(lines: readonly CardImportLine[]): Set<string> {
  return measurePerformance("occludedCardLineIDs", () => {
      const lastByRoute = new Map<string, string>();
      const hidden = new Set<string>();
      for (const line of lines) {
        const route = JSON.stringify([line.canonicalEdgeID, line.source, line.sourceHandle ?? null, line.target, line.targetHandle ?? null]);
        const previous = lastByRoute.get(route);
        if (previous !== undefined) hidden.add(previous);
        lastByRoute.set(route, line.id);
      }
      return hidden;
  });
}

export interface PresentationInput {
  childCounts?: Record<string,number>;
  fileCounts?: Record<string,number>;
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

export function buildVisibleGraph(input: PresentationInput): VisibleGraph {
  return measurePerformance("buildVisibleGraph", () => {
    const { canonicalGraph, packageProjection, expandedPackages, expandedFiles, visibleDeclarationKinds } = input;
    const { nodes: canonicalNodes, parent, children, importKeys: canonicalImports, imports } = getGraphIndex(canonicalGraph);
    const profiles = buildNodePresentations(canonicalGraph);
    const profileFor = (node: GraphNode) => profiles.get(node.id)!;

    const nodes: VisibleNode[] = [];
    const edges: VisibleEdge[] = [];
    const visibleIDs = new Set<string>();
    const addNode = (node: GraphNode, parentId?: string) => {
      if (visibleIDs.has(node.id)) return;
      visibleIDs.add(node.id);
      nodes.push({
        ...node,
        parentId,
        childCount: (profileFor(node).expansion === "container" ? input.fileCounts?.[node.id] : input.childCounts?.[node.id]) ?? (children.get(node.id) ?? []).filter((child) => profileFor(node).expansion !== "container" || profileFor(child).expansion === "source").length,
        expanded: profileFor(node).expansion === "container" ? expandedPackages.has(node.id) :
          profileFor(node).expansion === "source" ? expandedFiles.has(node.id) : false,
      });
      if (parentId) {
        edges.push({ id: `CONTAINS:${parentId}->${node.id}`, kind: "CONTAINS", source: parentId, target: node.id, evidence: [] });
      }
    };

    for (const pkg of canonicalGraph.nodes.filter((node) => profileFor(node).primaryCard && profileFor(node).expansion === "container").sort((a, b) => compare(a.id, b.id))) {
      addNode(pkg);
      if (!expandedPackages.has(pkg.id)) continue;
      for (const file of children.get(pkg.id) ?? []) {
        if (profileFor(file).expansion !== "source") continue;
        addNode(file, pkg.id);
        if (!expandedFiles.has(file.id)) continue;
        for (const declaration of children.get(file.id) ?? []) {
          if (isDeclarationKind(declaration.kind) && visibleDeclarationKinds.has(declaration.kind)) {
            addNode(declaration, file.id);
          }
        }
      }
    }

    for (const file of canonicalGraph.nodes.filter((node) => profileFor(node).primaryCard && profileFor(node).expansion === "source")) {
      addNode(file);
      if (expandedFiles.has(file.id)) for (const declaration of children.get(file.id) ?? []) {
        if (isDeclarationKind(declaration.kind) && visibleDeclarationKinds.has(declaration.kind)) addNode(declaration, file.id);
      }
    }

    const evidenceByEdge = new Map(packageProjection.evidence.map((item) => [edgeKey(item.edge), item.sources]));
    for (const edge of [...packageProjection.graph.edges].sort((a, b) => compare(edgeKey(a), edgeKey(b)))) {
      if (edge.kind !== "IMPORTS" || !visibleIDs.has(edge.from) || !visibleIDs.has(edge.to)) continue;
      const evidence = [...new Map((evidenceByEdge.get(edgeKey(edge)) ?? []).map((source) => [edgeKey(source), source])).values()].sort((a, b) => compare(edgeKey(a), edgeKey(b)));
      if (!expandedPackages.has(edge.from)) {
        edges.push({ id: edgeKey(edge), kind: "IMPORTS", source: edge.from, target: edge.to, projectionEdge: edge, evidence, supportingCount:packageProjection.relationship_totals?.[edgeKey(edge)] });
        continue;
      }
      for (const source of evidence) {
        if (source.kind !== "IMPORTS" || !visibleIDs.has(source.from) || !canonicalImports.has(edgeKey(source))) continue;
        edges.push({ id: edgeKey(source), kind: "IMPORTS", source: source.from, target: edge.to, projectionEdge: edge, evidence: [source] });
      }
    }

    // Imports without a package projection still use the saved canonical fact.
    const projectedFacts = new Set(packageProjection.evidence.flatMap((item) => item.sources.map(edgeKey)));
    for (const fact of imports) {
      if (projectedFacts.has(edgeKey(fact))) continue;
      const source = parent.get(fact.from) ?? fact.from;
      const target = canonicalNodes.get(fact.to)?.kind === "FILE" ? parent.get(fact.to) ?? fact.to : fact.to;
      if (visibleIDs.has(source) && visibleIDs.has(target)) edges.push({ id: edgeKey(fact), kind: "IMPORTS", source: visibleIDs.has(fact.from) ? fact.from : source, target, evidence: [fact] });
    }

    nodes.sort((a, b) => compare(a.id, b.id));
    edges.sort((a, b) => compare(a.id, b.id));
    return { nodes, edges };
  });
}

export function isDeclarationKind(kind: GraphNode["kind"]): kind is DeclarationKind {
  return (declarationKinds as readonly string[]).includes(kind);
}

export function revealNode(graph: Graph, id: string, state: RevealState): RevealState {
  return measurePerformance("revealNode", () => {
    const nodes = getGraphIndex(graph).nodes;
    const parent = getGraphIndex(graph).parent;
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
  });
}

export function searchNodes(graph: Graph, query: string): GraphNode[] {
  return measurePerformance("searchNodes", () => {
    const term = query.trim().toLocaleLowerCase();
    if (!term) return [];
    return graph.nodes
      .filter((node) => node.kind !== "MODULE" && [node.name, node.path, node.id].some((field) => field.toLocaleLowerCase().includes(term)))
      .sort((a, b) => compare(a.name, b.name) || compare(a.id, b.id))
      .slice(0, 40);
  });
}

export function buildExplorerTree(canonicalGraph: Graph, visibleGraph: VisibleGraph): {
  packages: VisibleNode[];
  modules: VisibleNode[];
  children: Map<string, VisibleNode[]>;
} {
  return measurePerformance("buildExplorerTree", () => {
    const { nodes, parent: parents } = getGraphIndex(canonicalGraph);
    const visibleIDs = new Set(visibleGraph.nodes.map((node) => node.id));
    const packages: VisibleNode[] = [];
    const modules: VisibleNode[] = [];
    const children = new Map<string, VisibleNode[]>();
    for (const node of visibleGraph.nodes) {
      const parentID = parents.get(node.id);
      const parent = nodes.get(parentID ?? "");
      const profile = presentationFor(node, parent);
      if (parentID && visibleIDs.has(parentID)) {
        const siblings = children.get(parentID) ?? [];
        siblings.push(node);
        children.set(parentID, siblings);
      } else if (profile.explorerSection === "packages") packages.push(node);
      else if (profile.explorerSection === "modules") modules.push(node);
    }
    for (const group of [packages, modules, ...children.values()]) group.sort((a, b) => compare(a.id, b.id));
    return { packages, modules, children };
  });
}
