import test from "node:test";
import assert from "node:assert/strict";
import { flattenExplorer, neighborhoodGraph, pushFocus, backFocus, RequestGeneration, type FocusEntry } from "./navigation.ts";
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
  assert.deepEqual(flattenExplorer(graph,explorerExpanded,new Set(["FUNCTION"])).map(r=>[r.node.id,r.depth]),[["p",0],["f",1],["d",2]]);
  assert.equal(canvasExpanded.size,0);
  assert.deepEqual(flattenExplorer(graph,canvasExpanded,new Set(["FUNCTION"])).map(r=>r.node.id),["p"]);
  assert.deepEqual(flattenExplorer(graph,explorerExpanded,new Set()).map(r=>r.node.id),["p","f"]);
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
