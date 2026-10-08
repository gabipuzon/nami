"use client";

import { useEffect, useState } from "react";
import type { GraphEdge, RelationshipFacts } from "../lib/types";
import { loadRelationshipFacts } from "../lib/api";
import { PageControls } from "./PagedList";
import { SourceEvidence } from "./SourceEvidence";

export function RelationshipEvidence({edge,scope,onReveal}: {edge:GraphEdge;scope:"canonical"|"package";onReveal:(ids:string[])=>void}) {
  const [page,setPage]=useState<RelationshipFacts | null>(null);
  const [offset,setOffset]=useState(0);
  const [error,setError]=useState<string | null>(null);
  const [attempt,setAttempt]=useState(0);
  useEffect(()=>{
    const controller=new AbortController();
    loadRelationshipFacts({kind:edge.kind,from:edge.from,to:edge.to},scope,offset,controller.signal).then(result=>{setPage(result);setError(null);}).catch((error:unknown)=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:"Unable to load supporting facts.");});
    return ()=>controller.abort();
  },[edge.kind,edge.from,edge.to,scope,offset,attempt]);
  return <section className="detail-section"><h3>Supporting saved facts {page&&<span>{page.total}</span>}</h3>
    {error&&<p role="alert">{error} <button onClick={()=>setAttempt(n=>n+1)}>Retry</button></p>}
    {!page&&!error&&<p>Supporting facts are loading…</p>}
    {page&&<><ul className="evidence-list">{page.items.map(fact=><li key={`${fact.kind}:${fact.from}:${fact.to}`}><code>{page.graph.nodes.find(n=>n.id===fact.from)?.path??fact.from} → {page.graph.nodes.find(n=>n.id===fact.to)?.path??fact.to}</code><button onClick={()=>onReveal([fact.from,fact.to])}>Reveal endpoints</button></li>)}</ul><PageControls total={page.total} offset={page.offset} onChange={setOffset}/></>}
    <SourceEvidence edge={edge} scope={scope}/>
  </section>;
}
