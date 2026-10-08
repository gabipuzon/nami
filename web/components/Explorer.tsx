"use client";

import { recordPerformance } from "../lib/performance";

import { useEffect, useMemo, useRef, useState } from "react";
import { mergeGraph } from "../lib/loading";
import { loadChildren, loadSearch } from "../lib/api";
import { PageControls } from "./PagedList";
import { presentationFor } from "../lib/languagePresentation";
import { getGraphIndex } from "../lib/graphIndex";
import type { DeclarationKind, Graph, GraphDetail, NodePage } from "../lib/types";
import { type NodePresentation } from "../lib/languagePresentation";
import { declarationKinds } from "../lib/types";
import { flattenExplorer } from "../lib/navigation";
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
  const [pages, setPages] = useState<Record<string,{next:number;total:number}>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [scrollTop, setScrollTop] = useState(0);
  const rows = useMemo(() => flattenExplorer(treeGraph, expanded, visibleDeclarationKinds, childCounts), [treeGraph, expanded, visibleDeclarationKinds,childCounts]);
  const [pageQuery,setPageQuery] = useState("");
  const [searchPage, setSearchPage] = useState<NodePage | null>(null);
  const [searchOffset, setSearchOffset] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const [pending, setPending] = useState<string | null>(null);
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
  const first = Math.max(0, Math.floor(scrollTop / 32) - 2);
  const last = Math.min(rows.length, first + 24);
  const load = (id: string, offset: number) => {
    childrenRequests.current.get(id)?.abort();
    const controller=new AbortController();childrenRequests.current.set(id,controller);setPending(id);setError(null);failedChild.current=null;
    loadChildren(id,offset,controller.signal).then(page => {
      if (controller.signal.aborted) return;
      onMerge(page);setTreeGraph(current=>mergeGraph(current,page.graph));setPages(current=>({...current,[id]:{next:offset+page.nodes.length,total:page.total}}));setExpanded(current => new Set(current).add(id));setPending(null);
    }).catch((error: unknown) => {if (!controller.signal.aborted) {setError(error instanceof Error ? error.message : "Unable to load children.");failedChild.current={id,offset};setPending(null);}});
  };
  const toggle = (id: string) => {
    if (expanded.has(id)) {childrenRequests.current.get(id)?.abort();setExpanded(current => {const next=new Set(current);next.delete(id);return next;});}
    else load(id,0);
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
      {pending && <p role="status">Loading saved children…</p>}
      <div className="section-heading tree-heading">Packages / modules <span>{counts.packages}</span></div>
      <div className="explorer-tree-window" onScroll={event => setScrollTop(event.currentTarget.scrollTop)}>
        <div style={{ height: first * 32 }} aria-hidden="true" />
        {rows.slice(first,last).map(({node, depth, expandable}) => {
          const profile = presentations.get(node.id)!;
          const loaded = getGraphIndex(treeGraph).children.get(node.id)?.length ?? 0;
          const total = pages[node.id]?.total ?? childCounts?.[node.id] ?? loaded;
          const next = pages[node.id]?.next ?? 0;
          return <div key={node.id} className={`tree-row virtual-tree-row ${selectedNodeID === node.id ? "is-selected" : ""}`} style={{ paddingLeft: 8 + depth * 14 }}>
            {expandable ? <button type="button" className="tree-toggle" onClick={() => toggle(node.id)} aria-label={`${expanded.has(node.id) ? "Collapse" : "Expand"} ${profile.displayName}`}>{expanded.has(node.id) ? "▾" : "▸"}</button> : <span className="tree-spacer" />}
            <button type="button" className="tree-label" onClick={() => onSelectNode(node.id)} title={`${node.path} · ${profile.displayKind}`}><span className="tree-label-primary">{profile.rowName}</span></button>
            {expanded.has(node.id) && next < total && <button type="button" className="tree-toggle" disabled={pending===node.id} onClick={() => load(node.id,next)} title={`Showing ${next} of ${total}; load next page`}>More {next}/{total}</button>}
            {profile.rowMark && <span className="tree-kind">{profile.rowMark}</span>}
          </div>;
        })}
        <div style={{ height: Math.max(0,rows.length-last) * 32 }} aria-hidden="true" />
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
