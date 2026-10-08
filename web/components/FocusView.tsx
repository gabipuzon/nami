"use client";

import { useCallback, useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import { ReactFlow, ReactFlowProvider, Background, BackgroundVariant, Controls, useNodesState, useReactFlow, type Edge } from "@xyflow/react";
import { setPerformanceCount } from "../lib/performance";
import { loadNeighborhood } from "../lib/api";
import { backFocus, pushFocus, neighborhoodGraph, RequestGeneration, type FocusEntry } from "../lib/navigation";
import { layoutVisibleGraph } from "../lib/layout";
import { synchronizeNodes } from "../lib/flowState";
import type { GraphNode, LoadedData } from "../lib/types";
import { presentationFor, type NodePresentation } from "../lib/languagePresentation";
import type { VisibleEdge } from "../lib/presentation";
import { MapNode, type MapFlowNode } from "./MapNode";
import { DependencyEdge } from "./DependencyEdge";
import { PageControls } from "./PagedList";
import type { LineStyle } from "../lib/preferences";

const nodeTypes = { map: MapNode };
const edgeTypes = { dependency: DependencyEdge };
const noToggle = () => {};
const emptyFiles: MapFlowNode["data"]["files"] = [];
const emptyIDs = new Set<string>();
interface FocusProps {
  onGraph: (graph: import("../lib/types").Graph) => void;
  target: { node: GraphNode; sequence: number };
  data: LoadedData;
  presentations: ReadonlyMap<string, NodePresentation>;
  selectedID: string | null;
  onInspect: (id: string) => void;
  onEdge: (edge: VisibleEdge) => void;
  onExit: () => void;
  lineStyle: LineStyle;
  showGrid: boolean;
}
const viewKey = (entry: FocusEntry) => JSON.stringify([entry.result.focal.id,entry.result.dependencies.offset,entry.result.dependents.offset]);
interface FocusRequest { node: GraphNode; dependencies: number; dependents: number; follow: boolean }

function FocusCanvas({ onGraph, target, data, presentations, selectedID, onInspect, onEdge, onExit, lineStyle, showGrid }: FocusProps) {
  const [entry, setEntry] = useState<FocusEntry | null>(null);
  const [history, setHistory] = useState<FocusEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nodes, setNodes, onNodesChange] = useNodesState<MapFlowNode>([]);
  const { getNodes, getViewport, setViewport, fitView } = useReactFlow<MapFlowNode, Edge>();
  const previousNodes = useRef<MapFlowNode[]>([]);
  const controller = useRef<AbortController | null>(null);
  const generation = useRef(new RequestGeneration());
  const [retry, setRetry] = useState<FocusRequest | null>(null);
  const pendingViewport = useRef<{ key: string; viewport?: FocusEntry["viewport"] } | null>(null);
  const capture = useCallback((): FocusEntry | null => entry ? {
    ...entry,
    positions: Object.fromEntries(getNodes().map(n => [n.id, { ...n.position }])),
    viewport: getViewport(),
  } : null, [entry, getNodes, getViewport]);
  const request = useCallback(async (action: FocusRequest) => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    const token = generation.current.next();
    setRetry(action);
    setLoading(true); setError(null);
    try {
      const result = await loadNeighborhood(action.node.id, action.node.kind === "PACKAGE" ? "package" : "canonical", action.dependencies, action.dependents, abort.signal);
      if (abort.signal.aborted) return;
      if (result.snapshot_id !== data.scan.id) throw new Error("The API is serving a different snapshot. Reload the saved graph.");
      if (!generation.current.accepts(token,result.snapshot_id,data.scan.id)) return;
      if (result.scope === "canonical") onGraph(result.context_graph ?? {nodes:result.graph.nodes,edges:[]});
      if (action.follow) setHistory(pushFocus(history, capture()));
      const graph = neighborhoodGraph(result);
      const positions = Object.fromEntries(layoutVisibleGraph({ ...graph, edges: graph.edges.map(e => ({ ...e, source: e.target, target: e.source })) }).map(n => [n.id,{x:n.x,y:n.y}]));
      const next = { result, positions };
      pendingViewport.current = { key: viewKey(next) };
      setEntry(next);
      onInspect(result.focal.id);
    } catch (failure) {
      if (!abort.signal.aborted) setError(failure instanceof Error ? failure.message : "Unable to load neighborhood.");
    } finally { if (!abort.signal.aborted) setLoading(false); }
  }, [capture, history, data.scan.id, onInspect, onGraph]);
  const requestTarget = useEffectEvent(request);
  useEffect(() => {
    let active = true;
    void Promise.resolve().then(() => { if (active) void requestTarget({ node: target.node, dependencies: 0, dependents: 0, follow: true }); });
    return () => { active = false; controller.current?.abort(); };
  }, [target]);
  const graph = useMemo(() => entry ? neighborhoodGraph(entry.result) : { nodes: [], edges: [] }, [entry]);
  useEffect(() => {
    const next: MapFlowNode[] = graph.nodes.map(item => ({ id: item.id, type: "map", position: entry?.positions[item.id] ?? {x:0,y:0}, selected: selectedID === item.id,
      data: { item, presentation: presentations.get(item.id) ?? presentationFor(item), files: emptyFiles, selectedID: selectedID === item.id ? selectedID : null, selectedSupplierID: null, selectedImporterID: null,
        impactTarget: false, dimmed: false, importingFiles: emptyIDs, supplyingFiles: emptyIDs, onToggle: noToggle, onWindow: noToggle, onSelect: onInspect } }));
    const previous = entry && pendingViewport.current?.key === viewKey(entry) ? [] : previousNodes.current;
    setNodes(old => synchronizeNodes(old,next,previous));
    previousNodes.current = next;
  }, [graph, entry, selectedID, presentations, onInspect, setNodes]);
  useEffect(() => {
    if (!entry || pendingViewport.current?.key !== viewKey(entry)) return;
    let inner = 0;
    const frame = requestAnimationFrame(() => { inner = requestAnimationFrame(() => {
      const pending = pendingViewport.current;
      if (pending?.key !== viewKey(entry)) return;
      if (pending.viewport) void setViewport(pending.viewport, {duration:0});
      else void fitView({padding:.12,duration:0,minZoom:.01,maxZoom:1.15});
      pendingViewport.current = null;
    }); });
    return () => { cancelAnimationFrame(frame); cancelAnimationFrame(inner); };
  }, [entry, nodes.length, fitView, setViewport]);
  const edges = useMemo<Edge[]>(() => graph.edges.map(e => ({ id:e.id, source:e.target, target:e.source, type:"dependency", data:{lineStyle}, interactionWidth:8 })), [graph,lineStyle]);
  useEffect(() => { setPerformanceCount("routes.focus", edges.length); setPerformanceCount("cards.focus", nodes.length); return () => { setPerformanceCount("routes.focus", undefined); setPerformanceCount("cards.focus", undefined); }; }, [edges.length,nodes.length]);
  const back = () => {
    controller.current?.abort(); generation.current.next(); setLoading(false); setError(null);
    const previous = backFocus(history);
    if (!previous.current) { onExit(); return; }
    pendingViewport.current = { key:viewKey(previous.current), viewport:previous.current.viewport ? {...previous.current.viewport} : undefined };
    setHistory(previous.history); setEntry(previous.current); onInspect(previous.current.result.focal.id);
  };
  return <section className="graph-panel focus-panel" aria-label="Focused import neighborhood">
    <div className="graph-head"><strong>Focus: {entry?.result.focal.path ?? target.node.path}</strong><div className="map-action-controls"><button type="button" onClick={back}>Back</button><button type="button" onClick={onExit}>Return to overview</button></div></div>
    <div className="focus-pages">{entry && <>
      <div><strong>Depends on</strong><PageControls total={entry.result.dependencies.total} offset={entry.result.dependencies.offset} disabled={loading} onChange={offset => void request({node:entry.result.focal,dependencies:offset,dependents:entry.result.dependents.offset,follow:false})} /></div>
      <div><strong>Depended on by</strong><PageControls total={entry.result.dependents.total} offset={entry.result.dependents.offset} disabled={loading} onChange={offset => void request({node:entry.result.focal,dependencies:entry.result.dependencies.offset,dependents:offset,follow:false})} /></div>
    </>}</div>
    {loading && <p className="focus-status" role="status">Loading neighborhood…</p>}
    {error && <p className="focus-status" role="alert">{error} <button type="button" onClick={() => {if (retry) void request(retry);}}>Retry</button></p>}
    {entry && entry.result.dependencies.total === 0 && entry.result.dependents.total === 0 && <p className="focus-status">No known import relationships</p>}
    {!!entry?.result.excluded_paths && <p className="focus-status">Scan scope excludes {entry.result.excluded_paths} paths; directories count once.</p>}
    {entry?.result.incomplete && <p className="focus-status">Analysis has gaps; known relationships may be incomplete.</p>}
    <div className="graph-stage"><ReactFlow<MapFlowNode,Edge> nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onNodesChange={onNodesChange} onNodeClick={(_,n) => onInspect(n.id)} onEdgeClick={(_,e) => {const fact = graph.edges.find(f => f.id === e.id); if (fact) onEdge(fact);}} minZoom={.01} maxZoom={2} nodesConnectable={false} deleteKeyCode={null} proOptions={{hideAttribution:true}}>
      {showGrid && <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid-dot)" />}<Controls showInteractive={false} />
    </ReactFlow></div>
    <div className="graph-footer"><span>{nodes.length} cards · {edges.length} incident imports</span></div>
  </section>;
}
export function FocusView(props: FocusProps) { return <ReactFlowProvider><FocusCanvas {...props} /></ReactFlowProvider>; }
