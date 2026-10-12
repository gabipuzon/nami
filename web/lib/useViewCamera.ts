"use client";

import { useCallback, useEffect, useRef } from "react";
import { useNodesInitialized, useReactFlow, useStore, type Edge } from "@xyflow/react";
import type { MapFlowNode } from "../components/MapNode";
import { CameraInitialization, selectCamera, type Camera } from "./viewport";

interface ViewCameraOptions {
  padding: number;
  focalID?: string;
  restore?: Camera;
  ready?: boolean;
}

export function useViewCamera(view: string | number, nodes: MapFlowNode[], options: ViewCameraOptions) {
  const { padding, focalID, restore, ready = true } = options;
  const width = useStore(s => s.width);
  const height = useStore(s => s.height);
  const measured = useNodesInitialized();
  const { getNodes, setViewport, viewportInitialized } = useReactFlow<MapFlowNode, Edge>();
  const initialization = useRef(new CameraInitialization());
  const apply = useCallback((mode: "readable" | "all") => {
    // A new focus must have its saved/layout positions before using geometry.
    // Once initialized, dragging away from those positions cannot disable Fit all.
    if ((!ready && initialization.current.pending(view)) || !viewportInitialized || !measured || width <= 0 || height <= 0) return false;
    const cards = getNodes().map(n => ({
      id: n.id, path: n.data.item.path, ...n.position,
      width: n.measured?.width ?? 0, height: n.measured?.height ?? 0,
    }));
    const camera = mode === "readable" && restore
      ? restore
      : selectCamera(cards, { width, height }, padding, mode, focalID);
    if (!camera) return false;
    void setViewport(camera, { duration: 0 });
    return true;
  }, [ready, view, viewportInitialized, measured, width, height, getNodes, setViewport, padding, focalID, restore]);

  useEffect(() => {
    if (initialization.current.pending(view) && apply("readable")) initialization.current.complete(view);
  }, [view, nodes, apply]);

  return useCallback(() => {
    if (apply("all")) initialization.current.complete(view);
  }, [apply, view]);
}
