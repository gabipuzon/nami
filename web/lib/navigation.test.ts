import test from "node:test";
import assert from "node:assert/strict";
import { flattenExplorer, explorerWindow, explorerPagination, type ExplorerPagination, neighborhoodGraph, pushFocus, backFocus, RequestGeneration, type FocusEntry } from "./navigation.ts";
import { bundleRoutes, rowWindow, sameWindow, connectedHandles, ABOVE, BELOW } from "./bundles.ts";
import { buildVisibleGraph, buildExportUseIndex, importCardLines, type CardImportLine, type VisibleEdge } from "./presentation.ts";
import type { Graph, Neighborhood } from "./types.ts";

function hubResult(): Neighborhood {
  const focal = { id:"hub",kind:"FILE" as const,path:"hub.py",name:"hub" };
  const dependencies = Array.from({length:20},(_,i) => ({id:`out:${i}`,kind:"FILE" as const,path:`out/${i}.py`,name:`out${i}`}));
  const dependents = Array.from({length:20},(_,i) => ({id:`in:${i}`,kind:"FILE" as const,path:`in/${i}.py`,name:`in${i}`}));
  return {focal,scope:"canonical",graph:{nodes:[focal,...dependencies,...dependents],edges:[...dependencies.map(n => ({kind:"IMPORTS" as const,from:focal.id,to:n.id})),...dependents.map(n => ({kind:"IMPORTS" as const,from:n.id,to:focal.id}))]},dependencies:{nodes:dependencies,total:3000,offset:20},dependents:{nodes:dependents,total:4000,offset:40},evidence:[],page_size:20,snapshot_id:"saved",coverage_status:"completed_with_gaps",incomplete:true};
}

test("large hub focus renders only returned cards and incident imports", () => {
  const result = hubResult();
  const visible = neighborhoodGraph(result);
  assert.equal(visible.nodes.length,41);
  assert.equal(visible.edges.length,40);
  assert(visible.edges.every(e => e.source === "hub" || e.target === "hub"));
  assert(visible.nodes.every(n => n.expanded === false && n.childCount === 0));
  const packageFact = {kind:"IMPORTS" as const,from:"hub",to:"pkg"};
  const packageResult = {...result,scope:"package" as const,graph:{nodes:[],edges:[packageFact]},evidence:[{edge:packageFact,sources:[{kind:"IMPORTS" as const,from:"file",to:"pkg"}]}]};
  assert.deepEqual(neighborhoodGraph(packageResult).edges[0].evidence,packageResult.evidence[0].sources);
});

test("focus history restores pages, dragged positions and viewport without mutating entries", () => {
  const original: FocusEntry = {result:hubResult(),positions:{hub:{x:123,y:456}},viewport:{x:77,y:88,zoom:.42}};
  const history = pushFocus([],original);
  const next = {...original,result:{...original.result,focal:{...original.result.focal,id:"other"}},positions:{other:{x:0,y:0}}};
  const followed = pushFocus(history,next);
  const back = backFocus(followed);
  assert.equal(back.current,next);
  const earlier = backFocus(back.history);
  assert.equal(earlier.current,original);
  assert.equal(earlier.current?.result.dependencies.offset,20);
  assert.equal(earlier.current?.result.dependents.offset,40);
  assert.deepEqual(earlier.current?.positions,{hub:{x:123,y:456}});
  assert.deepEqual(earlier.current?.viewport,{x:77,y:88,zoom:.42});
  assert.equal(history.length,1);
  assert.equal(backFocus([]).current,null);
});

test("superseded and wrong-snapshot responses cannot replace the last successful view", () => {
  const requests = new RequestGeneration();
  const first = requests.next(); const second = requests.next();
  assert.equal(requests.accepts(first,"saved","saved"),false);
  assert.equal(requests.accepts(second,"replacement","saved"),false);
  assert.equal(requests.accepts(second,"saved","saved"),true);
  requests.next();
  assert.equal(requests.accepts(second,"saved","saved"),false);
});

test("Explorer expansion flattens saved children independently of canvas state", () => {
  const graph: Graph = {nodes:[{id:"p",kind:"PACKAGE",name:"p",path:"p"},{id:"f",kind:"FILE",name:"f",path:"p/f.py"},{id:"d",kind:"FUNCTION",name:"run",path:"p/f.py"}],edges:[{kind:"CONTAINS",from:"p",to:"f"},{kind:"CONTAINS",from:"f",to:"d"}]};
  const canvasExpanded = new Set<string>();
  const explorerExpanded = new Set(["p","f"]);
  assert.deepEqual(flattenExplorer(graph,explorerExpanded,new Set(["FUNCTION"])).filter(r=>r.kind==="node").map(r=>[r.node.id,r.depth]),[["p",0],["f",1],["d",2]]);
  assert.equal(canvasExpanded.size,0);
  assert.deepEqual(flattenExplorer(graph,canvasExpanded,new Set(["FUNCTION"])).filter(r=>r.kind==="node").map(r=>r.node.id),["p"]);
  assert.deepEqual(flattenExplorer(graph,explorerExpanded,new Set()).filter(r=>r.kind==="node").map(r=>r.node.id),["p","f"]);
});

test("scrolling and collapse account for every route and fact exactly once with bounded handles", () => {
  const files = Array.from({length:3000},(_,i) => `file:${i}`);
  const connected = new Set(files);
  const edges = new Map<string,VisibleEdge>();
  const lines: CardImportLine[] = files.map((file,i) => {
    const id = `import:${i}`;
    edges.set(id,{id,kind:"IMPORTS",source:file,target:"supplier",evidence:[{kind:"IMPORTS",from:file,to:"provider"}]});
    return {id,canonicalEdgeID:id,source:"supplier",target:"importers",targetHandle:file,sourceHandle:"provider",importingFileID:file,supplyingFileID:"provider"};
  });
  const states = [rowWindow(files,0,10),rowWindow(files,1000,1010),rowWindow(files,2990,3000)];
  for (const window of states) {
    const windows = new Map([["importers",window],["supplier",rowWindow(["provider"],0,1)]]);
    const bundles = bundleRoutes(lines,edges,windows);
    assert(bundles.length <=12);
    assert.deepEqual(bundles.flatMap(b=>b.members.map(m=>m.id)).sort(),files.map((_,i)=>`import:${i}`).sort());
    assert.equal(new Set(bundles.flatMap(b=>b.members.map(m=>m.id))).size,3000);
    assert.deepEqual(bundles.flatMap(b=>b.facts).map(f=>f.from).sort(),[...files].sort());
    assert.deepEqual(bundleRoutes([...lines].reverse(),edges,windows),bundles);
    const handles = connectedHandles(window,connected);
    assert(handles.length <=12);
    assert.equal(handles.includes(ABOVE),window.above.length>0);
    assert.equal(handles.includes(BELOW),window.below.length>0);
    assert(sameWindow(window,rowWindow(files,window.above.length,window.above.length+10)));
  }
  assert.equal(sameWindow(states[0],states[1]),false);
  const collapsed = bundleRoutes(lines.map(l=>({...l,sourceHandle:undefined,targetHandle:undefined})),edges,new Map());
  assert.equal(collapsed.length,1);
  assert.equal(collapsed[0].members.length,3000);
  assert.equal(collapsed[0].facts.length,3000);
  assert.equal(connectedHandles(rowWindow(files,100,110),new Set()).length,0);
});

test("bundles separate relationship kinds and directions without inventing edges", () => {
  const edges = new Map<string,VisibleEdge>([["a",{id:"a",kind:"IMPORTS",source:"a",target:"b",evidence:[{kind:"IMPORTS",from:"a",to:"b"}]}],["b",{id:"b",kind:"USES_EXPORT",source:"a",target:"b",evidence:[{kind:"USES_EXPORT",from:"a",to:"b"}]}]]);
  const lines = [{id:"1",canonicalEdgeID:"a",source:"a",target:"b"},{id:"2",canonicalEdgeID:"b",source:"a",target:"b"},{id:"3",canonicalEdgeID:"a",source:"b",target:"a"}];
  const bundles = bundleRoutes(lines,edges,new Map());
  assert.equal(bundles.length,3);
  assert.equal(bundles.flatMap(b=>b.members).length,3);
  assert.deepEqual(new Set(bundles.flatMap(b=>b.facts).map(f=>f.kind)),new Set(["IMPORTS","USES_EXPORT"]));
});


test("bundling actual Go and Python presentation retains saved imports through every expansion", () => {
  for (const language of ["go","python"]) {
    const packages = ["p","q"].map(id => ({id,kind:"PACKAGE" as const,name:id,path:id,language}));
    const files = ["a","b","provider"].map(id => ({id,kind:"FILE" as const,name:id,path:`${id}.${language === "go" ? "go" : "py"}`,language}));
    const imports = ["a","b"].map(id => ({kind:"IMPORTS" as const,from:id,to:language === "go" ? "q" : "provider"}));
    const canonicalGraph: Graph = {nodes:[...packages,...files],edges:[{kind:"CONTAINS",from:"p",to:"a"},{kind:"CONTAINS",from:"p",to:"b"},{kind:"CONTAINS",from:"q",to:"provider"},...imports,
      ...(language === "go" ? [{kind:"USES_EXPORT" as const,from:"a",to:"provider"},{kind:"USES_EXPORT" as const,from:"b",to:"provider"}] : [])]};
    const projected = {kind:"IMPORTS" as const,from:"p",to:"q"};
    const projection = {graph:{nodes:packages,edges:[projected]},evidence:[{edge:projected,sources:imports}]};
    const canonicalNodes = new Map(canonicalGraph.nodes.map(n=>[n.id,n]));
    const exportUses = buildExportUseIndex(canonicalGraph);
    for (const expandedPackages of [new Set<string>(),new Set(["p"]),new Set(["q"]),new Set(["p","q"])]) {
      const visible = buildVisibleGraph({canonicalGraph,packageProjection:projection,expandedPackages,expandedFiles:new Set(),visibleDeclarationKinds:new Set()});
      const visibleNodes = new Map(visible.nodes.map(n=>[n.id,n]));
      const edges = new Map(visible.edges.filter(e=>e.kind==="IMPORTS").map(e=>[e.id,e]));
      const lines = [...edges.values()].flatMap(e=>importCardLines(e,visibleNodes,exportUses,canonicalNodes));
      for (const offset of [0,1,2]) {
        const windows = new Map<string, ReturnType<typeof rowWindow>>();
        if (expandedPackages.has("p")) windows.set("p",rowWindow(["a","b"],offset,offset+1));
        if (expandedPackages.has("q")) windows.set("q",rowWindow(["provider"],offset,offset+1));
        const bundles = bundleRoutes(lines,edges,windows);
        assert.deepEqual(bundles.flatMap(b=>b.members.map(m=>m.id)).sort(),lines.map(l=>l.id).sort());
        const keys = new Set(bundles.flatMap(b=>b.facts).map(f=>JSON.stringify(f)));
        assert.deepEqual(keys,new Set(imports.map(f=>JSON.stringify(f))));
        assert(bundles.flatMap(b=>b.facts).every(f=>f.kind==="IMPORTS" && f.to===(language === "go" ? "q" : "provider")));
      }
    }
  }
});

test("Explorer More controls appear after expanded nested descendants and retain 32px rows",()=>{
 const graph:Graph={nodes:[
  {id:"root",kind:"PACKAGE",name:"root",path:"root"},
  {id:"nested",kind:"PACKAGE",name:"nested",path:"root/nested"},
  {id:"grandchild",kind:"FILE",name:"grandchild",path:"root/nested/grandchild.py"},
  {id:"sibling",kind:"FILE",name:"sibling",path:"root/sibling.py"},
  {id:"other",kind:"PACKAGE",name:"other",path:"z/other"}
 ],edges:[
  {kind:"CONTAINS",from:"root",to:"nested"},
  {kind:"CONTAINS",from:"nested",to:"grandchild"},
  {kind:"CONTAINS",from:"root",to:"sibling"}
 ]};
 const rows=flattenExplorer(graph,new Set(["root","nested"]),new Set(),
  {root:4,nested:3},{root:{next:2,total:4},nested:{next:1,total:3}});
 assert.deepEqual(rows.map(r=>r.kind==="more"?"more:"+r.parent.id:r.node.id),
  ["root","nested","grandchild","more:nested","sibling","more:root","other"]);
 assert.deepEqual(rows.map(r=>r.depth),[0,1,2,2,1,1,0]);
 assert.equal(rows.length*32,224);
 assert.deepEqual(rows.filter(r=>r.kind==="more").map(r=>({parentID:r.parent.id,loaded:r.loaded,total:r.total})),[
  {parentID:"nested",loaded:1,total:3},{parentID:"root",loaded:2,total:4}
 ]);
 const completed=flattenExplorer(graph,new Set(["root","nested"]),new Set(),
 {root:2,nested:1},{root:{next:2,total:2},nested:{next:1,total:1}});
 assert(!completed.some(r=>r.kind==="more"));
});

test("Explorer collapse/reopen retains cursors, nested expansion and stable More keys", () => {
  const graph: Graph = { nodes: [
    { id: "p", kind: "PACKAGE", name: "p", path: "p" },
    { id: "q", kind: "PACKAGE", name: "q", path: "p/q" },
    { id: "f", kind: "FILE", name: "f", path: "p/q/f.py" },
  ], edges: [{ kind: "CONTAINS", from: "p", to: "q" }, { kind: "CONTAINS", from: "q", to: "f" }] };
  let state: ExplorerPagination = { pages: {}, expanded: new Set(), pending: new Set() };
  state = explorerPagination(state, { type: "loaded", id: "p", next: 20, total: 70 });
  state = explorerPagination(state, { type: "loaded", id: "q", next: 20, total: 50 });
  const flatten = () => flattenExplorer(graph, state.expanded, new Set(), undefined, state.pages);
  const keys = flatten().map(r => r.key);
  assert.deepEqual(keys, ["node:p", "node:q", "node:f", "more:q", "more:p"]);
  for (const next of [40, 60]) {
    state = explorerPagination(state, { type: "loaded", id: "p", next, total: 70 });
    assert.deepEqual(flatten().map(r => r.key), keys);
    const pages = state.pages;
    state = explorerPagination(state, { type: "collapse", id: "p" });
    assert.deepEqual(flatten().map(r => r.key), ["node:p"]);
    state = explorerPagination(state, { type: "expand", id: "p" });
    assert.equal(state.pages, pages);
    assert.equal(state.pages.p.next, next);
    assert(state.expanded.has("q"));
    assert.deepEqual(flatten().map(r => r.key), keys);
  }
});

test("concurrent Explorer loads settle or collapse independently", () => {
  let state: ExplorerPagination = { pages: {}, expanded: new Set(["a", "b"]), pending: new Set() };
  state = explorerPagination(state, { type: "start", id: "a" });
  state = explorerPagination(state, { type: "start", id: "b" });
  const bothPending = state;
  state = explorerPagination(state, { type: "loaded", id: "a", next: 20, total: 40 });
  assert.deepEqual([...state.pending], ["b"]);
  assert.deepEqual([...bothPending.pending], ["a", "b"]);
  state = explorerPagination(state, { type: "start", id: "a" });
  state = explorerPagination(state, { type: "collapse", id: "a" });
  assert.deepEqual([...state.pending], ["b"]);
  assert.equal(state.pages.a.next, 20);
  assert(state.expanded.has("b"));
  state = explorerPagination(state, { type: "settled", id: "b" });
  assert.equal(state.pending.size, 0);
});

test("Explorer virtual windows clamp after collapse and preserve fixed row offsets", () => {
  assert.deepEqual(explorerWindow(100, 32 * 30), { first: 28, last: 52, before: 896, after: 1536 });
  assert.deepEqual(explorerWindow(0, 1000), { first: 0, last: 0, before: 0, after: 0 });
  assert.deepEqual(explorerWindow(3, 3200), { first: 2, last: 3, before: 64, after: 0 });
  assert.deepEqual(explorerWindow(3, -32), { first: 0, last: 3, before: 0, after: 0 });
});
