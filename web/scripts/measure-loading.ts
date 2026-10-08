import { readFile } from "node:fs/promises";
import { performance } from "node:perf_hooks";
import { getGraphIndex } from "../lib/graphIndex.ts";
import { buildNodePresentations } from "../lib/languagePresentation.ts";
import { buildVisibleGraph, packageSourceCounts } from "../lib/presentation.ts";
import { declarationKinds, type Graph, type Overview, type PackageProjection } from "../lib/types.ts";

for (const id of process.argv.slice(2)) {
  const fullText=await readFile(`/tmp/nami-measure-${id}-full.json`,"utf8");
  const packagesText=await readFile(`/tmp/nami-measure-${id}-packages.json`,"utf8");
  const overviewText=await readFile(`/tmp/nami-measure-${id}-overview.json`,"utf8");
  for (const mode of ["full","overview"] as const) {
    const durations:number[]=[];
    let nodes=0,edges=0;
    for (let i=0;i<10;i++) {
      const start=performance.now();
      const overview:Overview=JSON.parse(overviewText);
      const graph:Graph=mode==="full"?JSON.parse(fullText):overview.graph;
      const projection:PackageProjection=mode==="full"?JSON.parse(packagesText):overview.package_projection;
      const index=getGraphIndex(graph);nodes=index.nodes.size;edges=graph.edges.length;
      buildNodePresentations(graph,packageSourceCounts(graph,mode==="overview"?overview.file_counts:undefined));
      buildVisibleGraph({canonicalGraph:graph,packageProjection:projection,expandedPackages:new Set(),expandedFiles:new Set(),visibleDeclarationKinds:new Set(declarationKinds)});
      durations.push(performance.now()-start);
    }
    durations.sort((a,b)=>a-b);
    console.log(`${id} ${mode}: ${nodes} canonical nodes, ${edges} edges; median parse/index/presentation ${durations[5].toFixed(2)} ms`);
  }
}
