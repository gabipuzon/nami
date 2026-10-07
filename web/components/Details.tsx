"use client";

import type { Graph, GraphEdge, GraphNode, Impact, PackageProjection } from "../lib/types";
import { isDeclarationKind, type VisibleEdge } from "../lib/presentation";

interface DetailsProps {
  selectedNode: GraphNode | null;
  selectedEdge: VisibleEdge | null;
  selectedSupplierID: string | null;
  selectedImporterID: string | null;
  canonicalGraph: Graph;
  packageProjection: PackageProjection;
  impact: Impact | null;
  impactLoading: boolean;
  impactError: string | null;
  onShowImpact: (id: string) => void;
  onSelectNode: (id: string) => void;
}

const nameFor = (id: string, nodes: ReadonlyMap<string, GraphNode>): string => nodes.get(id)?.name || id;

function FactList({ title, ids, nodes, onSelectNode }: {
  title: string;
  ids: string[];
  nodes: ReadonlyMap<string, GraphNode>;
  onSelectNode: (id: string) => void;
}) {
  return <section className="detail-section">
    <h3>{title} <span>{ids.length}</span></h3>
    {ids.length === 0 ? <p className="muted">None known</p> : <ul className="detail-list">{ids.map((id) =>
      <li key={id}><button type="button" onClick={() => onSelectNode(id)} title={id}>{nameFor(id, nodes)}</button></li>
    )}</ul>}
  </section>;
}

function EvidenceList({ sources, nodes }: { sources: GraphEdge[]; nodes: ReadonlyMap<string, GraphNode> }) {
  return <ul className="evidence-list">{sources.map((edge) =>
    <li key={`${edge.from}->${edge.to}`}><span>{nameFor(edge.from, nodes)}</span><span aria-hidden="true">→</span><span>{nameFor(edge.to, nodes)}</span></li>
  )}</ul>;
}

export function Details(props: DetailsProps) {
  const { selectedNode, selectedEdge, selectedSupplierID, selectedImporterID, canonicalGraph, packageProjection, impact, impactLoading, impactError, onShowImpact, onSelectNode } = props;
  const nodes = new Map(canonicalGraph.nodes.map((node) => [node.id, node]));
  const parent = new Map(canonicalGraph.edges.filter((edge) => edge.kind === "CONTAINS").map((edge) => [edge.to, edge.from]));
  const directChildren = (id: string) => canonicalGraph.edges.filter((edge) => edge.kind === "CONTAINS" && edge.from === id).map((edge) => edge.to).sort();
  const canonicalOutgoing = (id: string) => canonicalGraph.edges.filter((edge) => edge.kind === "IMPORTS" && edge.from === id).map((edge) => edge.to).sort();
  const canonicalIncoming = (id: string) => canonicalGraph.edges.filter((edge) => edge.kind === "IMPORTS" && edge.to === id).map((edge) => edge.from).sort();
  const packageOutgoing = (id: string) => packageProjection.graph.edges.filter((edge) => edge.from === id).map((edge) => edge.to).sort();
  const packageIncoming = (id: string) => packageProjection.graph.edges.filter((edge) => edge.to === id).map((edge) => edge.from).sort();

  if (selectedEdge) {
    return <aside className="details-panel" aria-label="Details">
      <div className="panel-heading"><h2>Inspector</h2><span>Relationship</span></div>
      <div className="details-scroll">
        <div className="detail-intro"><span className="kind-label">{selectedEdge.kind.toLowerCase()}</span><h3>{nameFor(selectedEdge.source, nodes)} <span aria-hidden="true">→</span> {nameFor(selectedEdge.target, nodes)}</h3></div>
        <div className="fact-grid"><span>Source</span><code>{selectedEdge.source}</code><span>Target</span><code>{selectedEdge.target}</code></div>
        {selectedEdge.kind === "IMPORTS" && <section className="detail-section">
          <h3>Supporting imports <span>{selectedEdge.evidence.length}</span></h3>
          <EvidenceList sources={selectedEdge.evidence} nodes={nodes} />
        </section>}
        {selectedSupplierID && selectedImporterID && canonicalGraph.edges.some((edge) => edge.kind === "USES_EXPORT" && edge.from === selectedImporterID && edge.to === selectedSupplierID) && <section className="detail-section">
          <h3>Source-backed file connection</h3>
          <div className="fact-grid"><span>Imports in</span><code>{nodes.get(selectedImporterID)?.path}</code><span>Export used from</span><code>{nodes.get(selectedSupplierID)?.path}</code></div>
        </section>}
      </div>
    </aside>;
  }

  if (!selectedNode) {
    return <aside className="details-panel" aria-label="Details">
      <div className="panel-heading"><h2>Inspector</h2></div>
      <div className="empty-details"><span className="empty-details-glyph" aria-hidden="true">⌖</span><p>Select a node or dependency to inspect its saved graph facts.</p></div>
    </aside>;
  }

  const isPackage = selectedNode.kind === "PACKAGE";
  const isFile = selectedNode.kind === "FILE";
  const outgoing = isPackage ? packageOutgoing(selectedNode.id) : canonicalOutgoing(selectedNode.id);
  const incoming = isPackage ? packageIncoming(selectedNode.id) : canonicalIncoming(selectedNode.id);
  const files = isPackage ? directChildren(selectedNode.id).filter((id) => nodes.get(id)?.kind === "FILE") : [];
  return <aside className="details-panel" aria-label="Details">
    <div className="panel-heading"><h2>Inspector</h2><span>{selectedNode.kind.toLowerCase()}</span></div>
    <div className="details-scroll">
      <div className="detail-intro"><h3>{selectedNode.name}</h3><span className="kind-label">{selectedNode.kind.toLowerCase()}</span><code className="detail-path">{selectedNode.path}</code></div>
      {isPackage && <div className="detail-summary"><span>{files.length} files</span><span>{outgoing.length} dependencies</span><span>{incoming.length} dependents</span></div>}

      {isPackage && <>
        <FactList title="Files" ids={files} nodes={nodes} onSelectNode={onSelectNode} />
        <FactList title="Depends on" ids={outgoing} nodes={nodes} onSelectNode={onSelectNode} />
        <FactList title="Depended on by" ids={incoming} nodes={nodes} onSelectNode={onSelectNode} />
        {packageProjection.evidence.filter((item) => item.edge.from === selectedNode.id).map((item) =>
          <section className="detail-section" key={`${item.edge.from}->${item.edge.to}`}>
            <h3>Evidence for {nameFor(item.edge.to, nodes)}</h3>
            <EvidenceList sources={item.sources} nodes={nodes} />
          </section>
        )}
        <section className="detail-section impact-section">
          <button className="action-button" type="button" onClick={() => onShowImpact(selectedNode.id)} disabled={impactLoading}>{impactLoading ? "Loading impact…" : "Show impact"}</button>
          {impactError && <p className="inline-error" role="alert">{impactError}</p>}
          {impact?.target === selectedNode.id && <>
            <h3>Potentially affected <span>{impact.affected.length}</span></h3>
            {impact.incomplete && <p className="impact-caveat">Analysis has gaps. This impact map may be incomplete.</p>}
            {impact.affected.length === 0 ? <p className="muted">No known dependent packages</p> : <ol className="affected-list">{impact.affected.map((item) =>
              <li key={item.id}><span className="distance-mark">{item.distance}</span><button type="button" onClick={() => onSelectNode(item.id)}>{nameFor(item.id, nodes)}</button></li>
            )}</ol>}
          </>}
        </section>
      </>}

      {isFile && <>
        <FactList title="Package" ids={parent.has(selectedNode.id) ? [parent.get(selectedNode.id)!] : []} nodes={nodes} onSelectNode={onSelectNode} />
        <FactList title="Declarations" ids={directChildren(selectedNode.id).filter((id) => isDeclarationKind(nodes.get(id)?.kind ?? "MODULE"))} nodes={nodes} onSelectNode={onSelectNode} />
        <FactList title="Imports" ids={outgoing} nodes={nodes} onSelectNode={onSelectNode} />
        <FactList title="Imported by" ids={incoming} nodes={nodes} onSelectNode={onSelectNode} />
      </>}

      {isDeclarationKind(selectedNode.kind) && <FactList title="Containing file" ids={parent.has(selectedNode.id) ? [parent.get(selectedNode.id)!] : []} nodes={nodes} onSelectNode={onSelectNode} />}
      <div className="detail-id"><span>Node ID</span><code>{selectedNode.id}</code></div>
    </div>
  </aside>;
}
