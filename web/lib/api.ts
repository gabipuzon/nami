import type { ApiError, Graph, Impact, LoadedData, PackageProjection, Scan } from "./types";

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, { signal });
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
  return response.json() as Promise<T>;
}

export async function loadInitialData(signal?: AbortSignal): Promise<LoadedData> {
  const [scan, canonicalGraph, packageProjection] = await Promise.all([
    getJSON<Scan>("/api/v1/scan", signal),
    getJSON<Graph>("/api/v1/graph", signal),
    getJSON<PackageProjection>("/api/v1/packages", signal),
  ]);
  return { scan, canonicalGraph, packageProjection };
}

export function loadImpact(packageID: string, signal?: AbortSignal): Promise<Impact> {
  return getJSON<Impact>(`/api/v1/impact?package_id=${encodeURIComponent(packageID)}`, signal);
}
