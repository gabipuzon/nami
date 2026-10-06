"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ReactFlow, Background, BackgroundVariant, Controls, MarkerType, ReactFlowProvider, useReactFlow, type Edge, type NodeChange } from "@xyflow/react";
import { Explorer } from "../components/Explorer";
import { Details } from "../components/Details";
import { MapNode, type MapFlowNode } from "../components/MapNode";
import { loadImpact, loadInitialData } from "../lib/api";
import { layoutVisibleGraph } from "../lib/layout";
import { buildVisibleGraph, revealNode } from "../lib/presentation";
import type { DeclarationKind, GraphNode, Impact, LoadedData } from "../lib/types";
import { declarationKinds } from "../lib/types";

const nodeTypes = { map: MapNode };
type Theme = "system" | "light" | "dark";

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
      setLoadError(error instanceof Error ? error.message : "Unable to connect to the local Nami API.");
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
  const positioned = useMemo(() => layoutVisibleGraph(visibleGraph), [visibleGraph]);
  const positions = useMemo(() => new Map(positioned.map((item) => [item.id, item])), [positioned]);
  const visibleNodes = useMemo(() => new Map(visibleGraph.nodes.map((node) => [node.id, node])), [visibleGraph]);
  const selectedNode = useMemo<GraphNode | null>(() => data?.canonicalGraph.nodes.find((node) => node.id === selectedNodeID) ?? null, [data, selectedNodeID]);
  const selectedEdge = useMemo(() => visibleGraph.edges.find((edge) => edge.id === selectedEdgeID) ?? null, [visibleGraph, selectedEdgeID]);

  const toggleNode = useCallback((id: string) => {
    const item = visibleNodes.get(id);
    if (!item || item.childCount === 0) return;
    if (item.kind === "PACKAGE") setExpandedPackages((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
    if (item.kind === "FILE") setExpandedFiles((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
    setManualPositions({});
  }, [visibleNodes]);

  const selectNode = useCallback((id: string) => {
    if (!data) return;
    if (!visibleNodes.has(id)) {
      const revealed = revealNode(data.canonicalGraph, id, { expandedPackages, expandedFiles, visibleDeclarationKinds: visibleKinds });
      setExpandedPackages(revealed.expandedPackages);
      setExpandedFiles(revealed.expandedFiles);
      setVisibleKinds(revealed.visibleDeclarationKinds);
      setManualPositions({});
    }
    setSelectedNodeID(id);
    setSelectedEdgeID(null);
    setFocusID(id);
  }, [data, expandedPackages, expandedFiles, visibleKinds, visibleNodes]);

  useEffect(() => {
    if (!focusID || !visibleNodes.has(focusID)) return;
    let innerFrame = 0;
    const outerFrame = requestAnimationFrame(() => {
      innerFrame = requestAnimationFrame(() => {
        void fitView({ nodes: [{ id: focusID }], padding: 0.8, duration: 0 });
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
  const flowNodes = useMemo<MapFlowNode[]>(() => visibleGraph.nodes.map((item) => {
    const layout = positions.get(item.id);
    return {
      id: item.id,
      type: "map",
      position: manualPositions[item.id] ?? { x: layout?.x ?? 0, y: layout?.y ?? 0 },
      draggable: true,
      data: { item, selected: selectedNodeID === item.id, impactDistance: impactDistances.get(item.id), impactTarget: impact?.target === item.id, onToggle: toggleNode },
    };
  }), [visibleGraph, positions, manualPositions, selectedNodeID, impactDistances, impact, toggleNode]);
  const flowEdges = useMemo<Edge[]>(() => visibleGraph.edges.map((item) => {
    const selected = selectedEdgeID === item.id;
    const outgoing = selectedNodeID === item.source || (selectedNodeID !== null && item.projectionEdge?.from === selectedNodeID);
    const incoming = selectedNodeID === item.target;
    const impacted = item.projectionEdge && impactEdges.has(`${item.projectionEdge.from}->${item.projectionEdge.to}`);
    const color = item.kind === "CONTAINS" ? "var(--containment)" : impacted ? "var(--impact)" : selected || outgoing ? "var(--import)" : incoming ? "var(--incoming)" : "var(--import-muted)";
    return {
      id: item.id,
      source: item.source,
      target: item.target,
      type: item.kind === "CONTAINS" ? "straight" : "smoothstep",
      selectable: item.kind === "IMPORTS",
      markerEnd: item.kind === "IMPORTS" ? { type: MarkerType.ArrowClosed, color } : undefined,
      style: { stroke: color, strokeWidth: item.kind === "CONTAINS" ? 1 : selected || outgoing || incoming || impacted ? 2.3 : 1.7, strokeDasharray: item.kind === "CONTAINS" ? "3 5" : undefined },
    };
  }), [visibleGraph, selectedEdgeID, selectedNodeID, impactEdges]);

  const onNodesChange = useCallback((changes: NodeChange<MapFlowNode>[]) => {
    const moved = changes.filter((change) => change.type === "position" && change.position);
    if (moved.length === 0) return;
    setManualPositions((current) => {
      const next = { ...current };
      for (const change of moved) if (change.type === "position" && change.position) next[change.id] = change.position;
      return next;
    });
  }, []);

  if (loading) return <div className="startup-screen"><div className="brand-mark">N</div><p>Loading saved graph…</p></div>;
  if (loadError || !data) return <div className="startup-screen error-screen"><div className="brand-mark">N</div><h1>Unable to connect to the local Nami API.</h1><p>{loadError}</p><span>Start it with</span><code>nami serve &lt;directory&gt; &lt;scan-id&gt;</code></div>;

  const { scan, canonicalGraph, packageProjection } = data;
  const hasGaps = scan.status !== "complete";
  return <div className="app-shell">
    <header className="topbar">
      <div className="brand"><span className="brand-mark">N</span><span>Nami</span></div>
      <div className="topbar-repo" title={scan.root}><span className="topbar-caption">Repository</span><strong>{scan.root.split(/[\\/]/).filter(Boolean).at(-1) || scan.root}</strong><code>{scan.root}</code></div>
      <div className="topbar-scan"><span className="topbar-caption">Scan</span><code title={scan.id}>{scan.id.slice(0, 12)}</code></div>
      <div className={`topbar-status ${hasGaps ? "is-partial" : ""}`}><span className="status-dot" />{hasGaps ? "Partial analysis" : "Complete analysis"}</div>
      <label className="theme-control"><span>Theme</span><select value={theme} onChange={(event) => setThemeChoice(event.target.value as Theme)} aria-label="Theme"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
    </header>

    {hasGaps && <div className="coverage-area">
      <div className="coverage-banner"><span className="warning-mark" aria-hidden="true">!</span><span>Graph is partial. Some files or relationships could not be analyzed. <strong>{scan.coverage.files_failed} failed</strong>, <strong>{scan.coverage.files_skipped} skipped</strong>, <strong>{scan.coverage.unresolved_imports} unresolved imports</strong>.</span><button type="button" onClick={() => setIssuesOpen((open) => !open)} aria-expanded={issuesOpen}>{issuesOpen ? "Hide issues" : `View issues (${scan.issues.length})`}</button></div>
      {issuesOpen && <div className="issues-panel"><h2>Known analysis gaps</h2>{scan.issues.length === 0 ? <p>No issue details were stored.</p> : <ul>{scan.issues.map((issue, index) => <li key={`${issue.kind}:${issue.path}:${index}`}><code>{issue.path}</code><span>{issue.kind.replaceAll("_", " ").toLowerCase()}</span><p>{issue.reason}</p></li>)}</ul>}</div>}
    </div>}

    <main className="workspace">
      <Explorer canonicalGraph={canonicalGraph} visibleGraph={visibleGraph} selectedNodeID={selectedNodeID} search={search} visibleDeclarationKinds={visibleKinds} onSearch={setSearch} onSelectNode={selectNode} onToggle={toggleNode} onToggleKind={(kind) => { setVisibleKinds((current) => { const next = new Set(current); next.has(kind) ? next.delete(kind) : next.add(kind); return next; }); setManualPositions({}); }} />
      <section className="graph-panel" aria-label="Dependency graph">
        <div className="graph-toolbar"><div><strong>Dependency map</strong><span>{visibleGraph.nodes.length} visible nodes · {visibleGraph.edges.filter((edge) => edge.kind === "IMPORTS").length} imports</span></div><div className="graph-toolbar-right">{impact && <button type="button" onClick={() => { impactRequest.current++; setImpact(null); setImpactError(null); setImpactLoading(false); }}>Clear impact</button>}<div className="graph-key"><span><i className="key-import" />imports</span><span><i className="key-containment" />contains</span></div></div></div>
        <div className="graph-stage">
          {packageProjection.graph.nodes.length === 0 ? <div className="graph-empty">This saved scan has no package nodes to display.</div> : <ReactFlow<MapFlowNode, Edge>
            nodes={flowNodes} edges={flowEdges} nodeTypes={nodeTypes} onNodesChange={onNodesChange}
            onNodeClick={(_, node) => { setSelectedNodeID(node.id); setSelectedEdgeID(null); }}
            onNodeDoubleClick={(_, node) => toggleNode(node.id)}
            onEdgeClick={(_, edge) => { setSelectedEdgeID(edge.id); setSelectedNodeID(null); }}
            onPaneClick={() => { setSelectedNodeID(null); setSelectedEdgeID(null); }}
            fitView fitViewOptions={{ padding: 0.18, minZoom: 0.35 }} minZoom={0.2} maxZoom={2}
            nodesConnectable={false} edgesFocusable elementsSelectable
          ><Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid-dot)" /><Controls showInteractive={false} /></ReactFlow>}
        </div>
      </section>
      <Details selectedNode={selectedNode} selectedEdge={selectedEdge} canonicalGraph={canonicalGraph} packageProjection={packageProjection} impact={impact} impactLoading={impactLoading} impactError={impactError} onShowImpact={showImpact} onSelectNode={selectNode} />
    </main>
  </div>;
}

export default function Home() {
  return <ReactFlowProvider><MapApp /></ReactFlowProvider>;
}
