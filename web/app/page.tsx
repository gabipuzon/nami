"use client";

import { recordPerformance, setPerformanceCount } from "../lib/performance";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ReactFlow, Background, BackgroundVariant, Controls, ReactFlowProvider, useNodesState, useStore, type ReactFlowState, type Edge, type NodeChange } from "@xyflow/react";
import { Explorer } from "../components/Explorer";
import { Details } from "../components/Details";
import { MapNode, type CardFile, type MapFlowNode } from "../components/MapNode";
import { DependencyEdge } from "../components/DependencyEdge";
import { FocusView } from "../components/FocusView";
import { PagedList } from "../components/PagedList";
import { bundleRoutes, sameWindow, type RowWindow, type RouteBundle } from "../lib/bundles";
import { Settings } from "../components/Settings";
import { usePreferences } from "../lib/preferences";
import { mergeDetail } from "../lib/loading";
import { loadImpact, loadInitialData, pinSnapshot, rescan, loadPackageDetail, loadDeclarations, loadInspection } from "../lib/api";
import { buildNodePresentations, type NodePresentation } from "../lib/languagePresentation";
import { getGraphIndex } from "../lib/graphIndex";
import { edgesWithRegisteredHandles, equalHandleSnapshots, synchronizeNodes } from "../lib/flowState";
import { layoutVisibleGraph } from "../lib/layout";
import { buildExportUseIndex, buildVisibleGraph, directlyConnectedPackages, fileDependencyRoles, importCardLines, packageSourceCounts, revealNode } from "../lib/presentation";
import type { DeclarationKind, GraphNode, Impact, LoadedData } from "../lib/types";
import type { VisibleEdge } from "../lib/presentation";
import { declarationKinds } from "../lib/types";

const nodeTypes = { map: MapNode };
const edgeTypes = { dependency: DependencyEdge };
const emptyCardFiles: CardFile[] = [];
const fitViewOptions = { padding: 0.08, minZoom: 0.01, maxZoom: 1.15 };
const proOptions = { hideAttribution: true };
const selectHandleSnapshot = (state: ReactFlowState) => [...state.nodeLookup].map(([id, node]) => [id, node.internals.handleBounds] as const);

interface GraphCanvasProps {
  nodes: MapFlowNode[];
  edges: Edge[];
  hasCards: boolean;
  showGrid: boolean;
  importCount: number;
  impactActive: boolean;
  impactTargetID: string | null;
  impactTargetName: string | null;
  impactLoading: boolean;
  impactError: string | null;
  selectedPackageID: string | null;
  focusActive: boolean;
  connectedOnly: boolean;
  hiddenCardCount: number;
  onClearImpact: () => void;
  onShowImpact: (id: string) => void;
  onResetPositions: () => void;
  onToggleConnectedOnly: () => void;
  onSelectNode: (id: string) => void;
  onSelectEdge: (id: string, supplyingFileID?: string, importingFileID?: string) => void;
  onClearSelection: () => void;
  onToggleNode: (id: string) => void;
  onDragStop: (id: string, position: { x: number; y: number }) => void;
}

function GraphCanvas(props: GraphCanvasProps) {
  recordPerformance("render.GraphCanvas");
  const { nodes, edges, hasCards, showGrid, importCount, impactActive, impactTargetID, impactTargetName, impactLoading, impactError, selectedPackageID, focusActive, connectedOnly, hiddenCardCount, onClearImpact, onShowImpact, onResetPositions, onToggleConnectedOnly, onSelectNode, onSelectEdge, onClearSelection, onToggleNode, onDragStop } = props;
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState<MapFlowNode>(nodes);
  useEffect(() => { setPerformanceCount("routes.overview", edges.length); return () => setPerformanceCount("routes.overview", undefined); }, [edges.length]);
  const handleSnapshot = useStore(selectHandleSnapshot, equalHandleSnapshots);
  const registeredEdges = useMemo(() => edgesWithRegisteredHandles(edges, handleSnapshot), [edges, handleSnapshot]);

  // Structural and selection changes come from the presentation layer. Drag frames
  // stay local to React Flow so the rest of the workspace does not redraw.
  const previousNodes = useRef(nodes);
  useEffect(() => {
    recordPerformance("flow.sync");
    const previous = previousNodes.current;
    setFlowNodes(current => synchronizeNodes(current, nodes, previous));
    previousNodes.current = nodes;
  }, [nodes, setFlowNodes]);
  const onVisualNodeChange = useCallback((changes: NodeChange<MapFlowNode>[]) => {
    recordPerformance("flow.changes");
    changes.forEach((change) => recordPerformance(`flow.change.${change.type}`));
    const visualChanges = changes.filter((change) => change.type === "position" || change.type === "dimensions" || (!impactActive && change.type === "select"));
    if (visualChanges.length) onNodesChange(visualChanges);
  }, [impactActive, onNodesChange]);

  return <section className={`graph-panel ${impactActive ? "is-impact-active" : ""}`} aria-label="Dependency graph">
    <div className="graph-head">
      <strong className="graph-title">Dependency map</strong>
      <div className="map-action-controls" role="group" aria-label="Map actions">
        {impactTargetID && <button className="map-impact-context" type="button" onClick={() => onSelectNode(impactTargetID)} title={`Inspect impact target: ${impactTargetName ?? impactTargetID}`} aria-label={`Inspect impact target ${impactTargetName ?? impactTargetID}`}>Impact: {impactTargetName ?? impactTargetID}</button>}
        <div className="map-impact-actions">
          {impactLoading ? <span className="map-impact-progress" role="status">Calculating impact…</span> : selectedPackageID && selectedPackageID !== impactTargetID && <button className="map-show-impact" type="button" onClick={() => onShowImpact(selectedPackageID)}>{impactActive ? "Update impact" : "Show impact"}</button>}
          {impactActive && <button className="map-clear-impact" type="button" onClick={onClearImpact}>Clear impact</button>}
        </div>
        {!impactActive && focusActive && <button className="map-connected-action" type="button" onClick={onToggleConnectedOnly} aria-pressed={connectedOnly}>
          {connectedOnly ? `Show full map (${hiddenCardCount} hidden)` : "Show connected only"}
        </button>}
        <div className="map-utility-actions">
          <button type="button" onClick={onResetPositions} title="Restore the current graph's starting layout">Reset positions</button>
        </div>
      </div>
      {impactError && <span className="map-action-error" role="alert">{impactError}</span>}
    </div>
    <div className="graph-stage">
      {!hasCards ? <div className="graph-empty">This saved scan has no packages or modules to display.</div> : <ReactFlow<MapFlowNode, Edge>
        nodes={flowNodes} edges={registeredEdges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onNodesChange={onVisualNodeChange}
        onNodeClick={(_, node) => onSelectNode(node.id)}
        onNodeDoubleClick={(_, node) => onToggleNode(node.id)}
        onNodeDragStop={(_, node) => onDragStop(node.id, node.position)}
        onEdgeClick={(_, edge) => onSelectEdge(edge.id, typeof edge.data?.supplyingFileID === "string" ? edge.data.supplyingFileID : undefined, typeof edge.data?.importingFileID === "string" ? edge.data.importingFileID : undefined)}
        onPaneClick={onClearSelection}
        fitView fitViewOptions={fitViewOptions} minZoom={0.01} maxZoom={2} proOptions={proOptions}
        nodesConnectable={false} deleteKeyCode={null} edgesFocusable elementsSelectable
      >
        {showGrid && <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid-dot)" />}
        <Controls showInteractive={false} fitViewOptions={fitViewOptions} />
      </ReactFlow>}
    </div>
    <div className="graph-footer">
      <div className="graph-stats"><span>{nodes.length} visible nodes</span><span>{importCount} known import groups</span><span>{edges.length} rendered bundles</span></div>
      {!selectedPackageID && !impactActive && !impactLoading && <span className="graph-guidance">Select a package to show impact</span>}
      <div className="graph-key" aria-label="Line colors"><span><i className="key-import" />imports</span><span><i className="key-export" />exports</span></div>
    </div>
  </section>;
}

function MapApp({ onSnapshot }: { onSnapshot: (id: string) => void }) {
  recordPerformance("render.MapApp");
  const [data, setData] = useState<LoadedData | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const detailRetry = useRef<{kind:"toggle"|"select"|"reveal";ids:string[]} | null>(null);
  const selectionController = useRef<AbortController | null>(null);
  const detailControllers = useRef(new Set<AbortController>());
  const mergeLoaded = useCallback((detail: import("../lib/types").GraphDetail) => setData(current => current ? mergeDetail(current,detail) : current), []);
  useEffect(() => () => {selectionController.current?.abort();detailControllers.current.forEach(c => c.abort());}, []);
  const [rescanning, setRescanning] = useState(false);
  const [rescanError, setRescanError] = useState<string | null>(null);

  const [loadAttempt, setLoadAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [expandedPackages, setExpandedPackages] = useState<Set<string>>(new Set());
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(new Set());
  const [visibleKinds, setVisibleKinds] = useState<Set<DeclarationKind>>(new Set(declarationKinds));
  const [selectedNodeID, setSelectedNodeID] = useState<string | null>(null);
  const [selectedEdgeID, setSelectedEdgeID] = useState<string | null>(null);
  const [selectedSupplierID, setSelectedSupplierID] = useState<string | null>(null);
  const [selectedImporterID, setSelectedImporterID] = useState<string | null>(null);
  const [connectedOnly, setConnectedOnly] = useState(false);
  const [search, setSearch] = useState("");
  const [focusTarget, setFocusTarget] = useState<{node: GraphNode; sequence: number} | null>(null);
  const [focusedEdge, setFocusedEdge] = useState<VisibleEdge | null>(null);
  const [selectedBundle, setSelectedBundle] = useState<RouteBundle | null>(null);
  const [rowWindows, setRowWindows] = useState<ReadonlyMap<string, RowWindow>>(new Map());
  const [overviewSelection, setOverviewSelection] = useState<{node: string | null; edge: string | null; supplier: string | null; importer: string | null; bundle: RouteBundle | null} | null>(null);
  const onWindow = useCallback((id: string, window: RowWindow) => setRowWindows(current => {
    if (sameWindow(current.get(id),window)) return current;
    const next = new Map(current); next.set(id,window); return next;
  }), []);
  const [manualPositions, setManualPositions] = useState<Record<string, { x: number; y: number }>>({});
  const [impact, setImpact] = useState<Impact | null>(null);
  const [impactLoading, setImpactLoading] = useState(false);
  const [impactError, setImpactError] = useState<string | null>(null);
  const [issuesOpen, setIssuesOpen] = useState(false);
  const { preferences, saveFailed } = usePreferences();
  const { theme, lineStyle, showGrid } = preferences;
  const impactRequest = useRef(0);

  useEffect(() => {
    const controller = new AbortController();
    loadInitialData(controller.signal).then((result) => {
      pinSnapshot(result.scan.id);
      setData(result);
      setLoading(false);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setLoadError(error instanceof Error ? error.message : "Unable to connect to the local nami API.");
      setLoading(false);
    });
    return () => controller.abort();
  }, [loadAttempt]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const visibleGraph = useMemo(() => data ? buildVisibleGraph({
    childCounts: data.childCounts,
    fileCounts: data.fileCounts,
    canonicalGraph: data.canonicalGraph,
    packageProjection: data.packageProjection,
    expandedPackages,
    expandedFiles,
    visibleDeclarationKinds: visibleKinds,
  }) : { nodes: [], edges: [] }, [data, expandedPackages, expandedFiles, visibleKinds]);
  const canonicalNodes = useMemo(() => data ? getGraphIndex(data.canonicalGraph).nodes : new Map<string, GraphNode>(), [data]);
  const visibleNodes = useMemo(() => new Map(visibleGraph.nodes.map((node) => [node.id, node])), [visibleGraph]);
  const presentations = useMemo(() => data ? buildNodePresentations(data.canonicalGraph, packageSourceCounts(data.canonicalGraph,data.fileCounts)) : new Map<string, NodePresentation>(), [data]);
  const cardFiles = useMemo(() => {
    const children = new Map<string, typeof visibleGraph.nodes>();
    for (const node of visibleGraph.nodes) {
      if (!node.parentId) continue;
      const siblings = children.get(node.parentId) ?? [];
      siblings.push(node);
      children.set(node.parentId, siblings);
    }
    const files = new Map<string, CardFile[]>();
    for (const node of visibleGraph.nodes) {
      if (presentations.get(node.id)?.expansion !== "source") continue;
      if (!node.parentId) {
        files.set(node.id, [{ file: { ...node, expanded: true, childCount: 0, presentation: presentations.get(node.id)! }, declarations: (children.get(node.id) ?? []).map((child) => ({ ...child, presentation: presentations.get(child.id)! })) }]);
        continue;
      }
      const group = files.get(node.parentId) ?? [];
      group.push({ file: { ...node, presentation: presentations.get(node.id)! }, declarations: (children.get(node.id) ?? []).map((child) => ({ ...child, presentation: presentations.get(child.id)! })) });
      files.set(node.parentId, group);
    }
    return files;
  }, [visibleGraph, presentations]);
  const cardGraph = useMemo(() => ({
    nodes: visibleGraph.nodes.filter((node) => presentations.get(node.id)?.primaryCard),
    edges: visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => ({ ...edge, source: edge.target, target: visibleNodes.get(edge.source)?.parentId ?? edge.source })),
  }), [visibleGraph, visibleNodes, presentations]);
  const exportUses = useMemo(() => data ? buildExportUseIndex(data.canonicalGraph) : new Map(), [data]);
  const fileRoles = useMemo(() => data ? fileDependencyRoles(data.canonicalGraph) : { importing: new Set<string>(), supplying: new Set<string>() }, [data]);
  const cardLines = useMemo(() => visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").flatMap((edge) => importCardLines(edge, visibleNodes, exportUses, canonicalNodes)), [visibleGraph, visibleNodes, exportUses, canonicalNodes]);
  const importEdgesByID = useMemo(() => new Map(visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => [edge.id, edge])), [visibleGraph]);
  const positioned = useMemo(() => {
    if (!data) return [];
    const collapsed = buildVisibleGraph({
      childCounts: data.childCounts,
    fileCounts: data.fileCounts,
    canonicalGraph: data.canonicalGraph,
      packageProjection: data.packageProjection,
      expandedPackages: new Set(),
      expandedFiles: new Set(),
      visibleDeclarationKinds: new Set(declarationKinds),
    });
    return layoutVisibleGraph({
      nodes: collapsed.nodes,
      edges: collapsed.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => ({ ...edge, source: edge.target, target: edge.source })),
    });
  }, [data]);
  const positions = useMemo(() => new Map(positioned.map((item) => [item.id, item])), [positioned]);
  const selectedNode = useMemo<GraphNode | null>(() => canonicalNodes.get(selectedNodeID ?? "") ?? null, [canonicalNodes, selectedNodeID]);
  const selectedEdge = useMemo(() => visibleGraph.edges.find((edge) => edge.id === selectedEdgeID) ?? null, [visibleGraph, selectedEdgeID]);

  const toggleNode = useCallback((id: string) => {
    const item = visibleNodes.get(id);
    if (!item) return;
    const expansion = presentations.get(id)?.expansion;
    const expanded = expansion === "container" ? expandedPackages : expandedFiles;
    const setter = expansion === "container" ? setExpandedPackages : setExpandedFiles;
    if (!expansion) return;
    if (expanded.has(id)) {setter(current => {const next = new Set(current);next.delete(id);return next;});return;}
    const controller = new AbortController();detailControllers.current.add(controller);
    setDetailLoading(true);setDetailError(null);
    const load = expansion === "container" ? loadPackageDetail(id,controller.signal) : loadDeclarations(id,controller.signal);
    load.then(detail => {if (controller.signal.aborted) return;mergeLoaded(detail);setter(current => new Set(current).add(id));}).catch((error: unknown) => {
      if (!controller.signal.aborted) {setDetailError(error instanceof Error ? error.message : "Unable to load detail.");detailRetry.current={kind:"toggle",ids:[id]};}
    }).finally(() => {detailControllers.current.delete(controller);if (!controller.signal.aborted) setDetailLoading(false);});
  }, [visibleNodes,presentations,expandedPackages,expandedFiles,mergeLoaded]);

  const selectNode = useCallback((id: string) => {
    setDetailError(null);
    selectionController.current?.abort();
    const controller = new AbortController();selectionController.current=controller;
    if (!canonicalNodes.has(id)) loadInspection(id,0,0,0,controller.signal).then(detail => {if (!controller.signal.aborted) mergeLoaded(detail);}).catch((error: unknown) => {
      if (!controller.signal.aborted) {setDetailError(error instanceof Error ? error.message : "Unable to inspect node.");detailRetry.current={kind:"select",ids:[id]};}
    });
    setSelectedNodeID(id); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null);
    setSelectedBundle(null); setFocusedEdge(null); setImpactError(null);
  }, [canonicalNodes,mergeLoaded]);
  const focusNode = useCallback((node: GraphNode) => {
    if (!focusTarget) setOverviewSelection({node:selectedNodeID,edge:selectedEdgeID,supplier:selectedSupplierID,importer:selectedImporterID,bundle:selectedBundle});
    impactRequest.current++;
    setImpactLoading(false);
    setFocusTarget(current => ({node,sequence:(current?.sequence ?? 0)+1}));
    setSelectedBundle(null); setFocusedEdge(null);
  }, [focusTarget,selectedNodeID,selectedEdgeID,selectedSupplierID,selectedImporterID,selectedBundle]);
  const returnToOverview = useCallback(() => {
    setFocusTarget(null); setFocusedEdge(null);
    const selection = overviewSelection;
    if (selection) {setSelectedNodeID(selection.node);setSelectedEdgeID(selection.edge);setSelectedSupplierID(selection.supplier);setSelectedImporterID(selection.importer);setSelectedBundle(selection.bundle);}
    setOverviewSelection(null);
  }, [overviewSelection]);
  const revealFiles = useCallback((ids: string[]) => {
    if (!data) return;
    const controller=new AbortController();detailControllers.current.add(controller);
    const work=async () => {
      let next=data;
      for (const id of ids) {
        const inspected=await loadInspection(id,0,0,0,controller.signal);
        next=mergeDetail(next,inspected);
        const parent=inspected.parent;
        if (parent && next.canonicalGraph.nodes.find(n => n.id===parent)?.kind==="PACKAGE") next=mergeDetail(next,await loadPackageDetail(parent,controller.signal));
      }
      if (controller.signal.aborted) return;
      setData(next);returnToOverview();
      let state={expandedPackages:new Set(expandedPackages),expandedFiles:new Set(expandedFiles),visibleDeclarationKinds:new Set(visibleKinds)};
      for (const id of ids) state=revealNode(next.canonicalGraph,id,state);
      setExpandedPackages(state.expandedPackages);setExpandedFiles(state.expandedFiles);setVisibleKinds(state.visibleDeclarationKinds);
      setSelectedSupplierID(ids[1]??null);setSelectedImporterID(ids[0]??null);setSelectedNodeID(ids[0]??null);setSelectedEdgeID(null);setSelectedBundle(null);
    };
    void work().catch((error: unknown) => {if (!controller.signal.aborted) {setDetailError(error instanceof Error ? error.message : "Unable to reveal files.");detailRetry.current={kind:"reveal",ids};}}).finally(() => detailControllers.current.delete(controller));
  }, [data,expandedPackages,expandedFiles,visibleKinds,returnToOverview]);

  const showImpact = useCallback(async (id: string) => {
    const request = ++impactRequest.current;
    setConnectedOnly(false);
    setImpactLoading(true);
    setImpactError(null);
    try {
      const result = await loadImpact(id);
      if (request === impactRequest.current) setImpact(result);
    } catch (error) {
      if (request === impactRequest.current) setImpactError(error instanceof Error ? error.message : "Unable to load impact.");
    } finally {
      if (request === impactRequest.current) setImpactLoading(false);
    }
  }, []);

  const impactDistances = useMemo(() => new Map(impact?.affected.map((item) => [item.id, item.distance]) ?? []), [impact]);
  const impactEdges = useMemo(() => new Set(impact?.graph.edges.map((edge) => `${edge.from}->${edge.to}`) ?? []), [impact]);
  const displayNodeID = focusTarget ? overviewSelection?.node ?? null : selectedNodeID;
  const displayEdgeID = focusTarget ? overviewSelection?.edge ?? null : selectedEdgeID;
  const displaySupplierID = focusTarget ? overviewSelection?.supplier ?? null : selectedSupplierID;
  const displayImporterID = focusTarget ? overviewSelection?.importer ?? null : selectedImporterID;
  const displayBundle = focusTarget ? overviewSelection?.bundle ?? null : selectedBundle;
  const focusedCardID = useMemo(() => {
    if (impact) return null;
    let node = displayNodeID ? visibleNodes.get(displayNodeID) : undefined;
    while (node && !presentations.get(node.id)?.primaryCard) node = visibleNodes.get(node.parentId ?? "");
    return node?.id ?? null;
  }, [impact, displayNodeID, visibleNodes, presentations]);
  const relatedCards = useMemo(() => {
    if (impact) return null;
    if (focusedCardID) return directlyConnectedPackages(cardGraph, focusedCardID);
    const selected = displayEdgeID ? importEdgesByID.get(displayEdgeID) : undefined;
    return selected?.projectionEdge ? new Set([selected.projectionEdge.from, selected.projectionEdge.to]) : null;
  }, [impact, focusedCardID, displayEdgeID, importEdgesByID, cardGraph]);
  const shownCardIDs = connectedOnly ? relatedCards : null;
  const shownCards = useMemo(() => shownCardIDs ? cardGraph.nodes.filter((item) => shownCardIDs.has(item.id)) : cardGraph.nodes, [cardGraph.nodes, shownCardIDs]);
  const shownLines = useMemo(() => shownCardIDs ? cardLines.filter((line) => shownCardIDs.has(line.source) && shownCardIDs.has(line.target)) : cardLines, [cardLines, shownCardIDs]);
  const selectionCards = useMemo(() => {
    const cardFor = (id: string | null) => {
      let node = id ? visibleNodes.get(id) : undefined;
      while (node && !presentations.get(node.id)?.primaryCard) node = visibleNodes.get(node.parentId ?? "");
      return node?.id;
    };
    return { selected: cardFor(displayNodeID), supplier: cardFor(displaySupplierID), importer: cardFor(displayImporterID) };
  }, [visibleNodes, presentations, displayNodeID, displaySupplierID, displayImporterID]);
  const bundles = useMemo(() => bundleRoutes(shownLines, importEdgesByID, new Map([...rowWindows].filter(([id]) => visibleNodes.get(id)?.expanded))), [shownLines,importEdgesByID,rowWindows,visibleNodes]);
  const flowNodes = useMemo<MapFlowNode[]>(() => shownCards.map((item) => {
    const layout = positions.get(item.id);
    return {
      id: item.id,
      type: "map",
      position: manualPositions[item.id] ?? { x: layout?.x ?? 0, y: layout?.y ?? 0 },
      zIndex: item.expanded ? 1 : 0,
      draggable: true,
      selected: !impact && displayNodeID === item.id,
      data: { item, presentation: presentations.get(item.id)!, files: cardFiles.get(item.id) ?? emptyCardFiles, selectedID: !impact && selectionCards.selected === item.id ? displayNodeID : null, selectedSupplierID: !impact && selectionCards.supplier === item.id ? displaySupplierID : null, selectedImporterID: !impact && selectionCards.importer === item.id ? displayImporterID : null, impactDistance: impactDistances.get(item.id), impactTarget: impact?.target === item.id, dimmed: relatedCards !== null && !relatedCards.has(item.id), importingFiles: fileRoles.importing, supplyingFiles: fileRoles.supplying, onToggle: toggleNode, onSelect: selectNode, onWindow },
    };
  }), [shownCards, cardFiles, positions, manualPositions, displayNodeID, displaySupplierID, displayImporterID, impactDistances, impact, relatedCards, fileRoles, presentations, toggleNode, selectNode, selectionCards, onWindow]);
  const flowEdges = useMemo<Edge[]>(() => bundles.map((line) => {
    const item = importEdgesByID.get(line.canonicalEdgeID)!;
    const selected = !impact && displayBundle?.id === line.id;
    const focused = focusedCardID !== null && (line.source === focusedCardID || line.target === focusedCardID);
    const impacted = item.projectionEdge && impactEdges.has(`${item.projectionEdge.from}->${item.projectionEdge.to}`);
    const opacity = selected ? 1 : focused ? 1 : relatedCards !== null ? .18 : impacted ? 1 : impact ? .4 : .78;
    return {
      id: line.id,
      source: line.source,
      sourceHandle: line.sourceHandle,
      target: line.target,
      targetHandle: line.targetHandle,
      data: { lineStyle, canonicalEdgeID: line.canonicalEdgeID, supplyingFileID: line.supplyingFileID, importingFileID: line.importingFileID, bundleCount: line.members.length },
      interactionWidth: 8,
      type: "dependency",
      selectable: item.kind === "IMPORTS",
      style: { strokeOpacity: opacity, strokeWidth: selected ? 1.6 : focused || impacted ? 1.4 : .95 },
    };
  }), [bundles, importEdgesByID, displayBundle, focusedCardID, relatedCards, impact, impactEdges, lineStyle]);

  if (loading) return <div className="startup-screen"><p>Loading saved graph…</p></div>;
  if (loadError || !data) return <div className="startup-screen error-screen"><h1>Unable to connect to the local nami API.</h1><p>{loadError}</p><button onClick={() => {pinSnapshot();setLoading(true);setLoadError(null);setLoadAttempt(n=>n+1);}}>Retry</button><span>Start it with</span><code>nami serve &lt;directory&gt; &lt;scan-id&gt;</code></div>;

  const { scan, canonicalGraph, packageProjection } = data;
  const hasGaps = scan.status !== "complete";
  const exclusions = scan.exclusions ?? [];
  const coverageLabel = hasGaps ? "Partial analysis" : exclusions.length ? "Complete within scope" : "Complete analysis";
  return <div className="app-shell">
    <header className="topbar">
      <div className="brand">nami</div>
      <div className="topbar-repo" title={scan.root}><span className="topbar-caption">Repository</span><strong>{scan.root.split(/[\\/]/).filter(Boolean).at(-1) || scan.root}</strong><code>{scan.root}</code></div>
      <div className="topbar-scan"><span className="topbar-caption">Scan</span><code title={scan.id}>{scan.id.slice(0, 12)}</code></div>
      <button type="button" className={`topbar-status ${hasGaps ? "is-partial" : ""}`} onClick={() => setIssuesOpen((open) => !open)} aria-expanded={issuesOpen} aria-controls="coverage-details"><span className="status-dot" />{coverageLabel}</button>
      <time className="topbar-time" dateTime={scan.created_at} title={scan.created_at}>{new Date(scan.created_at).toLocaleString()}</time>
      <button className="topbar-rescan" type="button" disabled={rescanning} onClick={() => {
        setRescanning(true);setRescanError(null);
        rescan(scan.id).then(onSnapshot).catch((error: unknown) => {setRescanError(error instanceof Error ? error.message : "Rescan failed.");setRescanning(false);});
      }}>{rescanning ? "Scanning…" : rescanError ? "Retry rescan" : "Rescan"}</button>
      <Settings preferences={preferences} saveFailed={saveFailed} />
    </header>

    {detailLoading && <p role="status">Loading saved detail…</p>}
    {detailError && <p role="alert">{detailError} <button onClick={() => {setDetailError(null);const retry=detailRetry.current;if(retry?.kind==="toggle")toggleNode(retry.ids[0]);else if(retry?.kind==="select")selectNode(retry.ids[0]);else if(retry)revealFiles(retry.ids);}}>Retry</button></p>}
    {rescanError && <p role="alert" className="map-action-error">{rescanError}</p>}
    {issuesOpen && <div className="coverage-area" id="coverage-details">
      <div className="coverage-summary"><h2>{coverageLabel}</h2><p>{hasGaps ? "Some files or relationships could not be analyzed." : exclusions.length ? "No analysis gaps were reported within the included scan scope." : "No analysis gaps were reported."}</p></div>
      <div className="coverage-counts"><div><strong>{scan.coverage.files_failed}</strong><span>failed files</span></div><div><strong>{scan.coverage.files_skipped}</strong><span>skipped files</span></div><div><strong>{scan.coverage.unresolved_imports}</strong><span>unresolved imports</span></div></div>
      {exclusions.length > 0 && <div className="issues-panel"><h3>Intentional exclusions <span>{exclusions.length}</span></h3><p>Directories count once; their contents were not scanned. Coverage describes the included scope.</p><ul><PagedList items={exclusions} render={(excluded,index) => <li key={`${excluded.path}:${index}`}><code>{excluded.path}</code><p>{excluded.reason}</p></li>} /></ul></div>}
      <div className="issues-panel"><h3>Known issues <span>{scan.issues.length}</span></h3>{scan.issues.length === 0 ? <p>No issue details were stored.</p> : <ul><PagedList items={scan.issues} render={(issue, index) => <li key={`${issue.kind}:${issue.path}:${index}`}><code>{issue.path}</code><span>{issue.kind.replaceAll("_", " ").toLowerCase()}</span><p>{issue.reason}</p></li>} /></ul>}</div>
    </div>}

    <main className="workspace">
      <Explorer key={scan.id} counts={data.counts} childCounts={data.childCounts} onMerge={mergeLoaded} presentations={presentations} canonicalGraph={canonicalGraph} visibleGraph={visibleGraph} selectedNodeID={impact && !focusTarget ? null : selectedNodeID} search={search} visibleDeclarationKinds={visibleKinds} onSearch={setSearch} onSelectNode={selectNode} onToggle={toggleNode} onToggleKind={(kind) => { setVisibleKinds((current) => { const next = new Set(current); next.has(kind) ? next.delete(kind) : next.add(kind); return next; }); }} />
      <div className="overview-canvas" style={{display:focusTarget ? "none" : undefined}}><GraphCanvas
        nodes={flowNodes} edges={flowEdges} hasCards={cardGraph.nodes.length > 0} showGrid={showGrid}
        importCount={new Set(shownLines.map((line) => line.canonicalEdgeID)).size}
        impactActive={impact !== null}
        impactTargetID={impact?.target ?? null}
        impactTargetName={impact ? canonicalNodes.get(impact.target)?.path ?? null : null}
        impactLoading={impactLoading}
        impactError={impactError}
        selectedPackageID={selectedNode?.kind === "PACKAGE" ? selectedNode.id : null}
        focusActive={selectedNodeID !== null || selectedEdgeID !== null}
        connectedOnly={connectedOnly && shownCardIDs !== null}
        hiddenCardCount={cardGraph.nodes.length - shownCards.length}
        onClearImpact={() => { impactRequest.current++; setImpact(null); setImpactError(null); setImpactLoading(false); }}
        onShowImpact={showImpact}
        onResetPositions={() => setManualPositions({})}
        onToggleConnectedOnly={() => setConnectedOnly((current) => !current)}
        onSelectNode={selectNode}
        onSelectEdge={(id) => {const bundle = bundles.find(b => b.id === id);if (bundle) {setSelectedBundle(bundle);setSelectedEdgeID(bundle.canonicalEdgeID);setSelectedNodeID(null);setSelectedSupplierID(null);setSelectedImporterID(null);setImpactError(null);}}}
        onClearSelection={() => { setSelectedBundle(null); setSelectedNodeID(null); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null); setConnectedOnly(false); }}
        onToggleNode={toggleNode}
        onDragStop={(id, position) => setManualPositions((current) => ({ ...current, [id]: position }))}
      /></div>
      {focusTarget && <FocusView onGraph={graph => mergeLoaded({graph,package_projection:{graph:{nodes:[],edges:[]},evidence:[]},child_counts:{}})} target={focusTarget} data={data} presentations={presentations} selectedID={selectedNodeID} onInspect={selectNode} onEdge={edge => {setFocusedEdge(edge);setSelectedNodeID(null);}} onExit={returnToOverview} lineStyle={lineStyle} showGrid={showGrid} />}
      <Details key={`${scan.id}:${selectedNodeID ?? selectedEdgeID ?? selectedBundle?.id ?? "none"}`} onMerge={mergeLoaded} presentations={presentations} selectedNode={selectedNode} selectedEdge={focusedEdge ?? selectedEdge} bundle={selectedBundle} onRevealFiles={revealFiles} onFocus={focusNode} selectedSupplierID={selectedSupplierID} selectedImporterID={selectedImporterID} canonicalGraph={canonicalGraph} packageProjection={packageProjection} impact={focusTarget ? null : impact} onSelectNode={selectNode} />
    </main>
  </div>;
}

export default function Home() {
  const [session, setSession] = useState("initial");
  return <ReactFlowProvider key={session}><MapApp onSnapshot={id => {pinSnapshot(id);setSession(id);}} /></ReactFlowProvider>;
}
