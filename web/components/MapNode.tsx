"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Handle, Position, useUpdateNodeInternals, type Node, type NodeProps } from "@xyflow/react";
import type { VisibleNode } from "../lib/presentation";

export interface CardFile {
  file: VisibleNode;
  declarations: VisibleNode[];
}

export type MapFlowNode = Node<{
  item: VisibleNode;
  files: CardFile[];
  selectedID: string | null;
  impactDistance?: number;
  impactTarget: boolean;
  dimmed: boolean;
  importingFiles: ReadonlySet<string>;
  supplyingFiles: ReadonlySet<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
}, "map">;

export const MapNode = memo(function MapNode({ id, data }: NodeProps<MapFlowNode>) {
  const { item, files, selectedID, impactDistance, impactTarget, dimmed, importingFiles, supplyingFiles, onToggle, onSelect } = data;
  const listRef = useRef<HTMLDivElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef<number | null>(null);
  const [range, setRange] = useState({ first: 0, last: Math.min(files.length, 10) });
  const [anchors, setAnchors] = useState<Record<string, number>>({});
  const updateNodeInternals = useUpdateNodeInternals();
  const updateGeometry = useCallback(() => {
    const list = listRef.current;
    const card = cardRef.current;
    if (!list || !card) return;
    const rows = Array.from(list.querySelectorAll<HTMLElement>(".package-file-row"));
    const listRect = list.getBoundingClientRect();
    const cardRect = card.getBoundingClientRect();
    const scale = cardRect.height / card.offsetHeight;
    if (!scale) return;
    const top = listRect.top;
    const bottom = listRect.bottom;
    const visible = rows.map((row, index) => ({ row, index })).filter(({ row }) => {
      const rect = row.getBoundingClientRect();
      return rect.bottom > top && rect.top < bottom;
    });
    const first = (visible[0]?.index ?? 0) + 1;
    const last = (visible.at(-1)?.index ?? -1) + 1;
    setRange((current) => current.first === first && current.last === last ? current : { first, last });
    const next: Record<string, number> = {};
    for (const row of rows) {
      const main = row.querySelector<HTMLElement>(".package-file-main");
      const fileID = row.dataset.nodeId;
      if (!main || !fileID) continue;
      const rect = main.getBoundingClientRect();
      next[fileID] = (Math.max(top + 4 * scale, Math.min(bottom - 4 * scale, rect.top + rect.height / 2)) - cardRect.top) / scale;
    }
    setAnchors((current) => files.every(({ file }) => current[file.id] === next[file.id]) ? current : next);
  }, [files]);
  useLayoutEffect(() => { updateGeometry(); }, [updateGeometry, item.expanded]);
  useLayoutEffect(() => { if (item.expanded) updateNodeInternals(id); }, [anchors, id, item.expanded, updateNodeInternals]);
  useEffect(() => () => { if (frameRef.current !== null) cancelAnimationFrame(frameRef.current); }, []);
  const onListScroll = () => {
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    frameRef.current = requestAnimationFrame(() => { frameRef.current = null; updateGeometry(); });
  };
  useEffect(() => {
    if (!selectedID || !item.expanded) return;
    const list = listRef.current;
    const target = Array.from(list?.querySelectorAll<HTMLElement>("[data-node-id]") ?? []).find((row) => row.dataset.nodeId === selectedID);
    if (list && target) {
      const top = target.getBoundingClientRect().top - list.getBoundingClientRect().top + list.scrollTop;
      if (top < list.scrollTop) list.scrollTop = top;
      else if (top + target.offsetHeight > list.scrollTop + list.clientHeight) list.scrollTop = top + target.offsetHeight - list.clientHeight;
      updateGeometry();
    }
  }, [selectedID, item.expanded, updateGeometry]);

  return <div ref={cardRef} className={["map-node", "map-node-package", item.expanded ? "is-expanded" : "", selectedID === item.id ? "is-selected" : "", impactTarget ? "is-impact-target" : "", impactDistance !== undefined ? "is-affected" : "", dimmed ? "is-dimmed" : ""].filter(Boolean).join(" ")} aria-label={`package ${item.name}`}>
    <Handle type="target" position={Position.Left} isConnectable={false} className="map-handle" />
    <div className="map-node-main">
      <span className="map-node-name" title={item.name}>{item.name}</span>
      {item.childCount > 0 && <button className="map-node-toggle nodrag nopan" type="button" onClick={(event) => { event.stopPropagation(); onToggle(item.id); }} aria-label={`${item.expanded ? "Collapse" : "Expand"} ${item.name}`}>{item.expanded ? "▾" : "▸"}</button>}
    </div>
    <div className="map-node-meta"><span>{item.childCount} {item.childCount === 1 ? "file" : "files"}</span>{impactDistance !== undefined && <span className="impact-distance">distance {impactDistance}</span>}</div>
    {item.expanded && <>
      <div className="package-list-head"><span>Files</span><span className="package-list-import-label">imports</span><span className="package-list-export-label">exports</span></div>
      <div className="package-file-list nodrag nopan nowheel" ref={listRef} onScroll={onListScroll}>
        {files.map(({ file, declarations }) => <div className={["package-file-row", importingFiles.has(file.id) ? "has-import" : "", supplyingFiles.has(file.id) ? "has-export" : "", !importingFiles.has(file.id) && !supplyingFiles.has(file.id) ? "is-disconnected" : ""].filter(Boolean).join(" ")} key={file.id} data-node-id={file.id}>
          <div className={`package-file-main ${selectedID === file.id ? "is-row-selected" : ""}`}>
            {file.childCount > 0 && <button className="package-file-toggle nodrag nopan" type="button" onClick={(event) => { event.stopPropagation(); onToggle(file.id); }} aria-label={`${file.expanded ? "Collapse" : "Expand"} ${file.name}`}>{file.expanded ? "▾" : "▸"}</button>}
            <button className="package-file-name nodrag nopan" type="button" title={file.path} onClick={(event) => { event.stopPropagation(); onSelect(file.id); }}>{file.name}</button>
            <span className="package-file-counts" title={file.import_count === undefined ? "Counts unavailable for this scan" : `${file.import_count} imports, ${file.export_count} exports`}><span>{file.import_count ?? "—"}</span><span>{file.export_count ?? "—"}</span></span>
          </div>
          {file.expanded && declarations.map((declaration) => <button key={declaration.id} data-node-id={declaration.id} type="button" className={`package-declaration nodrag nopan ${selectedID === declaration.id ? "is-row-selected" : ""}`} onClick={(event) => { event.stopPropagation(); onSelect(declaration.id); }} title={`${declaration.kind.toLowerCase()} ${declaration.name}`}><span>{declaration.name}</span><small>{declaration.kind.toLowerCase()}</small></button>)}
        </div>)}
      </div>
      <div className="package-list-foot">{files.length ? `${range.first}–${range.last} of ${files.length} files` : "No files"}</div>
    </>}
    {item.expanded && files.map(({ file }) => <Handle key={`in:${file.id}`} id={file.id} type="target" position={Position.Left} isConnectable={false} className="map-handle" style={{ top: anchors[file.id] ?? 72, left: 3 }} />)}
    {item.expanded && files.map(({ file }) => <Handle key={`out:${file.id}`} id={file.id} type="source" position={Position.Right} isConnectable={false} className="map-handle" style={{ top: anchors[file.id] ?? 72, right: 3 }} />)}
    <Handle type="source" position={Position.Right} isConnectable={false} className="map-handle" />
  </div>;
});
