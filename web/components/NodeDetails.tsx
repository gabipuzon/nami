"use client";

import { useEffect, useState } from "react";
import { loadInspection } from "../lib/api";
import type { GraphDetail, GraphNode, Impact, Inspection, NodePage } from "../lib/types";
import { presentationFor, type NodePresentation } from "../lib/languagePresentation";
import { PageControls } from "./PagedList";

export function NodeDetails({ node, presentation, impact, onMerge, onSelectNode, onFocus }: {node:GraphNode;presentation:NodePresentation|undefined;impact:Impact|null;onMerge:(detail:GraphDetail)=>void;onSelectNode:(id:string)=>void;onFocus:(node:GraphNode)=>void}) {
  const [detail, setDetail]=useState<Inspection | null>(null);
  const [offsets,setOffsets]=useState({children:0,dependencies:0,dependents:0});
  const [error,setError]=useState<string | null>(null);
  const [attempt,setAttempt]=useState(0);
  useEffect(() => {
    const controller=new AbortController();
    loadInspection(node.id,offsets.children,offsets.dependencies,offsets.dependents,controller.signal).then(result=>{setDetail(result);onMerge(result);setError(null);}).catch((error:unknown)=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:"Unable to inspect node.");});
    return ()=>controller.abort();
  },[node.id,offsets,onMerge,attempt]);
  const profile=presentation ?? presentationFor(node);
  const list=(title:string,key:keyof typeof offsets,page:NodePage)=> <section className="detail-section">
    <h3>{title} <span>{page.total}</span></h3>
    {page.total===0?<p className="muted">None known in this saved scan</p>:<>
      <ul className="detail-list">{page.nodes.map(item=><li key={item.id}><button onClick={()=>onSelectNode(item.id)}><span className="detail-row-name">{item.name}</span><code className="detail-row-meta">{item.path}</code></button></li>)}</ul>
      <PageControls total={page.total} offset={page.offset} onChange={offset=>setOffsets(current=>({...current,[key]:offset}))}/>
    </>}
  </section>;
  return <aside className="details-panel" aria-label="Details">
    <div className="panel-heading"><h2>Inspector</h2><span>{profile.displayKind}</span></div>
    <div className="details-scroll"><div className="detail-intro"><h3>{profile.displayName}</h3><code>{node.path}</code></div>
      <p className="muted">{node.kind.toLowerCase()} · {node.language || "unknown language"}</p>
      {(node.kind==="FILE"||node.kind==="PACKAGE")&&<button className="focus-action" onClick={()=>onFocus(node)}>Focus {node.kind.toLowerCase()}</button>}
      {detail && node.kind!=="FILE"&&node.kind!=="PACKAGE"&&detail.parent&&<button onClick={()=>{const parent=detail.graph.nodes.find(n=>n.id===detail.parent);if(parent?.kind==="FILE")onFocus(parent);}}>Focus containing file</button>}
      {error&&<p role="alert">{error} <button onClick={()=>setAttempt(n=>n+1)}>Retry</button></p>}
      {!detail&&!error&&<p>Inspection detail is loading…</p>}
      {detail&&<>
        {detail.parent&&<button onClick={()=>onSelectNode(detail.parent)}>Inspect parent</button>}
        {list("Contains","children",detail.children)}
        {list(node.kind==="PACKAGE"?"Depends on":"Known outgoing relationships","dependencies",detail.dependencies)}
        {list(node.kind==="PACKAGE"?"Depended on by":"Known incoming relationships","dependents",detail.dependents)}
      </>}
      {impact?.affected.some(item=>item.id===node.id)&&<p>Potential impact · dependency distance {impact.affected.find(item=>item.id===node.id)?.distance}</p>}
      <code>{node.id}</code>
    </div>
  </aside>;
}
