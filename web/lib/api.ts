import type { ApiError, Impact, LoadedData, Scan } from "./types";

import { SnapshotCache } from "./loading";
const cache = new SnapshotCache();
let snapshotID: string | undefined;
export function pinSnapshot(id?: string) { snapshotID = id; cache.pin(id); }

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  if (path === "/api/v1/scan" || path.startsWith("/api/v1/source-status")) return fetchJSON(path,signal);
  return cache.get(path, () => fetchJSON<T>(path,signal), signal);
}

async function fetchJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response;
  try {
    const expected = snapshotID;
    response = await fetch(path, { signal, headers: expected ? { "X-Nami-Snapshot": expected } : undefined });
    if (signal?.aborted) throw new Error("Request cancelled.");
    if (expected && response.headers.get("X-Nami-Snapshot") && response.headers.get("X-Nami-Snapshot") !== expected) throw new Error("Snapshot changed; reload the map.");
  } catch (error) {
    if (signal?.aborted) throw error;
    throw new Error("Unable to connect to the local nami API.");
  }
  if (!response.ok) {
    let message = `nami API returned ${response.status}.`;
    try {
      const body: ApiError = await response.json();
      if (body.error?.message) message = body.error.message;
    } catch {
      // The local proxy may return non-JSON when its backend is offline.
      if (response.status >= 500) message = "Unable to connect to the local nami API.";
    }
    throw new Error(message);
  }
  const result: T = await response.json();
  if (signal?.aborted) throw new Error("Request cancelled.");
  return result;
}

export async function loadInitialData(signal?: AbortSignal): Promise<LoadedData> {
  const scan = await getJSON<Scan>("/api/v1/scan", signal);
  pinSnapshot(scan.id);
  const overview = await getJSON<import("./types").Overview>("/api/v1/overview", signal);
  return { scan, canonicalGraph:overview.graph, packageProjection:overview.package_projection, childCounts:overview.child_counts, fileCounts:overview.file_counts, counts:overview.counts };
}

export function loadImpact(packageID: string, signal?: AbortSignal): Promise<Impact> {
  return getJSON<Impact>(`/api/v1/impact?package_id=${encodeURIComponent(packageID)}`, signal);
}

export function loadNeighborhood(nodeID: string, scope: "canonical" | "package", dependencyOffset = 0, dependentOffset = 0, signal?: AbortSignal): Promise<import("./types").Neighborhood> {
  const params = new URLSearchParams({ node_id: nodeID, scope, dependency_offset: String(dependencyOffset), dependent_offset: String(dependentOffset) });
  return getJSON(`/api/v1/neighborhood?${params}`, signal);
}

export function loadSourceEvidence(edge: import("./types").GraphEdge, scope: "canonical" | "package", offset: number, signal?: AbortSignal): Promise<import("./types").SourceEvidencePage> {
  return getJSON(`/api/v1/evidence?${new URLSearchParams({ ...edge, scope, offset: String(offset) })}`, signal);
}
export function loadSourceStatus(fileID: string, signal?: AbortSignal): Promise<import("./types").SourceStatus> {
  return getJSON(`/api/v1/source-status?${new URLSearchParams({ file_id: fileID })}`, signal);
}

export async function rescan(id: string): Promise<string> {
  const response = await fetch("/api/v1/rescan", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({snapshot_id:id})});
  if (!response.ok) {
    const error: ApiError = await response.json();
    throw new Error(error.error?.message ?? "Rescan failed.");
  }
  const result: {snapshot_id:string} = await response.json();
  return result.snapshot_id;
}

export function loadChildren(nodeID: string, offset = 0, signal?: AbortSignal): Promise<import("./types").ChildrenPage> {
  return getJSON(`/api/v1/children?${new URLSearchParams({node_id:nodeID,offset:String(offset)})}`, signal);
}
export function loadSearch(query: string, offset = 0, signal?: AbortSignal): Promise<import("./types").NodePage> {
  return getJSON(`/api/v1/search?${new URLSearchParams({query,offset:String(offset)})}`, signal);
}
export function loadInspection(nodeID: string, children = 0, dependencies = 0, dependents = 0, signal?: AbortSignal): Promise<import("./types").Inspection> {
  return getJSON(`/api/v1/inspect?${new URLSearchParams({node_id:nodeID,offset:String(children),dependency_offset:String(dependencies),dependent_offset:String(dependents)})}`, signal);
}
export function loadPackageDetail(nodeID: string, signal?: AbortSignal): Promise<import("./types").GraphDetail> {
  return getJSON(`/api/v1/package-detail?${new URLSearchParams({node_id:nodeID})}`, signal);
}
export function loadDeclarations(fileID: string, signal?: AbortSignal): Promise<import("./types").GraphDetail> {
  return getJSON(`/api/v1/file-detail?${new URLSearchParams({node_id:fileID})}`,signal);
}
export function loadRelationshipFacts(edge: import("./types").GraphEdge, scope: "canonical" | "package", offset: number, signal?: AbortSignal): Promise<import("./types").RelationshipFacts> {
  return getJSON(`/api/v1/relationship-facts?${new URLSearchParams({...edge,scope,offset:String(offset)})}`,signal);
}
