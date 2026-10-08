"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ReactFlow, Background, BackgroundVariant, Controls, ReactFlowProvider, useNodesState, useReactFlow, type Edge, type NodeChange } from "@xyflow/react";
import { Explorer } from "../components/Explorer";
import { Details } from "../components/Details";
import { MapNode, type CardFile, type MapFlowNode } from "../components/MapNode";
import { DependencyEdge } from "../components/DependencyEdge";
import { loadImpact, loadInitialData } from "../lib/api";
import { layoutVisibleGraph } from "../lib/layout";
import { buildExportUseIndex, buildVisibleGraph, directlyConnectedPackages, fileDependencyRoles, importCardLines, packageSourceCounts, revealNode } from "../lib/presentation";
import type { DeclarationKind, GraphNode, Impact, LoadedData } from "../lib/types";
import { declarationKinds } from "../lib/types";

const nodeTypes = { map: MapNode };
const edgeTypes = { dependency: DependencyEdge };
type Theme = "system" | "light" | "dark";

interface GraphCanvasProps {
  nodes: MapFlowNode[];
  edges: Edge[];
  hasPackages: boolean;
  importCount: number;
  impactActive: boolean;
  impactTargetID: string | null;
  impactTargetName: string | null;
  impactLoading: boolean;
  impactError: string | null;
  selectedPackageID: string | null;
  focusActive: boolean;
  connectedOnly: boolean;
  hiddenPackageCount: number;
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
  const { nodes, edges, hasPackages, importCount, impactActive, impactTargetID, impactTargetName, impactLoading, impactError, selectedPackageID, focusActive, connectedOnly, hiddenPackageCount, onClearImpact, onShowImpact, onResetPositions, onToggleConnectedOnly, onSelectNode, onSelectEdge, onClearSelection, onToggleNode, onDragStop } = props;
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState<MapFlowNode>(nodes);

  // Structural and selection changes come from the presentation layer. Drag frames
  // stay local to React Flow so the rest of the workspace does not redraw.
  useEffect(() => setFlowNodes(nodes), [nodes, setFlowNodes]);
  const onVisualNodeChange = useCallback((changes: NodeChange<MapFlowNode>[]) => {
    onNodesChange(changes.filter((change) => change.type === "position" || change.type === "dimensions" || (!impactActive && change.type === "select")));
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
          {connectedOnly ? `Show full map (${hiddenPackageCount} hidden)` : "Show connected only"}
        </button>}
        <div className="map-utility-actions">
          <button type="button" onClick={onResetPositions} title="Restore the current graph's starting layout">Reset positions</button>
        </div>
      </div>
      {impactError && <span className="map-action-error" role="alert">{impactError}</span>}
    </div>
    <div className="graph-stage">
      {!hasPackages ? <div className="graph-empty">This saved scan has no packages or modules to display.</div> : <ReactFlow<MapFlowNode, Edge>
        nodes={flowNodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onNodesChange={onVisualNodeChange}
        onNodeClick={(_, node) => onSelectNode(node.id)}
        onNodeDoubleClick={(_, node) => onToggleNode(node.id)}
        onNodeDragStop={(_, node) => onDragStop(node.id, node.position)}
        onEdgeClick={(_, edge) => onSelectEdge(typeof edge.data?.canonicalEdgeID === "string" ? edge.data.canonicalEdgeID : edge.id, typeof edge.data?.supplyingFileID === "string" ? edge.data.supplyingFileID : undefined, typeof edge.data?.importingFileID === "string" ? edge.data.importingFileID : undefined)}
        onPaneClick={onClearSelection}
        fitView fitViewOptions={{ padding: 0.08, minZoom: 0.01, maxZoom: 1.15 }} minZoom={0.01} maxZoom={2} proOptions={{ hideAttribution: true }}
        nodesConnectable={false} deleteKeyCode={null} edgesFocusable elementsSelectable
      >
        <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid-dot)" />
        <Controls showInteractive={false} fitViewOptions={{ padding: 0.08, minZoom: 0.01, maxZoom: 1.15 }} />
      </ReactFlow>}
    </div>
    <div className="graph-footer">
      <div className="graph-stats"><span>{nodes.length} visible nodes</span><span>{importCount} imports</span></div>
      {!selectedPackageID && !impactActive && !impactLoading && <span className="graph-guidance">Select a package to show impact</span>}
      <div className="graph-key" aria-label="Line colors"><span><i className="key-import" />imports</span><span><i className="key-export" />exports</span></div>
    </div>
  </section>;
}

function readTheme(): Theme {
  const stored = localStorage.getItem("nami-theme");
  return stored === "light" || stored === "dark" ? stored : "system";
}

function subscribeTheme(callback: () => void): () => void {
  window.addEventListener("nami-theme", callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener("nami-theme", callback);
    window.removeEventListener("storage", callback);
  };
}

function setThemeChoice(theme: Theme): void {
  localStorage.setItem("nami-theme", theme);
  window.dispatchEvent(new Event("nami-theme"));
}

function MapApp() {
  const [data, setData] = useState<LoadedData | null>(null);
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
  const [focusID, setFocusID] = useState<string | null>(null);
  const [manualPositions, setManualPositions] = useState<Record<string, { x: number; y: number }>>({});
  const [impact, setImpact] = useState<Impact | null>(null);
  const [impactLoading, setImpactLoading] = useState(false);
  const [impactError, setImpactError] = useState<string | null>(null);
  const [issuesOpen, setIssuesOpen] = useState(false);
  const theme = useSyncExternalStore(subscribeTheme, readTheme, () => "system");
  const impactRequest = useRef(0);
  const { fitView } = useReactFlow<MapFlowNode, Edge>();

  useEffect(() => {
    const controller = new AbortController();
    loadInitialData(controller.signal).then((result) => {
      setData(result);
      setLoading(false);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setLoadError(error instanceof Error ? error.message : "Unable to connect to the local nami API.");
      setLoading(false);
    });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const visibleGraph = useMemo(() => data ? buildVisibleGraph({
    canonicalGraph: data.canonicalGraph,
    packageProjection: data.packageProjection,
    expandedPackages,
    expandedFiles,
    visibleDeclarationKinds: visibleKinds,
  }) : { nodes: [], edges: [] }, [data, expandedPackages, expandedFiles, visibleKinds]);
  const visibleNodes = useMemo(() => new Map(visibleGraph.nodes.map((node) => [node.id, node])), [visibleGraph]);
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
      if (node.kind !== "FILE") continue;
      if (!node.parentId) {
        files.set(node.id, [{ file: { ...node, expanded: true, childCount: 0 }, declarations: children.get(node.id) ?? [] }]);
        continue;
      }
      const group = files.get(node.parentId) ?? [];
      group.push({ file: node, declarations: children.get(node.id) ?? [] });
      files.set(node.parentId, group);
    }
    return files;
  }, [visibleGraph]);
  const cardGraph = useMemo(() => ({
    nodes: visibleGraph.nodes.filter((node) => node.kind === "PACKAGE" || (node.kind === "FILE" && !node.parentId)),
    edges: visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => ({ ...edge, source: edge.target, target: visibleNodes.get(edge.source)?.parentId ?? edge.source })),
  }), [visibleGraph, visibleNodes]);
  const exportUses = useMemo(() => data ? buildExportUseIndex(data.canonicalGraph) : new Map(), [data]);
  const fileRoles = useMemo(() => data ? fileDependencyRoles(data.canonicalGraph) : { importing: new Set<string>(), supplying: new Set<string>() }, [data]);
  const packageCounts = useMemo(() => data ? packageSourceCounts(data.canonicalGraph) : new Map(), [data]);
  const cardLines = useMemo(() => visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").flatMap((edge) => importCardLines(edge, visibleNodes, exportUses)), [visibleGraph, visibleNodes, exportUses]);
  const importEdgesByID = useMemo(() => new Map(visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => [edge.id, edge])), [visibleGraph]);
  const positioned = useMemo(() => {
    if (!data) return [];
    const collapsed = buildVisibleGraph({
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
  const selectedNode = useMemo<GraphNode | null>(() => data?.canonicalGraph.nodes.find((node) => node.id === selectedNodeID) ?? null, [data, selectedNodeID]);
  const selectedEdge = useMemo(() => visibleGraph.edges.find((edge) => edge.id === selectedEdgeID) ?? null, [visibleGraph, selectedEdgeID]);

  const toggleNode = useCallback((id: string) => {
    const item = visibleNodes.get(id);
    if (!item || item.childCount === 0) return;
    if (item.kind === "PACKAGE") setExpandedPackages((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
    if (item.kind === "FILE") setExpandedFiles((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
  }, [visibleNodes]);

  const selectNode = useCallback((id: string) => {
    if (!data) return;
    if (!visibleNodes.has(id)) {
      const revealed = revealNode(data.canonicalGraph, id, { expandedPackages, expandedFiles, visibleDeclarationKinds: visibleKinds });
      setExpandedPackages(revealed.expandedPackages);
      setExpandedFiles(revealed.expandedFiles);
      setVisibleKinds(revealed.visibleDeclarationKinds);
    }
    setSelectedNodeID(id);
    setSelectedEdgeID(null);
    setSelectedSupplierID(null);
    setSelectedImporterID(null);
    setImpactError(null);
    setFocusID(id);
  }, [data, expandedPackages, expandedFiles, visibleKinds, visibleNodes]);

  useEffect(() => {
    if (!focusID || !visibleNodes.has(focusID)) return;
    let innerFrame = 0;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const item = visibleNodes.get(focusID);
        const cardID = item?.kind === "PACKAGE" ? item.id : item?.kind === "FILE" ? item.parentId ?? item.id : (visibleNodes.get(item?.parentId ?? "")?.parentId ?? item?.parentId);
        if (cardID) void fitView({ nodes: [{ id: cardID }], padding: 0.8, duration: 0 });
        setFocusID(null);
      });
    });
    return () => { cancelAnimationFrame(outerFrame); cancelAnimationFrame(innerFrame); };
  }, [focusID, visibleNodes, fitView]);

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
  const focusedPackageID = useMemo(() => {
    if (impact) return null;
    let node = selectedNodeID ? visibleNodes.get(selectedNodeID) : undefined;
    while (node?.parentId && node.kind !== "PACKAGE") node = visibleNodes.get(node.parentId ?? "");
    return node?.id ?? null;
  }, [impact, selectedNodeID, visibleNodes]);
  const relatedPackages = useMemo(() => {
    if (impact) return null;
    if (focusedPackageID) return directlyConnectedPackages(cardGraph, focusedPackageID);
    const selected = selectedEdgeID ? importEdgesByID.get(selectedEdgeID) : undefined;
    return selected?.projectionEdge ? new Set([selected.projectionEdge.from, selected.projectionEdge.to]) : null;
  }, [impact, focusedPackageID, selectedEdgeID, importEdgesByID, cardGraph]);
  const shownPackageIDs = connectedOnly ? relatedPackages : null;
  const shownPackages = useMemo(() => shownPackageIDs ? cardGraph.nodes.filter((item) => shownPackageIDs.has(item.id)) : cardGraph.nodes, [cardGraph.nodes, shownPackageIDs]);
  const shownLines = useMemo(() => shownPackageIDs ? cardLines.filter((line) => shownPackageIDs.has(line.source) && shownPackageIDs.has(line.target)) : cardLines, [cardLines, shownPackageIDs]);
  const flowNodes = useMemo<MapFlowNode[]>(() => shownPackages.map((item) => {
    const layout = positions.get(item.id);
    return {
      id: item.id,
      type: "map",
      position: manualPositions[item.id] ?? { x: layout?.x ?? 0, y: layout?.y ?? 0 },
      zIndex: item.expanded ? 1 : 0,
      draggable: true,
      selected: !impact && selectedNodeID === item.id,
      data: { item, files: cardFiles.get(item.id) ?? [], selectedID: impact ? null : selectedNodeID, selectedSupplierID: impact ? null : selectedSupplierID, selectedImporterID: impact ? null : selectedImporterID, impactDistance: impactDistances.get(item.id), impactTarget: impact?.target === item.id, dimmed: relatedPackages !== null && !relatedPackages.has(item.id), importingFiles: fileRoles.importing, supplyingFiles: fileRoles.supplying, sourceCounts: item.kind === "FILE" ? { imports: item.import_count, exports: item.export_count } : packageCounts.get(item.id), onToggle: toggleNode, onSelect: selectNode },
    };
  }), [shownPackages, cardFiles, positions, manualPositions, selectedNodeID, selectedSupplierID, selectedImporterID, impactDistances, impact, relatedPackages, fileRoles, packageCounts, toggleNode, selectNode]);
  const flowEdges = useMemo<Edge[]>(() => shownLines.map((line) => {
    const item = importEdgesByID.get(line.canonicalEdgeID)!;
    const selected = !impact && selectedEdgeID === item.id;
    const focused = focusedPackageID !== null && (line.source === focusedPackageID || line.target === focusedPackageID);
    const impacted = item.projectionEdge && impactEdges.has(`${item.projectionEdge.from}->${item.projectionEdge.to}`);
    const opacity = selected ? 1 : focused ? 1 : relatedPackages !== null ? .18 : impacted ? 1 : impact ? .4 : .78;
    return {
      id: line.id,
      source: line.source,
      sourceHandle: line.sourceHandle,
      target: line.target,
      targetHandle: line.targetHandle,
      data: { canonicalEdgeID: line.canonicalEdgeID, supplyingFileID: line.supplyingFileID, importingFileID: line.importingFileID },
      interactionWidth: 8,
      type: "dependency",
      selectable: item.kind === "IMPORTS",
      style: { strokeOpacity: opacity, strokeWidth: selected ? 1.6 : focused || impacted ? 1.4 : .95 },
    };
  }), [shownLines, importEdgesByID, selectedEdgeID, focusedPackageID, relatedPackages, impact, impactEdges]);

  if (loading) return <div className="startup-screen"><p>Loading saved graph…</p></div>;
  if (loadError || !data) return <div className="startup-screen error-screen"><h1>Unable to connect to the local nami API.</h1><p>{loadError}</p><span>Start it with</span><code>nami serve &lt;directory&gt; &lt;scan-id&gt;</code></div>;

  const { scan, canonicalGraph, packageProjection } = data;
  const hasGaps = scan.status !== "complete";
  return <div className="app-shell">
    <header className="topbar">
      <div className="brand">nami</div>
      <div className="topbar-repo" title={scan.root}><span className="topbar-caption">Repository</span><strong>{scan.root.split(/[\\/]/).filter(Boolean).at(-1) || scan.root}</strong><code>{scan.root}</code></div>
      <div className="topbar-scan"><span className="topbar-caption">Scan</span><code title={scan.id}>{scan.id.slice(0, 12)}</code></div>
      <button type="button" className={`topbar-status ${hasGaps ? "is-partial" : ""}`} onClick={() => setIssuesOpen((open) => !open)} aria-expanded={issuesOpen} aria-controls="coverage-details"><span className="status-dot" />{hasGaps ? "Partial analysis" : "Complete analysis"}</button>
      <label className="theme-control"><span>Theme</span><select value={theme} onChange={(event) => setThemeChoice(event.target.value as Theme)} aria-label="Theme"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
    </header>

    {issuesOpen && <div className="coverage-area" id="coverage-details">
      <div className="coverage-summary"><h2>{hasGaps ? "Partial analysis" : "Complete analysis"}</h2><p>{hasGaps ? "Some files or relationships could not be analyzed." : "No analysis gaps were reported."}</p></div>
      <div className="coverage-counts"><div><strong>{scan.coverage.files_failed}</strong><span>failed files</span></div><div><strong>{scan.coverage.files_skipped}</strong><span>skipped files</span></div><div><strong>{scan.coverage.unresolved_imports}</strong><span>unresolved imports</span></div></div>
      <div className="issues-panel"><h3>Known issues <span>{scan.issues.length}</span></h3>{scan.issues.length === 0 ? <p>No issue details were stored.</p> : <ul>{scan.issues.map((issue, index) => <li key={`${issue.kind}:${issue.path}:${index}`}><code>{issue.path}</code><span>{issue.kind.replaceAll("_", " ").toLowerCase()}</span><p>{issue.reason}</p></li>)}</ul>}</div>
    </div>}

    <main className="workspace">
      <Explorer canonicalGraph={canonicalGraph} visibleGraph={visibleGraph} selectedNodeID={impact ? null : selectedNodeID} search={search} visibleDeclarationKinds={visibleKinds} onSearch={setSearch} onSelectNode={selectNode} onToggle={toggleNode} onToggleKind={(kind) => { setVisibleKinds((current) => { const next = new Set(current); next.has(kind) ? next.delete(kind) : next.add(kind); return next; }); }} />
      <GraphCanvas
        nodes={flowNodes} edges={flowEdges} hasPackages={cardGraph.nodes.length > 0}
        importCount={new Set(shownLines.map((line) => line.canonicalEdgeID)).size}
        impactActive={impact !== null}
        impactTargetID={impact?.target ?? null}
        impactTargetName={impact ? canonicalGraph.nodes.find((node) => node.id === impact.target)?.path ?? null : null}
        impactLoading={impactLoading}
        impactError={impactError}
        selectedPackageID={selectedNode?.kind === "PACKAGE" ? selectedNode.id : null}
        focusActive={selectedNodeID !== null || selectedEdgeID !== null}
        connectedOnly={connectedOnly && shownPackageIDs !== null}
        hiddenPackageCount={cardGraph.nodes.length - shownPackages.length}
        onClearImpact={() => { impactRequest.current++; setImpact(null); setImpactError(null); setImpactLoading(false); }}
        onShowImpact={showImpact}
        onResetPositions={() => setManualPositions({})}
        onToggleConnectedOnly={() => setConnectedOnly((current) => !current)}
        onSelectNode={(id) => { setSelectedNodeID(id); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null); setImpactError(null); }}
        onSelectEdge={(id, supplierID, importerID) => { setSelectedEdgeID(id); setSelectedSupplierID(supplierID ?? null); setSelectedImporterID(importerID ?? null); setSelectedNodeID(null); setImpactError(null); }}
        onClearSelection={() => { setSelectedNodeID(null); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null); setConnectedOnly(false); }}
        onToggleNode={toggleNode}
        onDragStop={(id, position) => setManualPositions((current) => ({ ...current, [id]: position }))}
      />
      <Details selectedNode={selectedNode} selectedEdge={selectedEdge} selectedSupplierID={selectedSupplierID} selectedImporterID={selectedImporterID} canonicalGraph={canonicalGraph} packageProjection={packageProjection} impact={impact} onSelectNode={selectNode} />
    </main>
  </div>;
}

export default function Home() {
  return <ReactFlowProvider><MapApp /></ReactFlowProvider>;
}
