import assert from "node:assert/strict";
import test from "node:test";
import { buildVisibleGraph, revealNode, type PresentationInput } from "./presentation.ts";
import { declarationKinds, type Graph, type PackageProjection } from "./types.ts";

const canonicalGraph: Graph = {
  nodes: [
    { id: "package:A", kind: "PACKAGE", name: "auth", path: "auth" },
    { id: "package:B", kind: "PACKAGE", name: "store", path: "store" },
    { id: "file:a.go", kind: "FILE", name: "a.go", path: "auth/a.go" },
    { id: "file:quiet.go", kind: "FILE", name: "quiet.go", path: "auth/quiet.go" },
    { id: "file:b.go", kind: "FILE", name: "b.go", path: "store/b.go" },
    { id: "function:a.go#Run", kind: "FUNCTION", name: "Run", path: "auth/a.go" },
    { id: "method:a.go#Service.Save", kind: "METHOD", name: "Service.Save", path: "auth/a.go" },
  ],
  edges: [
    { kind: "CONTAINS", from: "package:A", to: "file:a.go" },
    { kind: "CONTAINS", from: "package:A", to: "file:quiet.go" },
    { kind: "CONTAINS", from: "package:B", to: "file:b.go" },
    { kind: "CONTAINS", from: "file:a.go", to: "function:a.go#Run" },
    { kind: "CONTAINS", from: "file:a.go", to: "method:a.go#Service.Save" },
    { kind: "IMPORTS", from: "file:a.go", to: "package:B" },
  ],
};

const packageProjection: PackageProjection = {
  graph: {
    nodes: canonicalGraph.nodes.filter((node) => node.kind === "PACKAGE"),
    edges: [{ kind: "IMPORTS", from: "package:A", to: "package:B" }],
  },
  evidence: [{
    edge: { kind: "IMPORTS", from: "package:A", to: "package:B" },
    sources: [{ kind: "IMPORTS", from: "file:a.go", to: "package:B" }],
  }],
};

const input = (): PresentationInput => ({
  canonicalGraph,
  packageProjection,
  expandedPackages: new Set(),
  expandedFiles: new Set(),
  visibleDeclarationKinds: new Set(declarationKinds),
});

test("collapsed graph uses only backend package projection", () => {
  const graph = buildVisibleGraph(input());
  assert.deepEqual(graph.nodes.map((node) => node.id), ["package:A", "package:B"]);
  assert.deepEqual(graph.edges.map((edge) => [edge.kind, edge.source, edge.target]), [["IMPORTS", "package:A", "package:B"]]);
  assert.deepEqual(graph.edges[0].evidence, packageProjection.evidence[0].sources);
});

test("expansion shows canonical file and declaration children", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A"]);
  state.expandedFiles = new Set(["file:a.go"]);
  const graph = buildVisibleGraph(state);
  assert.deepEqual(graph.nodes.map((node) => node.id), [
    "file:a.go", "file:quiet.go", "function:a.go#Run", "method:a.go#Service.Save", "package:A", "package:B",
  ]);
  assert.equal(graph.edges.filter((edge) => edge.kind === "CONTAINS").length, 4);
  assert.ok(graph.edges.some((edge) => edge.source === "file:a.go" && edge.target === "package:B"));
  assert.ok(!graph.edges.some((edge) => edge.source === "package:A" && edge.target === "package:B"));
});

test("expanded target remains a package import target", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A", "package:B"]);
  const graph = buildVisibleGraph(state);
  const imports = graph.edges.filter((edge) => edge.kind === "IMPORTS");
  assert.deepEqual(imports.map((edge) => [edge.source, edge.target]), [["file:a.go", "package:B"]]);
  assert.ok(graph.nodes.some((node) => node.id === "file:b.go"));
});

test("declaration filters affect visibility only", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A"]);
  state.expandedFiles = new Set(["file:a.go"]);
  state.visibleDeclarationKinds = new Set(["METHOD"]);
  const filtered = buildVisibleGraph(state);
  assert.ok(!filtered.nodes.some((node) => node.kind === "FUNCTION"));
  assert.ok(filtered.nodes.some((node) => node.kind === "METHOD"));
  assert.deepEqual(canonicalGraph.nodes.filter((node) => node.kind === "FUNCTION").map((node) => node.name), ["Run"]);
  assert.deepEqual(filtered.edges.filter((edge) => edge.kind === "IMPORTS").map((edge) => edge.target), ["package:B"]);
});

test("search reveal expands required ancestors and enables the declaration kind", () => {
  const state = revealNode(canonicalGraph, "function:a.go#Run", {
    expandedPackages: new Set(), expandedFiles: new Set(), visibleDeclarationKinds: new Set(),
  });
  assert.deepEqual([...state.expandedPackages], ["package:A"]);
  assert.deepEqual([...state.expandedFiles], ["file:a.go"]);
  assert.deepEqual([...state.visibleDeclarationKinds], ["FUNCTION"]);
  const visible = buildVisibleGraph({ ...input(), ...state });
  assert.ok(visible.nodes.some((node) => node.id === "function:a.go#Run"));
});

test("identical facts and view state produce identical output", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A"]);
  state.expandedFiles = new Set(["file:a.go"]);
  assert.deepEqual(buildVisibleGraph(state), buildVisibleGraph(state));
});
