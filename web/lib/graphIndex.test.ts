import assert from "node:assert/strict";
import test from "node:test";
import { getGraphIndex } from "./graphIndex.ts";
import { buildNodePresentations } from "./languagePresentation.ts";
import { buildVisibleGraph, importCardLines, occludedCardLineIDs, type CardImportLine } from "./presentation.ts";
import { declarationKinds, type Graph } from "./types.ts";

const graph: Graph = {
  nodes: [
    { id: "p", name: "users", path: "users", kind: "PACKAGE", language: "python" },
    { id: "b", name: "b.py", path: "users/b.py", kind: "FILE", language: "python" },
    { id: "a", name: "a.py", path: "users/a.py", kind: "FILE", language: "python" },
    { id: "d", name: "run", path: "users/a.py", kind: "FUNCTION", language: "python" },
  ],
  edges: [
    { kind: "CONTAINS", from: "p", to: "b" },
    { kind: "CONTAINS", from: "p", to: "a" },
    { kind: "CONTAINS", from: "a", to: "d" },
    { kind: "IMPORTS", from: "a", to: "b" },
  ],
};

test("a snapshot shares indexes and a replacement snapshot cannot reuse stale facts", () => {
  const factsBefore = JSON.stringify(graph);
  const index = getGraphIndex(graph);
  assert.strictEqual(getGraphIndex(graph), index);
  assert.deepEqual(index.children.get("p")?.map(node => node.id), ["a", "b"]);
  assert.equal(index.parent.get("d"), "a");
  assert.deepEqual(index.outgoing.get("a"), ["b"]);
  assert.deepEqual(index.incoming.get("b"), ["a"]);
  assert.deepEqual(index.counts, { packages: 1, files: 2, declarations: 1 });
  const replacement: Graph = { nodes: graph.nodes, edges: [] };
  assert.notStrictEqual(getGraphIndex(replacement), index);
  assert.equal(getGraphIndex(replacement).parent.size, 0);
  assert.equal(JSON.stringify(graph), factsBefore);
});

test("cached presentations separate source-stat variants and snapshots", () => {
  const plain = buildNodePresentations(graph);
  assert.strictEqual(buildNodePresentations(graph), plain);
  const counts = new Map([["p", { imports: 4 }]]);
  const counted = buildNodePresentations(graph, counts);
  assert.strictEqual(buildNodePresentations(graph, counts), counted);
  assert.equal(counted.get("p")?.sourceStats.imports, 4);
  assert.equal(plain.get("p")?.sourceStats.imports, undefined);
  assert.notStrictEqual(buildNodePresentations({ ...graph }), plain);
});

test("index reuse across expansion retains facts and line identities", () => {
  const before = JSON.stringify(graph);
  const input = { canonicalGraph: graph, packageProjection: { graph: { nodes: [], edges: [] }, evidence: [] }, expandedPackages: new Set<string>(), expandedFiles: new Set<string>(), visibleDeclarationKinds: new Set(declarationKinds) };
  const collapsed = buildVisibleGraph(input);
  const expanded = buildVisibleGraph({ ...input, expandedPackages: new Set(["p"]), expandedFiles: new Set(["a"]) });
  assert.ok(expanded.nodes.some(node => node.id === "d"));
  assert.deepEqual(buildVisibleGraph(input), collapsed);
  for (const visible of [collapsed, expanded]) {
    const nodes = new Map(visible.nodes.map(node => [node.id, node]));
    const lines = visible.edges.filter(edge => edge.kind === "IMPORTS").flatMap(edge => importCardLines(edge, nodes, new Map(), getGraphIndex(graph).nodes));
    assert.equal(new Set(lines.map(line => line.id)).size, lines.length);
  }
  assert.equal(JSON.stringify(graph), before);
});

const lines: CardImportLine[] = [
  { id: '["canonical","a","b"]', canonicalEdgeID: "canonical", source: "supplier", target: "importer", importingFileID: "a", supplyingFileID: "b" },
  { id: '["canonical","a","c"]', canonicalEdgeID: "canonical", source: "supplier", target: "importer", importingFileID: "a", supplyingFileID: "c" },
];

test("folded route paints the previous topmost line without deleting or renaming evidence", () => {
  const before = JSON.stringify(lines);
  const hidden = occludedCardLineIDs(lines);
  assert.deepEqual([...hidden], [lines[0].id]);
  assert.deepEqual(lines.filter(line => !hidden.has(line.id)), [lines[1]]);
  assert.equal(JSON.stringify(lines), before);
  assert.deepEqual(occludedCardLineIDs(lines), hidden);
});

test("expansion restores distinct file routes and unrelated canonical IDs never coalesce", () => {
  const expanded = lines.map(line => ({ ...line, sourceHandle: line.supplyingFileID, targetHandle: line.importingFileID }));
  assert.equal(occludedCardLineIDs(expanded).size, 0);
  assert.deepEqual(expanded.map(line => line.id), lines.map(line => line.id));
  assert.equal(occludedCardLineIDs(lines.map((line, i) => ({ ...line, canonicalEdgeID: `canonical:${i}` }))).size, 0);
});
