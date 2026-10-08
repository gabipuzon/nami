"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Handle, Position, useUpdateNodeInternals, type Node, type NodeProps } from "@xyflow/react";
import { sourceStatsTitle, type NodePresentation } from "../lib/languagePresentation";
import type { VisibleNode } from "../lib/presentation";

export interface PresentedNode extends VisibleNode {
  presentation: NodePresentation;
}

export interface CardFile {
  file: PresentedNode;
  declarations: PresentedNode[];
}

export type MapFlowNode = Node<{
  item: VisibleNode;
  presentation: NodePresentation;
  files: CardFile[];
  selectedID: string | null;
  selectedSupplierID: string | null;
  selectedImporterID: string | null;
  impactDistance?: number;
  impactTarget: boolean;
  dimmed: boolean;
  importingFiles: ReadonlySet<string>;
  supplyingFiles: ReadonlySet<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
}, "map">;

const FILE_ROW_HEIGHT = 31;
const DECLARATION_ROW_HEIGHT = 26;

function firstRowEndingAfter(offsets: number[], position: number): number {
  let low = 0;
  let high = offsets.length - 1;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (offsets[middle + 1] <= position) low = middle + 1;
    else high = middle;
  }
  return low;
}

function firstRowStartingAtOrAfter(offsets: number[], position: number): number {
  let low = 0;
  let high = offsets.length - 1;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (offsets[middle] < position) low = middle + 1;
    else high = middle;
  }
  return low;
}

export const MapNode = memo(function MapNode({ id, data }: NodeProps<MapFlowNode>) {
  const { item, files, selectedID, selectedSupplierID, selectedImporterID, impactDistance, impactTarget, dimmed, importingFiles, supplyingFiles, presentation, onToggle, onSelect } = data;
  const listRef = useRef<HTMLDivElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef<number | null>(null);
  const [range, setRange] = useState({ first: 0, last: Math.min(files.length, 10), renderFirst: 0, renderLast: Math.min(files.length, 12) });
  const [anchors, setAnchors] = useState<Record<string, number>>({});
  const rowLayout = useMemo(() => {
    const offsets = [0];
    const locations = new Map<string, { top: number; bottom: number }>();
    for (const { file, declarations } of files) {
      const top = offsets[offsets.length - 1];
      locations.set(file.id, { top, bottom: top + FILE_ROW_HEIGHT });
      if (file.expanded) declarations.forEach((declaration, index) => {
        const declarationTop = top + 30 + index * DECLARATION_ROW_HEIGHT;
        locations.set(declaration.id, { top: declarationTop, bottom: declarationTop + DECLARATION_ROW_HEIGHT });
      });
      offsets.push(top + FILE_ROW_HEIGHT + (file.expanded ? declarations.length * DECLARATION_ROW_HEIGHT : 0));
    }
    return { offsets, locations };
  }, [files]);
  const connectedFiles = useMemo(() => files.map(({ file }, index) => ({ id: file.id, top: rowLayout.offsets[index] })).filter(({ id: fileID }) => importingFiles.has(fileID) || supplyingFiles.has(fileID)), [files, rowLayout, importingFiles, supplyingFiles]);
  const updateNodeInternals = useUpdateNodeInternals();
  const updateGeometry = useCallback(() => {
    const list = listRef.current;
    const card = cardRef.current;
    if (!list || !card) return;
    const listRect = list.getBoundingClientRect();
    const cardRect = card.getBoundingClientRect();
    const scale = cardRect.height / card.offsetHeight;
    if (!scale) return;
    const { offsets } = rowLayout;
    const first = firstRowEndingAfter(offsets, list.scrollTop);
    const last = firstRowStartingAtOrAfter(offsets, list.scrollTop + list.clientHeight);
    const renderFirst = Math.max(0, first - 2);
    const renderLast = Math.min(files.length, last + 2);
    setRange((current) => current.first === first && current.last === last && current.renderFirst === renderFirst && current.renderLast === renderLast ? current : { first, last, renderFirst, renderLast });
    const next: Record<string, number> = {};
    const listTop = (listRect.top - cardRect.top) / scale;
    for (const file of connectedFiles) {
      next[file.id] = Math.max(listTop + 4, Math.min(listTop + list.clientHeight - 4, listTop + file.top + 15 - list.scrollTop));
    }
    setAnchors((current) => connectedFiles.every(({ id: fileID }) => current[fileID] === next[fileID]) && Object.keys(current).length === connectedFiles.length ? current : next);
  }, [files.length, rowLayout, connectedFiles]);
  useLayoutEffect(() => { updateGeometry(); }, [updateGeometry, item.expanded]);
  useLayoutEffect(() => { if (item.expanded) updateNodeInternals(id); }, [anchors, id, item.expanded, updateNodeInternals]);
  useEffect(() => () => { if (frameRef.current !== null) cancelAnimationFrame(frameRef.current); }, []);
  const onListScroll = () => {
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    frameRef.current = requestAnimationFrame(() => { frameRef.current = null; updateGeometry(); });
  };
  useEffect(() => {
    if (!item.expanded) return;
    const list = listRef.current;
    const target = [selectedID, selectedImporterID, selectedSupplierID].map((selected) => rowLayout.locations.get(selected ?? "")).find(Boolean);
    if (list && target) {
      if (target.top < list.scrollTop) list.scrollTop = target.top;
      else if (target.bottom > list.scrollTop + list.clientHeight) list.scrollTop = target.bottom - list.clientHeight;
      updateGeometry();
    }
  }, [selectedID, selectedImporterID, selectedSupplierID, rowLayout, item.expanded, updateGeometry]);

  return <div ref={cardRef} className={["map-node", "map-node-package", item.expanded ? "is-expanded" : "", selectedID === item.id ? "is-selected" : "", impactTarget ? "is-impact-target" : "", impactDistance !== undefined ? "is-affected" : "", dimmed ? "is-dimmed" : ""].filter(Boolean).join(" ")} aria-label={`${presentation.displayKind} ${presentation.displayName}`}>
    <Handle type="target" position={Position.Left} isConnectable={false} className="map-handle" />
    <div className="map-node-main">
      <span className="map-node-identity"><span className="map-node-name" title={item.path}>{presentation.cardName}</span>{presentation.secondaryLabel && <span className="map-node-package-label" title={presentation.secondaryLabel}>{presentation.secondaryLabel}</span>}</span>
      {item.childCount > 0 && <button className="map-node-toggle nodrag nopan" type="button" onClick={(event) => { event.stopPropagation(); onToggle(item.id); }} aria-label={`${item.expanded ? "Collapse" : "Expand"} ${presentation.displayName}`}>{item.expanded ? "▾" : "▸"}</button>}
    </div>
    <div className="map-node-meta"><span>{item.childCount} {item.childCount === 1 ? presentation.childLabel.singular : presentation.childLabel.plural}</span>{!item.expanded && <span className="map-node-folded-counts" title={sourceStatsTitle(presentation.sourceStats)}><span>in {presentation.sourceStats.imports ?? "—"}</span><span>out {presentation.sourceStats.exports ?? "—"}</span></span>}{impactDistance !== undefined && <span className="impact-distance">distance {impactDistance}</span>}</div>
    {item.expanded && <>
      <div className="package-list-head"><span>{presentation.rowGroupLabel}</span><span className="package-list-import-label">imports</span><span className="package-list-export-label">exports</span></div>
      <div className="package-file-list nodrag nopan nowheel" ref={listRef} onScroll={onListScroll}>
        <div aria-hidden="true" style={{ height: rowLayout.offsets[range.renderFirst] ?? 0 }} />
        {files.slice(range.renderFirst, range.renderLast).map(({ file, declarations }) => <div className={["package-file-row", importingFiles.has(file.id) ? "has-import" : "", supplyingFiles.has(file.id) ? "has-export" : "", !importingFiles.has(file.id) && !supplyingFiles.has(file.id) ? "is-disconnected" : "", selectedImporterID === file.id ? "is-line-importer" : "", selectedSupplierID === file.id ? "is-line-supplier" : ""].filter(Boolean).join(" ")} key={file.id} data-node-id={file.id}>
          <div className={`package-file-main ${selectedID === file.id ? "is-row-selected" : ""}`}>
            {file.childCount > 0 && <button className="package-file-toggle nodrag nopan" type="button" onClick={(event) => { event.stopPropagation(); onToggle(file.id); }} aria-label={`${file.expanded ? "Collapse" : "Expand"} ${file.presentation.displayName}`}>{file.expanded ? "▾" : "▸"}</button>}
            <button className="package-file-name nodrag nopan" type="button" title={file.path} onClick={(event) => { event.stopPropagation(); onSelect(file.id); }}>{file.presentation.rowName}</button>
            <span className="package-file-counts" title={sourceStatsTitle(file.presentation.sourceStats)}><span>{file.presentation.sourceStats.imports ?? "—"}</span><span>{file.presentation.sourceStats.exports ?? "—"}</span></span>
          </div>
          {file.expanded && declarations.map((declaration) => <button key={declaration.id} data-node-id={declaration.id} type="button" className={`package-declaration nodrag nopan ${selectedID === declaration.id ? "is-row-selected" : ""}`} onClick={(event) => { event.stopPropagation(); onSelect(declaration.id); }} title={`${declaration.presentation.displayKind} ${declaration.presentation.displayName}`}><span>{declaration.presentation.displayName}</span><small>{declaration.presentation.displayKind}</small></button>)}
        </div>)}
        <div aria-hidden="true" style={{ height: rowLayout.offsets[files.length] - (rowLayout.offsets[range.renderLast] ?? rowLayout.offsets[files.length]) }} />
      </div>
      <div className="package-list-foot">{files.length ? `${range.first + 1}–${range.last} of ${files.length} ${presentation.rowGroupLabel.toLowerCase()}` : `No ${presentation.rowGroupLabel.toLowerCase()}`}</div>
    </>}
    {item.expanded && connectedFiles.filter(({ id: fileID }) => importingFiles.has(fileID)).map(({ id: fileID }) => <Handle key={`in:${fileID}`} id={fileID} type="target" position={Position.Left} isConnectable={false} className="map-handle" style={{ top: anchors[fileID] ?? 72, left: 3 }} />)}
    {item.expanded && connectedFiles.filter(({ id: fileID }) => supplyingFiles.has(fileID)).map(({ id: fileID }) => <Handle key={`out:${fileID}`} id={fileID} type="source" position={Position.Right} isConnectable={false} className="map-handle" style={{ top: anchors[fileID] ?? 72, right: 3 }} />)}
    <Handle type="source" position={Position.Right} isConnectable={false} className="map-handle" />
  </div>;
});
