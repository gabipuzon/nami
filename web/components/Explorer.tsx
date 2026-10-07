"use client";

import type { DeclarationKind, Graph, GraphNode } from "../lib/types";
import { declarationKinds } from "../lib/types";
import { searchNodes, type VisibleGraph } from "../lib/presentation";

interface ExplorerProps {
  canonicalGraph: Graph;
  visibleGraph: VisibleGraph;
  selectedNodeID: string | null;
  search: string;
  visibleDeclarationKinds: ReadonlySet<DeclarationKind>;
  onSearch: (value: string) => void;
  onSelectNode: (id: string) => void;
  onToggle: (id: string) => void;
  onToggleKind: (kind: DeclarationKind) => void;
}

export function Explorer(props: ExplorerProps) {
  const { canonicalGraph, visibleGraph, selectedNodeID, search, visibleDeclarationKinds, onSearch, onSelectNode, onToggle, onToggleKind } = props;
  const packages = visibleGraph.nodes.filter((node) => node.kind === "PACKAGE");
  const children = new Map<string, GraphNode[]>();
  for (const node of visibleGraph.nodes) {
    if (node.parentId) children.set(node.parentId, [...(children.get(node.parentId) ?? []), node]);
  }
  const results = searchNodes(canonicalGraph, search);
  const counts = {
    packages: canonicalGraph.nodes.filter((node) => node.kind === "PACKAGE").length,
    files: canonicalGraph.nodes.filter((node) => node.kind === "FILE").length,
    declarations: canonicalGraph.nodes.filter((node) => declarationKinds.includes(node.kind as DeclarationKind)).length,
  };

  const renderEntry = (node: GraphNode, depth: number) => {
    const item = visibleGraph.nodes.find((visible) => visible.id === node.id);
    if (!item) return null;
    const expandable = item.childCount > 0 && (item.kind === "PACKAGE" || item.kind === "FILE");
    return <div key={item.id}>
      <div className={`tree-row ${selectedNodeID === item.id ? "is-selected" : ""}`} style={{ paddingLeft: 12 + depth * 14 }}>
        {expandable ? <button type="button" className="tree-toggle" onClick={() => onToggle(item.id)} aria-label={`${item.expanded ? "Collapse" : "Expand"} ${item.name}`}>{item.expanded ? "▾" : "▸"}</button> : <span className="tree-spacer" />}
        <button type="button" className="tree-label" onClick={() => onSelectNode(item.id)} title={item.path}>{item.name}</button>
        <span className="tree-kind">{item.kind === "PACKAGE" ? "P" : item.kind === "FILE" ? "F" : "·"}</span>
      </div>
      {item.expanded && (children.get(item.id) ?? []).map((child) => renderEntry(child, depth + 1))}
    </div>;
  };

  return <aside className="explorer-panel" aria-label="Explorer">
    <div className="panel-heading"><h2>Explorer</h2><span>Saved graph</span></div>
    <div className="explorer-scroll">
      <label className="search-label" htmlFor="node-search">Find a node</label>
      <input id="node-search" className="search-input" type="search" value={search} onChange={(event) => onSearch(event.target.value)} placeholder="Name, path, or ID" autoComplete="off" />
      {search.trim() && <div className="search-results" aria-label="Search results">
        <div className="section-heading">Matches <span>{results.length}{results.length === 40 ? "+" : ""}</span></div>
        {results.length === 0 ? <p className="muted compact">No matching graph nodes</p> : results.map((node) =>
          <button key={node.id} className="search-result" type="button" onClick={() => onSelectNode(node.id)} title={node.id}>
            <span>{node.name}</span><small>{node.kind.toLowerCase()} · {node.path}</small>
          </button>
        )}
      </div>}
      <div className="section-heading tree-heading">Packages <span>{packages.length}</span></div>
      <div className="tree-list">{packages.map((pkg) => renderEntry(pkg, 0))}</div>

      <div className="explorer-secondary">
        <div className="section-heading">Visible levels</div>
        <div className="count-row"><span>Packages</span><strong>{counts.packages}</strong></div>
        <div className="count-row"><span>Files</span><strong>{counts.files}</strong></div>
        <div className="count-row"><span>Declarations</span><strong>{counts.declarations}</strong></div>

        <div className="section-heading filter-heading">Declaration kinds</div>
        <div className="kind-filters">{declarationKinds.map((kind) =>
          <label key={kind} className="kind-filter"><input type="checkbox" checked={visibleDeclarationKinds.has(kind)} onChange={() => onToggleKind(kind)} /><span>{kind.toLowerCase()}</span></label>
        )}</div>
      </div>
    </div>
  </aside>;
}
