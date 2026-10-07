"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ReactFlow, Background, BackgroundVariant, Controls, ReactFlowProvider, useNodesState, useReactFlow, type Edge, type NodeChange } from "@xyflow/react";
import { Explorer } from "../components/Explorer";
import { Details } from "../components/Details";
import { MapNode, type CardFile, type MapFlowNode } from "../components/MapNode";
import { DependencyEdge } from "../components/DependencyEdge";
import { loadImpact, loadInitialData } from "../lib/api";
import { layoutVisibleGraph } from "../lib/layout";
import { buildExportUseIndex, buildVisibleGraph, directlyConnectedPackages, fileDependencyRoles, importCardLines, revealNode } from "../lib/presentation";
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
  onClearImpact: () => void;
  onResetPositions: () => void;
  onSelectNode: (id: string) => void;
  onSelectEdge: (id: string, supplyingFileID?: string, importingFileID?: string) => void;
  onClearSelection: () => void;
  onHoverNode: (id: string | null) => void;
  onToggleNode: (id: string) => void;
  onDragStop: (id: string, position: { x: number; y: number }) => void;
}

function GraphCanvas(props: GraphCanvasProps) {
  const { nodes, edges, hasPackages, importCount, impactActive, onClearImpact, onResetPositions, onSelectNode, onSelectEdge, onClearSelection, onHoverNode, onToggleNode, onDragStop } = props;
  const [flowNodes, setFlowNodes, onNodesChange] = useNodesState<MapFlowNode>(nodes);
  const draggedNodeID = useRef<string | null>(null);

  // Structural and selection changes come from the presentation layer. Drag frames
  // stay local to React Flow so the rest of the workspace does not redraw.
  useEffect(() => setFlowNodes(nodes), [nodes, setFlowNodes]);
  const onVisualNodeChange = useCallback((changes: NodeChange<MapFlowNode>[]) => {
    onNodesChange(changes.filter((change) => change.type === "position" || change.type === "select" || change.type === "dimensions"));
  }, [onNodesChange]);

  return <section className="graph-panel" aria-label="Dependency graph">
    <div className="graph-toolbar"><div><strong>Dependency map</strong><span>{nodes.length} visible nodes · {importCount} imports</span></div><div className="graph-toolbar-right"><button type="button" onClick={onResetPositions} title="Restore the current graph's starting layout">Reset positions</button>{impactActive && <button type="button" onClick={onClearImpact}>Clear impact</button>}<div className="graph-key"><span><i className="key-import" />imports</span><span><i className="key-export" />exports</span></div></div></div>
    <div className="graph-stage">
      {!hasPackages ? <div className="graph-empty">This saved scan has no package nodes to display.</div> : <ReactFlow<MapFlowNode, Edge>
        nodes={flowNodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onNodesChange={onVisualNodeChange}
        onNodeClick={(_, node) => onSelectNode(node.id)}
        onNodeMouseEnter={(_, node) => { if (draggedNodeID.current === null) onHoverNode(node.id); }}
        onNodeMouseLeave={() => { if (draggedNodeID.current === null) onHoverNode(null); }}
        onNodeDoubleClick={(_, node) => onToggleNode(node.id)}
        onNodeDragStart={(_, node) => { draggedNodeID.current = node.id; onHoverNode(node.id); }}
        onNodeDragStop={(_, node) => { draggedNodeID.current = null; onDragStop(node.id, node.position); onHoverNode(null); }}
        onEdgeClick={(_, edge) => onSelectEdge(typeof edge.data?.canonicalEdgeID === "string" ? edge.data.canonicalEdgeID : edge.id, typeof edge.data?.supplyingFileID === "string" ? edge.data.supplyingFileID : undefined, typeof edge.data?.importingFileID === "string" ? edge.data.importingFileID : undefined)}
        onPaneClick={onClearSelection}
        fitView fitViewOptions={{ padding: 0.18, minZoom: 0.35 }} minZoom={0.2} maxZoom={2}
        nodesConnectable={false} deleteKeyCode={null} edgesFocusable elementsSelectable
      ><Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid-dot)" /><Controls showInteractive={false} /></ReactFlow>}
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
  const [hoveredNodeID, setHoveredNodeID] = useState<string | null>(null);
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
    const files = new Map<string, CardFile[]>();
    for (const node of visibleGraph.nodes) if (node.kind === "FILE" && node.parentId) files.set(node.parentId, [...(files.get(node.parentId) ?? []), { file: node, declarations: [] }]);
    for (const groups of files.values()) for (const group of groups) group.declarations = visibleGraph.nodes.filter((node) => node.parentId === group.file.id);
    return files;
  }, [visibleGraph]);
  const cardGraph = useMemo(() => ({
    nodes: visibleGraph.nodes.filter((node) => node.kind === "PACKAGE"),
    edges: visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => ({ ...edge, source: edge.target, target: visibleNodes.get(edge.source)?.parentId ?? edge.source })),
  }), [visibleGraph, visibleNodes]);
  const exportUses = useMemo(() => data ? buildExportUseIndex(data.canonicalGraph) : new Map(), [data]);
  const fileRoles = useMemo(() => data ? fileDependencyRoles(data.canonicalGraph) : { importing: new Set<string>(), supplying: new Set<string>() }, [data]);
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
    const children = new Map<string, string[]>();
    for (const edge of data.canonicalGraph.edges) if (edge.kind === "CONTAINS") children.set(edge.from, [...(children.get(edge.from) ?? []), edge.to]);
    // Reserve each card's maximum list height once. Dropdowns change card size, not layout coordinates.
    const heights = new Map(collapsed.nodes.map((node) => {
      const files = children.get(node.id) ?? [];
      const rows = files.reduce((height, fileID) => height + 31 + (children.get(fileID)?.length ?? 0) * 26, 0);
      return [node.id, files.length ? 125 + Math.min(300, rows) : 72] as const;
    }));
    return layoutVisibleGraph({
      nodes: collapsed.nodes,
      edges: collapsed.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => ({ ...edge, source: edge.target, target: edge.source })),
    }, heights);
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
    setFocusID(id);
  }, [data, expandedPackages, expandedFiles, visibleKinds, visibleNodes]);

  useEffect(() => {
    if (!focusID || !visibleNodes.has(focusID)) return;
    let innerFrame = 0;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        const item = visibleNodes.get(focusID);
        const cardID = item?.kind === "PACKAGE" ? item.id : item?.kind === "FILE" ? item.parentId : visibleNodes.get(item?.parentId ?? "")?.parentId;
        if (cardID) void fitView({ nodes: [{ id: cardID }], padding: 0.8, duration: 0 });
        setFocusID(null);
      });
    });
    return () => { cancelAnimationFrame(outerFrame); cancelAnimationFrame(innerFrame); };
  }, [focusID, visibleNodes, fitView]);

  const showImpact = useCallback(async (id: string) => {
    const request = ++impactRequest.current;
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
  const focusedNodeID = hoveredNodeID ?? selectedNodeID;
  const focusedPackageID = useMemo(() => {
    let node = focusedNodeID ? visibleNodes.get(focusedNodeID) : undefined;
    while (node && node.kind !== "PACKAGE") node = visibleNodes.get(node.parentId ?? "");
    return node?.id ?? null;
  }, [focusedNodeID, visibleNodes]);
  const relatedPackages = useMemo(() => {
    if (focusedPackageID) return directlyConnectedPackages(cardGraph, focusedPackageID);
    const selected = selectedEdgeID ? importEdgesByID.get(selectedEdgeID) : undefined;
    return selected?.projectionEdge ? new Set([selected.projectionEdge.from, selected.projectionEdge.to]) : null;
  }, [focusedPackageID, selectedEdgeID, importEdgesByID, cardGraph]);
  const flowNodes = useMemo<MapFlowNode[]>(() => cardGraph.nodes.map((item) => {
    const layout = positions.get(item.id);
    return {
      id: item.id,
      type: "map",
      position: manualPositions[item.id] ?? { x: layout?.x ?? 0, y: layout?.y ?? 0 },
      draggable: true,
      data: { item, files: cardFiles.get(item.id) ?? [], selectedID: selectedNodeID, impactDistance: impactDistances.get(item.id), impactTarget: impact?.target === item.id, dimmed: relatedPackages !== null && !relatedPackages.has(item.id), importingFiles: fileRoles.importing, supplyingFiles: fileRoles.supplying, onToggle: toggleNode, onSelect: selectNode },
    };
  }), [cardGraph, cardFiles, positions, manualPositions, selectedNodeID, impactDistances, impact, relatedPackages, fileRoles, toggleNode, selectNode]);
  const flowEdges = useMemo<Edge[]>(() => cardLines.map((line) => {
    const item = importEdgesByID.get(line.canonicalEdgeID)!;
    const selected = selectedEdgeID === item.id;
    const focused = focusedPackageID !== null && (line.source === focusedPackageID || line.target === focusedPackageID);
    const impacted = item.projectionEdge && impactEdges.has(`${item.projectionEdge.from}->${item.projectionEdge.to}`);
    const opacity = selected ? 1 : focused ? .95 : relatedPackages !== null ? .06 : impacted ? .85 : .6;
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
      style: { strokeOpacity: opacity, strokeWidth: selected ? 1.3 : focused || impacted ? 1.05 : .75 },
    };
  }), [cardLines, importEdgesByID, selectedEdgeID, focusedPackageID, relatedPackages, impactEdges]);

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
      <Explorer canonicalGraph={canonicalGraph} visibleGraph={visibleGraph} selectedNodeID={selectedNodeID} search={search} visibleDeclarationKinds={visibleKinds} onSearch={setSearch} onSelectNode={selectNode} onToggle={toggleNode} onToggleKind={(kind) => { setVisibleKinds((current) => { const next = new Set(current); next.has(kind) ? next.delete(kind) : next.add(kind); return next; }); }} />
      <GraphCanvas
        nodes={flowNodes} edges={flowEdges} hasPackages={packageProjection.graph.nodes.length > 0}
        importCount={visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").length}
        impactActive={impact !== null}
        onClearImpact={() => { impactRequest.current++; setImpact(null); setImpactError(null); setImpactLoading(false); }}
        onResetPositions={() => setManualPositions({})}
        onSelectNode={(id) => { setSelectedNodeID(id); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null); }}
        onSelectEdge={(id, supplierID, importerID) => { setSelectedEdgeID(id); setSelectedSupplierID(supplierID ?? null); setSelectedImporterID(importerID ?? null); setSelectedNodeID(null); }}
        onClearSelection={() => { setSelectedNodeID(null); setSelectedEdgeID(null); setSelectedSupplierID(null); setSelectedImporterID(null); }}
        onHoverNode={setHoveredNodeID}
        onToggleNode={toggleNode}
        onDragStop={(id, position) => setManualPositions((current) => ({ ...current, [id]: position }))}
      />
      <Details selectedNode={selectedNode} selectedEdge={selectedEdge} selectedSupplierID={selectedSupplierID} selectedImporterID={selectedImporterID} canonicalGraph={canonicalGraph} packageProjection={packageProjection} impact={impact} impactLoading={impactLoading} impactError={impactError} onShowImpact={showImpact} onSelectNode={selectNode} />
    </main>
  </div>;
}

export default function Home() {
  return <ReactFlowProvider><MapApp /></ReactFlowProvider>;
}
