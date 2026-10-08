// Benchmark saved API facts; an optional baseline directory verifies exact output.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import path from 'node:path';
import * as current from '../lib/presentation.ts';
import { declarationKinds } from '../lib/types.ts';
const [graphPath, projectionPath, baselineDirectory] = process.argv.slice(2);
if (!graphPath || !projectionPath) throw new Error('Usage: node web/scripts/benchmark.mjs <graph.json> <packages.json> [baseline-lib-directory]');
const canonicalGraph = JSON.parse(fs.readFileSync(graphPath, 'utf8'));
const packageProjection = JSON.parse(fs.readFileSync(projectionPath, 'utf8'));
const baseline = baselineDirectory ? await import(pathToFileURL(path.join(baselineDirectory, 'presentation.ts')).href) : current;
const originalFacts = JSON.stringify({ canonicalGraph, packageProjection });
const packages = canonicalGraph.nodes.filter(node => node.kind === 'PACKAGE');
const files = canonicalGraph.nodes.filter(node => node.kind === 'FILE');
const exportUses = baseline.buildExportUseIndex(canonicalGraph);
const canonicalNodes = new Map(canonicalGraph.nodes.map(node => [node.id, node]));
const states = Array.from({ length: 16 }, (_, i) => ({ canonicalGraph, packageProjection,
  expandedPackages: new Set(packages.filter((_, j) => i === 15 || (i > 0 && j % 4 === i % 4)).map(node => node.id)),
  expandedFiles: new Set(files.filter((_, j) => i === 15 || (i > 5 && j % 5 === i % 5)).map(node => node.id)),
  visibleDeclarationKinds: new Set(i % 2 ? declarationKinds : ['CLASS', 'FUNCTION']),
}));
function linesFor(api, visible) {
  const nodes = new Map(visible.nodes.map(node => [node.id, node]));
  return visible.edges.filter(edge => edge.kind === 'IMPORTS').flatMap(edge => api.importCardLines(edge, nodes, exportUses, canonicalNodes));
}
const computationMs = { baseline: 0, current: 0 };
for (const state of states) {
  let start = performance.now();
  const oldGraph = baseline.buildVisibleGraph(state);
  const oldLines = linesFor(baseline, oldGraph);
  computationMs.baseline += performance.now() - start;
  start = performance.now();
  const newGraph = current.buildVisibleGraph(state);
  const newLines = linesFor(current, newGraph);
  computationMs.current += performance.now() - start;
  assert.deepEqual(newGraph, oldGraph);
  assert.deepEqual(newLines, oldLines);
  assert.deepEqual(current.buildExplorerTree(canonicalGraph, newGraph), baseline.buildExplorerTree(canonicalGraph, oldGraph));
}
assert.equal(JSON.stringify({ canonicalGraph, packageProjection }), originalFacts);
console.log(JSON.stringify({ states: states.length, canonicalNodes: canonicalGraph.nodes.length, canonicalEdges: canonicalGraph.edges.length,
  exactGraphAndLineEquality: true, computationMs }));
