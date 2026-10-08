import assert from "node:assert/strict";
import { vscodeLocation } from "./editor.ts";
import { SnapshotCache, mergeDetail } from "./loading.ts";
import type { LoadedData, GraphDetail } from "./types.ts";

const cache=new SnapshotCache();cache.pin("old");
let calls=0;
assert.equal(await cache.get("detail",async()=>{calls++;return 1;}),1);
assert.equal(await cache.get("detail",async()=>{calls++;return 2;}),1);
assert.equal(calls,1);
let finish:(value:number)=>void=()=>{};
const pending=cache.get("pending",()=>new Promise<number>(resolve=>{finish=resolve;}));
cache.pin("new");finish(3);
await assert.rejects(pending,/superseded/);
assert.equal(await cache.get("detail",async()=>4),4);
const abort=new AbortController();abort.abort();
await assert.rejects(cache.get("detail",async()=>5,abort.signal),/cancelled/);
await assert.rejects(cache.get("failed",async()=>{throw new Error("offline");}),/offline/);
assert.equal(await cache.get("failed",async()=>6),6);
const edge={kind:"IMPORTS",from:"f",to:"p"} as const;
const current:LoadedData={scan:{id:"saved",root:"/repo",created_at:"",status:"complete",coverage:{files_discovered:0,supported_source_files:0,files_analyzed:0,files_skipped:0,files_failed:0,imports_discovered:0,internal_imports_resolved:0,standard_library_imports:0,external_imports:0,unresolved_imports:0,cgo_imports:0,unclassified_imports:0},issues:[]},canonicalGraph:{nodes:[{id:"p",kind:"PACKAGE",name:"p",path:"p"}],edges:[]},packageProjection:{graph:{nodes:[],edges:[]},evidence:[]}};
const next:GraphDetail={graph:{nodes:[{id:"f",kind:"FILE",name:"f",path:"p/f"}],edges:[edge]},package_projection:{graph:{nodes:[],edges:[]},evidence:[]},child_counts:{p:100}};
const merged=mergeDetail(mergeDetail(current,next),next);
assert.equal(merged.canonicalGraph.nodes.length,2);
assert.deepEqual(merged.canonicalGraph.edges,[edge]);
assert.equal(merged.childCounts?.p,100);
assert.equal(current.canonicalGraph.nodes.length,1);
assert.equal(current.canonicalGraph.edges.length,0);
console.log("Snapshot loading: cache, cancellation, stale responses, retry and exact merges passed");

assert.equal(vscodeLocation("/repo/with space/a#?.go",2,3),"vscode://file/repo/with%20space/a%23%3F.go:2:3");
assert.equal(vscodeLocation("C:\u005crepo\u005cfile.go",2,3),"vscode://file/C:/repo/file.go:2:3");

const { buildVisibleGraph, importCardLines, packageSourceCounts }=await import("./presentation.ts");
const { bundleRoutes }=await import("./bundles.ts");
const packages=[{id:"p",kind:"PACKAGE" as const,name:"p",path:"p"},{id:"q",kind:"PACKAGE" as const,name:"q",path:"q"}];
const projected={kind:"IMPORTS" as const,from:"p",to:"q"};
const summary={nodes:packages,edges:[]};
const visible=buildVisibleGraph({canonicalGraph:summary,packageProjection:{graph:{nodes:packages,edges:[projected]},evidence:[],relationship_totals:{"IMPORTS:p->q":100}},expandedPackages:new Set(),expandedFiles:new Set(),visibleDeclarationKinds:new Set(),fileCounts:{p:100,q:1}});
const nodes=new Map(visible.nodes.map(n=>[n.id,n]));
const lines=visible.edges.flatMap(e=>importCardLines(e,nodes,new Map()));
const bundles=bundleRoutes(lines,new Map(visible.edges.map(e=>[e.id,e])),new Map());
assert.equal(bundles.length,1);
assert.equal(bundles[0].facts.length,0);
assert.equal(bundles[0].unloadedFacts,100);
assert.equal(visible.nodes.find(n=>n.id==="p")?.childCount,100);
assert.equal(packageSourceCounts(summary,{p:100,q:1}).get("p")?.imports,undefined);

// Inspection returns only part of a projection; it must not replace complete evidence.
const imports=[{kind:"IMPORTS" as const,from:"f1",to:"q"},{kind:"IMPORTS" as const,from:"f2",to:"q"}];
const loaded={...current,packageProjection:{graph:{nodes:packages,edges:[projected]},evidence:[{edge:projected,sources:imports}],relationship_totals:{"IMPORTS:p->q":100}}};
const partial={...next,package_projection:{graph:{nodes:packages,edges:[projected]},evidence:[{edge:projected,sources:[imports[0]]}],relationship_totals:{"IMPORTS:p->q":100}}};
assert.deepEqual(mergeDetail(loaded,partial).packageProjection.evidence[0].sources,imports);

const firstImport={kind:"IMPORTS" as const,from:"f1",to:"q"};
const partialCanonical={nodes:[...packages,{id:"f1",kind:"FILE" as const,name:"a.go",path:"p/a.go",language:"go"}],edges:[{kind:"CONTAINS" as const,from:"p",to:"f1"},firstImport]};
const partialVisible=buildVisibleGraph({canonicalGraph:partialCanonical,packageProjection:{graph:{nodes:packages,edges:[projected]},evidence:[{edge:projected,sources:[firstImport]}],relationship_totals:{"IMPORTS:p->q":100}},expandedPackages:new Set(),expandedFiles:new Set(),visibleDeclarationKinds:new Set()});
assert.equal(partialVisible.edges.filter(e=>e.kind==="IMPORTS").length,1);
const partialNodes=new Map(partialVisible.nodes.map(n=>[n.id,n]));
const partialLines=partialVisible.edges.filter(e=>e.kind==="IMPORTS").flatMap(e=>importCardLines(e,partialNodes,new Map()));
const partialBundles=bundleRoutes(partialLines,new Map(partialVisible.edges.map(e=>[e.id,e])),new Map());
assert.equal(partialBundles[0].members.length,1);
assert.equal(partialBundles[0].facts.length,1);
assert.equal(partialBundles[0].unloadedFacts,99);
