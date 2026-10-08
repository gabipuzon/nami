"use client";

import { getGraphIndex } from "../lib/graphIndex";
import { useState } from "react";
import { PagedList } from "./PagedList";
import { SourceEvidence } from "./SourceEvidence";
import { RelationshipEvidence } from "./RelationshipEvidence";
import { NodeDetails } from "./NodeDetails";
import type { RouteBundle } from "../lib/bundles";
import type { Graph, GraphDetail, GraphEdge, GraphNode, Impact, PackageProjection } from "../lib/types";
import type { NodePresentation } from "../lib/languagePresentation";
import type { VisibleEdge } from "../lib/presentation";

interface DetailsProps {
  onMerge: (detail: GraphDetail) => void;
  bundle: RouteBundle | null;
  onRevealFiles: (ids: string[]) => void;
  onFocus: (node: GraphNode) => void;
  selectedNode: GraphNode | null;
  selectedEdge: VisibleEdge | null;
  selectedSupplierID: string | null;
  selectedImporterID: string | null;
  canonicalGraph: Graph;
  presentations: ReadonlyMap<string, NodePresentation>;
  packageProjection: PackageProjection;
  impact: Impact | null;
  onSelectNode: (id: string) => void;
}

function Fact({ edge, scope, onRevealFiles }: {edge:GraphEdge;scope:"canonical"|"package";onRevealFiles:(ids:string[])=>void}) {
  const [open,setOpen]=useState(false);
  return <li><code>{edge.from} → {edge.to}</code>
    <button onClick={()=>onRevealFiles([edge.from,edge.to])}>Reveal endpoints</button>
    <button onClick={()=>setOpen(value=>!value)}>{open?"Hide":"Show"} source evidence</button>
    {open&&<SourceEvidence edge={edge} scope={scope}/>}
  </li>;
}

export function Details(props: DetailsProps) {
  const {selectedNode,selectedEdge,bundle,onRevealFiles,onFocus,onSelectNode,onMerge,canonicalGraph}=props;
  if(selectedNode&&!bundle&&!selectedEdge)return <NodeDetails presentation={props.presentations.get(selectedNode.id)} impact={props.impact} node={selectedNode} onMerge={onMerge} onSelectNode={onSelectNode} onFocus={onFocus}/>;
  const nodes=getGraphIndex(canonicalGraph).nodes;
  const scope=(edge:GraphEdge)=>nodes.get(edge.from)?.kind==="PACKAGE"?"package" as const:"canonical" as const;
  return <aside className="details-panel" aria-label="Details"><div className="panel-heading"><h2>Inspector</h2><span>{bundle?"Rendered bundle":selectedEdge?"Relationship":""}</span></div><div className="details-scroll">
    {bundle?<>
      <p>{bundle.members.length} display routes · {bundle.facts.length} loaded supporting imports</p>
      <p className="muted">Display counts describe rendering; analysis coverage is reported separately.</p>
      {!!bundle.unloadedFacts&&<p>{bundle.unloadedFacts} supporting imports are not loaded. Inspect the relationship below for saved evidence.</p>}
      <ul className="evidence-list"><PagedList items={bundle.facts} render={edge=><Fact key={`${edge.kind}:${edge.from}:${edge.to}`} edge={edge} scope={scope(edge)} onRevealFiles={onRevealFiles}/>}/></ul>
      {selectedEdge?.projectionEdge&&<RelationshipEvidence edge={selectedEdge.projectionEdge} scope="package" onReveal={onRevealFiles}/>}
      <h3>Route members</h3><ul className="evidence-list"><PagedList items={bundle.members} render={member=><li key={member.id}>
        <code>{member.importingFileID??bundle.target} → {member.supplyingFileID??bundle.source}</code>
        {member.importingFileID && member.supplyingFileID && canonicalGraph.edges.some(e=>e.kind==="USES_EXPORT"&&e.from===member.importingFileID&&e.to===member.supplyingFileID)&&<ul><Fact edge={{kind:"USES_EXPORT",from:member.importingFileID,to:member.supplyingFileID}} scope="canonical" onRevealFiles={onRevealFiles}/></ul>}
        {(member.importingFileID||member.supplyingFileID)&&<button onClick={()=>onRevealFiles([member.importingFileID,member.supplyingFileID].filter((id):id is string=>!!id))}>Reveal files</button>}
      </li>}/></ul>
    </>:selectedEdge?<>
      <h3>{selectedEdge.kind.toLowerCase()}</h3><code>{selectedEdge.source} → {selectedEdge.target}</code>
      <RelationshipEvidence edge={selectedEdge.projectionEdge??{kind:selectedEdge.kind,from:selectedEdge.source,to:selectedEdge.target}} scope={selectedEdge.projectionEdge?"package":"canonical"} onReveal={onRevealFiles}/>
      <ul className="evidence-list"><PagedList items={selectedEdge.evidence} render={edge=><Fact key={`${edge.kind}:${edge.from}:${edge.to}`} edge={edge} scope={scope(edge)} onRevealFiles={onRevealFiles}/>}/></ul>
    </>:<p>Select a node or dependency to inspect its saved facts.</p>}
  </div></aside>;
}
