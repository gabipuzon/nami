import type { Edge, InternalNode, Node } from "@xyflow/react";

const shallowEqual = (a: Record<string, unknown>, b: Record<string, unknown>): boolean => {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every(key => a[key] === b[key]);
};

// Presentation updates must retain Flow's measured dimensions. Dropping them
// makes every selection trigger ResizeObserver and a second store update.
export function synchronizeNodes<T extends Node>(current: T[], next: T[], previous: T[]): T[] {
  if (current === next) return current;
  const currentByID = new Map(current.map(node => [node.id, node]));
  const previousByID = new Map(previous.map(node => [node.id, node]));
  return next.map(node => {
    const existing = currentByID.get(node.id);
    if (!existing) return node;
    const before = previousByID.get(node.id);
    const positionUnchanged = before?.position.x === node.position.x && before?.position.y === node.position.y;
    return {
      ...node,
      measured: existing.measured,
      dragging: existing.dragging,
      resizing: existing.resizing,
      position: positionUnchanged ? existing.position : node.position,
      data: shallowEqual(existing.data, node.data) ? existing.data : node.data,
    };
  });
}

export type HandleSnapshot = readonly (readonly [string, InternalNode["internals"]["handleBounds"]])[];
export function equalHandleSnapshots(a: HandleSnapshot, b: HandleSnapshot): boolean {
  return a.length === b.length && a.every(([id, bounds], i) => id === b[i][0] && bounds === b[i][1]);
}

// Flow calculates geometry even for hidden edges. Until a newly mounted file
// handle is registered, keep the edge hidden and let Flow measure its existing
// card handles. Restore the exact file handles as soon as they are available.
export function edgesWithRegisteredHandles<T extends Edge>(edges: T[], snapshot: HandleSnapshot): T[] {
  const handles = new Map(snapshot.map(([id, bounds]) => [id, {
    source: new Set(bounds?.source?.map(handle => handle.id)),
    target: new Set(bounds?.target?.map(handle => handle.id)),
  }]));
  let changed = false;
  const ready = edges.map(edge => {
    const sourceReady = !edge.sourceHandle || handles.get(edge.source)?.source.has(edge.sourceHandle);
    const targetReady = !edge.targetHandle || handles.get(edge.target)?.target.has(edge.targetHandle);
    if (sourceReady && targetReady) return edge;
    changed = true;
    return { ...edge, hidden: true, sourceHandle: sourceReady ? edge.sourceHandle : undefined, targetHandle: targetReady ? edge.targetHandle : undefined };
  });
  return changed ? ready : edges;
}
