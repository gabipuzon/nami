"use client";

import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { VisibleNode } from "../lib/presentation";

export type MapFlowNode = Node<{
  item: VisibleNode;
  selected: boolean;
  impactDistance?: number;
  impactTarget: boolean;
  onToggle: (id: string) => void;
}, "map">;

export function MapNode({ data }: NodeProps<MapFlowNode>) {
  const { item, selected, impactDistance, impactTarget, onToggle } = data;
  const canExpand = item.childCount > 0 && (item.kind === "PACKAGE" || item.kind === "FILE");
  const className = [
    "map-node",
    item.kind === "PACKAGE" ? "map-node-package" : item.kind === "FILE" ? "map-node-file" : "map-node-declaration",
    selected ? "is-selected" : "",
    impactTarget ? "is-impact-target" : "",
    impactDistance !== undefined ? "is-affected" : "",
  ].filter(Boolean).join(" ");
  return (
    <div className={className} aria-label={`${item.kind.toLowerCase()} ${item.name}`}>
      <Handle type="target" position={Position.Left} isConnectable={false} className="map-handle" />
      <div className="map-node-main">
        <span className="map-node-name" title={item.name}>{item.name}</span>
        {canExpand && <button
          className="map-node-toggle nodrag nopan"
          type="button"
          onClick={(event) => { event.stopPropagation(); onToggle(item.id); }}
          aria-label={`${item.expanded ? "Collapse" : "Expand"} ${item.name}`}
          title={`${item.expanded ? "Collapse" : "Expand"} ${item.name}`}
        >{item.expanded ? "−" : "+"}</button>}
      </div>
      <div className="map-node-meta">
        <span>{item.kind.toLowerCase()}</span>
        {canExpand && <span>{item.childCount} {item.kind === "PACKAGE" ? "files" : "symbols"}</span>}
        {impactDistance !== undefined && <span className="impact-distance">distance {impactDistance}</span>}
      </div>
      <Handle type="source" position={Position.Right} isConnectable={false} className="map-handle" />
    </div>
  );
}
