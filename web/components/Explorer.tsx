"use client";

import { recordPerformance } from "../lib/performance";

import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { mergeGraph } from "../lib/loading";
import { loadChildren, loadSearch } from "../lib/api";
import { PageControls } from "./PagedList";
import { presentationFor } from "../lib/languagePresentation";
import { getGraphIndex } from "../lib/graphIndex";
import type { DeclarationKind, Graph, GraphDetail, NodePage } from "../lib/types";
import { type NodePresentation } from "../lib/languagePresentation";
import { declarationKinds } from "../lib/types";
import { flattenExplorer, explorerWindow, explorerPagination } from "../lib/navigation";
import { type VisibleGraph } from "../lib/presentation";

interface ExplorerProps {
  counts?: { packages: number; files: number; declarations: number };
  childCounts?: Record<string, number>;
  onMerge: (detail: GraphDetail) => void;
  canonicalGraph: Graph;
  presentations: ReadonlyMap<string, NodePresentation>;
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
  recordPerformance("render.Explorer");
  const { childCounts, onMerge, canonicalGraph, presentations, selectedNodeID, search, visibleDeclarationKinds, onSearch, onSelectNode, onToggleKind } = props;
  const [treeGraph, setTreeGraph] = useState(canonicalGraph);
  const [{ pages, expanded, pending }, dispatch] = useReducer(explorerPagination, { pages: {}, expanded: new Set<string>(), pending: new Set<string>() });
  const [scrollTop, setScrollTop] = useState(0);
  const rows = useMemo(() => flattenExplorer(treeGraph, expanded, visibleDeclarationKinds, childCounts, pages), [treeGraph, expanded, visibleDeclarationKinds,childCounts,pages]);
  const [pageQuery,setPageQuery] = useState("");
  const [searchPage, setSearchPage] = useState<NodePage | null>(null);
  const [searchOffset, setSearchOffset] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const childrenRequests = useRef(new Map<string,AbortController>());
  const failedChild = useRef<{id:string;offset:number} | null>(null);
  useEffect(() => () => childrenRequests.current.forEach(c => c.abort()), []);
  useEffect(() => {
    if (!search.trim()) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      loadSearch(search,searchOffset,controller.signal).then(page => {setSearchPage(page);setPageQuery(search);setError(null);}).catch((error: unknown) => {
        if (!controller.signal.aborted) {failedChild.current=null;setError(error instanceof Error ? error.message : "Unable to search saved nodes.");}
      });
    },150);
    return () => {clearTimeout(timer);controller.abort();};
  }, [search,searchOffset,retry]);
  const results = searchPage?.nodes ?? [];
  const counts = props.counts ?? getGraphIndex(canonicalGraph).counts;
  const {first,last,before,after}=explorerWindow(rows.length,scrollTop);
  const load = (id: string, offset: number) => {
    childrenRequests.current.get(id)?.abort();
    const controller = new AbortController();
    childrenRequests.current.set(id, controller);
    dispatch({ type: "start", id });
    setError(null);
    failedChild.current = null;
    loadChildren(id,offset,controller.signal).then(page => {
      if (controller.signal.aborted) return;
      onMerge(page);
      setTreeGraph(current => mergeGraph(current, page.graph));
      dispatch({ type: "loaded", id, next: offset + page.nodes.length, total: page.total });
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setError(error instanceof Error ? error.message : "Unable to load children.");
      failedChild.current = { id, offset };
      dispatch({ type: "settled", id });
    }).finally(() => {
      if (childrenRequests.current.get(id) === controller) childrenRequests.current.delete(id);
    });
  };
  const toggle = (id: string) => {
    if (expanded.has(id)) {
      childrenRequests.current.get(id)?.abort();
      dispatch({ type: "collapse", id });
    } else if (pages[id]) {
      dispatch({ type: "expand", id });
    } else {
      load(id, 0);
    }
  };

  return <aside className="explorer-panel" aria-label="Explorer">
    <div className="panel-heading"><h2>Explorer</h2><span>Saved graph</span></div>
    <div className="explorer-scroll">
      <label className="search-label" htmlFor="node-search">Find a node</label>
      <input id="node-search" className="search-input" type="search" value={search} onChange={(event) => {setSearchOffset(0);onSearch(event.target.value);}} placeholder="Name, path, or ID" autoComplete="off" />
      {search.trim() && <div className="search-results" aria-label="Search results">
        {pageQuery!==search && <p role="status">Searching…{pageQuery && ` Showing previous matches for ${pageQuery}.`}</p>}
        <div className="section-heading">Matches <span>{searchPage?.total ?? "…"}</span></div>
        {results.length === 0 ? <p className="muted compact">{searchPage ? "No matching graph nodes" : "Searching saved nodes…"}</p> : results.map((node) =>
          <button key={node.id} className="search-result" type="button" onClick={() => onSelectNode(node.id)} title={node.id}>
            <span>{(presentations.get(node.id) ?? presentationFor(node)).displayName}</span><small>{(presentations.get(node.id) ?? presentationFor(node)).displayKind} · {node.path}</small>
          </button>
        )}
        {searchPage && <PageControls total={searchPage.total} offset={searchPage.offset} onChange={setSearchOffset} />}
      </div>}
      {error && <p role="alert">{error} <button onClick={() => {if (failedChild.current) load(failedChild.current.id,failedChild.current.offset);else setRetry(n=>n+1);}}>Retry</button></p>}
      {pending.size>0 && <p role="status">Loading saved children…</p>}
      <div className="section-heading tree-heading">Packages / modules <span>{counts.packages}</span></div>
      <div className="explorer-tree-window" onScroll={event => setScrollTop(event.currentTarget.scrollTop)}>
        <div style={{ height: before }} aria-hidden="true" />
        {rows.slice(first,last).map(row => {
          if(row.kind==="more") return <div key={row.key} className="tree-row virtual-tree-row tree-more-row" style={{paddingLeft:Math.min(8+row.depth*14,64)}} data-parent-id={row.parent.id}>
            <button type="button" disabled={pending.has(row.parent.id)} onClick={()=>load(row.parent.id,row.loaded)} aria-label={`Load more children of ${row.parent.name}; showing ${row.loaded} of ${row.total}`} title={row.parent.path}>More</button>
            <span>{row.loaded} / {row.total}</span>
          </div>;
          const {node,depth,expandable}=row;
          const profile = presentations.get(node.id) ?? presentationFor(node);
          return <div key={row.key} className={`tree-row virtual-tree-row ${selectedNodeID === node.id ? "is-selected" : ""}`} style={{ paddingLeft: Math.min(8 + depth * 14,64) }}>
            {expandable ? <button type="button" className="tree-toggle" onClick={() => toggle(node.id)} aria-label={`${expanded.has(node.id) ? "Collapse" : "Expand"} ${profile.displayName}`}>{expanded.has(node.id) ? "▾" : "▸"}</button> : <span className="tree-spacer" />}
            <button type="button" className="tree-label" onClick={() => onSelectNode(node.id)} title={`${node.path} · ${profile.displayKind}`}><span className="tree-label-primary">{profile.rowName}</span></button>
            {profile.rowMark && <span className="tree-kind">{profile.rowMark}</span>}
          </div>;
        })}
        <div style={{ height: after }} aria-hidden="true" />
      </div>
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
