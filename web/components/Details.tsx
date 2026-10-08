"use client";

import type { Graph, GraphEdge, GraphNode, Impact, PackageProjection } from "../lib/types";
import { buildNodePresentations, type NodePresentation } from "../lib/languagePresentation";
import { isDeclarationKind, type VisibleEdge } from "../lib/presentation";

interface DetailsProps {
  selectedNode: GraphNode | null;
  selectedEdge: VisibleEdge | null;
  selectedSupplierID: string | null;
  selectedImporterID: string | null;
  canonicalGraph: Graph;
  packageProjection: PackageProjection;
  impact: Impact | null;
  onSelectNode: (id: string) => void;
}

const nameFor = (id: string, nodes: ReadonlyMap<string, GraphNode>, presentations: ReadonlyMap<string, NodePresentation>): string => presentations.get(id)?.displayName || nodes.get(id)?.name || id;

function FactList({ title, ids, nodes, presentations, onSelectNode }: {
  title: string;
  ids: string[];
  nodes: ReadonlyMap<string, GraphNode>;
  presentations: ReadonlyMap<string, NodePresentation>;
  onSelectNode: (id: string) => void;
}) {
  return <section className="detail-section">
    <h3>{title} <span>{ids.length}</span></h3>
    {ids.length === 0 ? <p className="muted">None known</p> : <ul className="detail-list">{ids.map((id) =>
      <li key={id}><button type="button" onClick={() => onSelectNode(id)} title={nodes.get(id)?.path ?? id}>
        <span className="detail-row-name">{presentations.get(id)?.rowName ?? id}</span>
        <span className="detail-row-meta">{presentations.get(id)?.secondaryLabel ?? nodes.get(id)?.path}</span>
      </button></li>
    )}</ul>}
  </section>;
}

function EvidenceList({ sources, nodes, presentations }: { sources: GraphEdge[]; nodes: ReadonlyMap<string, GraphNode>; presentations: ReadonlyMap<string, NodePresentation> }) {
  return <ul className="evidence-list">{sources.map((edge) =>
    <li key={`${edge.from}->${edge.to}`}><span>{nameFor(edge.from, nodes, presentations)}</span><span aria-hidden="true">→</span><span>{nameFor(edge.to, nodes, presentations)}</span></li>
  )}</ul>;
}

export function Details(props: DetailsProps) {
  const { selectedNode, selectedEdge, selectedSupplierID, selectedImporterID, canonicalGraph, packageProjection, impact, onSelectNode } = props;
  const presentations = buildNodePresentations(canonicalGraph);
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
        <div className="detail-intro"><span className="kind-label">{selectedEdge.kind.toLowerCase()}</span><h3>{nameFor(selectedEdge.source, nodes, presentations)} <span aria-hidden="true">→</span> {nameFor(selectedEdge.target, nodes, presentations)}</h3></div>
        <div className="fact-grid"><span>Source</span><code>{selectedEdge.source}</code><span>Target</span><code>{selectedEdge.target}</code></div>
        {selectedEdge.kind === "IMPORTS" && <section className="detail-section">
          <h3>Supporting imports <span>{selectedEdge.evidence.length}</span></h3>
          <EvidenceList sources={selectedEdge.evidence} nodes={nodes} presentations={presentations} />
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

  const profile = presentations.get(selectedNode.id)!;
  const isPackage = profile.expansion === "container";
  const isFile = profile.expansion === "source";
  const outgoing = isPackage ? packageOutgoing(selectedNode.id) : canonicalOutgoing(selectedNode.id);
  const incoming = isPackage ? packageIncoming(selectedNode.id) : canonicalIncoming(selectedNode.id);
  const files = isPackage ? directChildren(selectedNode.id).filter((id) => nodes.get(id)?.kind === "FILE") : [];
  const outgoingEvidence = isPackage ? packageProjection.evidence.filter((item) => item.edge.from === selectedNode.id) : [];
  return <aside className="details-panel" aria-label="Details">
    <div className="panel-heading"><h2>Inspector</h2><span>{profile.displayKind}</span></div>
    <div className="details-scroll">
      <div className="detail-intro"><h3>{profile.displayName}</h3><span className="kind-label">{profile.displayKind}</span><code className="detail-path">{selectedNode.path}</code></div>
      <div className="fact-grid"><span>Kind</span><code>{selectedNode.kind}</code><span>Language</span><code>{selectedNode.language || "unknown"}</code></div>
      {isPackage && <div className="detail-summary"><span>{files.length} {profile.childLabel.plural}</span><span>{outgoing.length} dependencies</span><span>{incoming.length} dependents</span></div>}

      {isPackage && <>
        <FactList title="Depends on" ids={outgoing} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        <FactList title="Depended on by" ids={incoming} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        <FactList title={profile.rowGroupLabel} ids={files} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        {outgoingEvidence.length > 0 && <details className="detail-evidence">
          <summary>Import evidence <span>{outgoingEvidence.reduce((count, item) => count + item.sources.length, 0)}</span></summary>
          {outgoingEvidence.map((item) => <section key={`${item.edge.from}->${item.edge.to}`}>
            <h3>{nodes.get(item.edge.to)?.path ?? nameFor(item.edge.to, nodes, presentations)}</h3>
            <EvidenceList sources={item.sources} nodes={nodes} presentations={presentations} />
          </section>)}
        </details>}
        {impact?.target === selectedNode.id && <section className="detail-section impact-section">
            <h3>Potentially affected <span>{impact.affected.length}</span></h3>
            {impact.incomplete && <p className="impact-caveat">Analysis has gaps. This impact map may be incomplete.</p>}
            {impact.affected.length === 0 ? <p className="muted">No known dependent packages</p> : <ol className="affected-list">{impact.affected.map((item) =>
              <li key={item.id}><span className="distance-mark">{item.distance}</span><button type="button" onClick={() => onSelectNode(item.id)}>{nameFor(item.id, nodes, presentations)}</button></li>
            )}</ol>}
        </section>}
      </>}

      {isFile && <>
        <FactList title="Package" ids={parent.has(selectedNode.id) ? [parent.get(selectedNode.id)!] : []} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        <FactList title="Declarations" ids={directChildren(selectedNode.id).filter((id) => isDeclarationKind(nodes.get(id)?.kind ?? "MODULE"))} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        <FactList title="Imports" ids={outgoing} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
        <FactList title="Imported by" ids={incoming} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />
      </>}

      {isDeclarationKind(selectedNode.kind) && <FactList title={`Containing ${presentations.get(parent.get(selectedNode.id) ?? "")?.displayKind ?? "source"}`} ids={parent.has(selectedNode.id) ? [parent.get(selectedNode.id)!] : []} nodes={nodes} presentations={presentations} onSelectNode={onSelectNode} />}
      <div className="detail-id"><span>Node ID</span><code>{selectedNode.id}</code></div>
    </div>
  </aside>;
}
