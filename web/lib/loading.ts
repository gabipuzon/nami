import type { Graph, GraphDetail, LoadedData } from "./types.ts";
import { factKey } from "./graphIndex.ts";

export class SnapshotCache {
  private snapshot: string | undefined;
  private generation = 0;
  private values = new Map<string, unknown>();
  pin(id?: string): void {
    if (id === this.snapshot) return;
    this.snapshot = id; this.generation++; this.values.clear();
  }
  async get<T>(key: string, load: () => Promise<T>, signal?: AbortSignal): Promise<T> {
    if (signal?.aborted) throw new Error("Request cancelled.");
    if (this.values.has(key)) return this.values.get(key) as T;
    const generation = this.generation;
    const value = await load();
    if (signal?.aborted || generation !== this.generation) throw new Error("Snapshot request superseded.");
    this.values.set(key,value);
    return value;
  }
}

// Merge only facts returned by the server. Missing detail is never filled in.
export function mergeGraph(current: Graph, next: Graph): Graph {
  const nodes = new Map(current.nodes.map(n => [n.id,n]));
  next.nodes.forEach(n => nodes.set(n.id,n));
  const edges = new Map(current.edges.map(e => [factKey(e),e]));
  next.edges.forEach(e => edges.set(factKey(e),e));
  return {nodes:[...nodes.values()],edges:[...edges.values()]};
}
export function mergeDetail(current: LoadedData, next: GraphDetail): LoadedData {
  const evidence = new Map(current.packageProjection.evidence.map(e => [factKey(e.edge),e]));
  next.package_projection.evidence.forEach(item => {
    const key=factKey(item.edge);
    const sources=new Map((evidence.get(key)?.sources ?? []).map(source=>[factKey(source),source]));
    item.sources.forEach(source=>sources.set(factKey(source),source));
    evidence.set(key,{edge:item.edge,sources:[...sources.values()]});
  });
  return {...current,canonicalGraph:mergeGraph(current.canonicalGraph,next.graph),childCounts:{...current.childCounts,...next.child_counts},fileCounts:{...current.fileCounts,...next.file_counts},
    packageProjection:{graph:mergeGraph(current.packageProjection.graph,next.package_projection.graph),evidence:[...evidence.values()],relationship_totals:{...current.packageProjection.relationship_totals,...next.package_projection.relationship_totals}}};
}
