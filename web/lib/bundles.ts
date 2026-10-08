import type { CardImportLine, VisibleEdge } from "./presentation.ts";
import { factKey } from "./graphIndex.ts";
import type { GraphEdge } from "./types.ts";

export const ABOVE = "window:above";
export const BELOW = "window:below";
export interface RowWindow { above: readonly string[]; visible: readonly string[]; below: readonly string[] }
export interface RouteBundle extends CardImportLine {
  kind: GraphEdge["kind"];
  members: CardImportLine[];
  facts: GraphEdge[];
  unloadedFacts?: number;
}

export function sameWindow(a: RowWindow | undefined, b: RowWindow): boolean {
  return !!a && (["above", "visible", "below"] as const).every(key => a[key].length === b[key].length && a[key].every((id, i) => id === b[key][i]));
}

// Routes are a display aggregation. Members and saved evidence remain exact.
export function bundleRoutes(lines: readonly CardImportLine[], edges: ReadonlyMap<string, VisibleEdge>, windows: ReadonlyMap<string, RowWindow>): RouteBundle[] {
  const endpoints = new Map<string, Map<string, string>>();
  for (const [id, window] of windows) endpoints.set(id, new Map([
    ...window.above.map(file => [file, ABOVE] as const),
    ...window.visible.map(file => [file, file] as const),
    ...window.below.map(file => [file, BELOW] as const),
  ]));
  const handle = (card: string, file: string | undefined) => file ? endpoints.get(card)?.get(file) ?? BELOW : undefined;
  const bundles = new Map<string, RouteBundle>();
  for (const line of lines) {
    const edge = edges.get(line.canonicalEdgeID)!;
    const sourceHandle = handle(line.source, line.sourceHandle);
    const targetHandle = handle(line.target, line.targetHandle);
    const id = JSON.stringify([edge.kind, line.source, sourceHandle ?? null, line.target, targetHandle ?? null]);
    let bundle = bundles.get(id);
    if (!bundle) {
      bundle = { ...line, id, sourceHandle, targetHandle, kind: edge.kind, members: [], facts: [] };
      bundles.set(id, bundle);
    }
    bundle.members.push(line);
  }
  for (const bundle of bundles.values()) {
    bundle.members.sort((a,b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    const representative = bundle.members[0];
    bundle.canonicalEdgeID = representative.canonicalEdgeID;
    bundle.importingFileID = representative.importingFileID;
    bundle.supplyingFileID = representative.supplyingFileID;
    const facts = new Map<string, GraphEdge>();
    const unloaded = new Map<string,number>();
    for (const member of bundle.members) {
      const edge = edges.get(member.canonicalEdgeID)!;
      // A folded import may stand for several facts. A row route selects the
      // corresponding importer/provider facts without inventing file imports.
      for (const fact of edge.evidence) {
        if (member.importingFileID && fact.from !== member.importingFileID) continue;
        if (member.supplyingFileID && fact.to !== member.supplyingFileID && fact.to !== edge.target) continue;
        facts.set(factKey(fact), fact);
      }
      if (edge.projectionEdge && (edge.supportingCount ?? 0) > edge.evidence.length) unloaded.set(factKey(edge.projectionEdge), (edge.supportingCount ?? 0)-edge.evidence.length);
    }
    bundle.unloadedFacts = [...unloaded.values()].reduce((a,b) => a+b,0);
    bundle.facts = [...facts.values()].sort((a,b) => factKey(a).localeCompare(factKey(b)));
  }
  return [...bundles.values()].sort((a,b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
}

export function rowWindow(ids: readonly string[], first: number, last: number): RowWindow {
  return { above: ids.slice(0,first), visible: ids.slice(first,last), below: ids.slice(last) };
}
export function connectedHandles(window: RowWindow, connected: ReadonlySet<string>): string[] {
  return [ ...(window.above.some(id => connected.has(id)) ? [ABOVE] : []),
    ...window.visible.filter(id => connected.has(id)),
    ...(window.below.some(id => connected.has(id)) ? [BELOW] : []) ];
}
