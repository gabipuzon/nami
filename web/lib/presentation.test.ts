import assert from "node:assert/strict";
import test from "node:test";
import { buildExportUseIndex, buildVisibleGraph, directlyConnectedPackages, fileDependencyRoles, importCardLines, packageSourceCounts, revealNode, type PresentationInput } from "./presentation.ts";
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
    { kind: "USES_EXPORT", from: "file:a.go", to: "file:b.go" },
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

test("only known imports and export uses mark file rows as connected", () => {
  const roles = fileDependencyRoles(canonicalGraph);
  assert.deepEqual([...roles.importing], ["file:a.go"]);
  assert.deepEqual([...roles.supplying], ["file:b.go"]);
  assert.ok(!roles.importing.has("file:quiet.go") && !roles.supplying.has("file:quiet.go"));
});

test("folded package counts sum saved source counts without filling gaps", () => {
  const graph: Graph = { ...canonicalGraph, nodes: canonicalGraph.nodes.map((node) => node.id === "file:a.go" ? { ...node, import_count: 2, export_count: 3 } : node.id === "file:quiet.go" ? { ...node, import_count: 1, export_count: 0 } : node) };
  assert.deepEqual(packageSourceCounts(graph).get("package:A"), { imports: 3, exports: 3 });
  assert.deepEqual(packageSourceCounts(canonicalGraph).get("package:A"), { imports: undefined, exports: undefined });
  const partial: Graph = { ...graph, nodes: graph.nodes.map((node) => node.id === "file:quiet.go" ? { ...node, export_count: undefined } : node) };
  assert.deepEqual(packageSourceCounts(partial).get("package:A"), { imports: 3, exports: undefined });
});

test("focus keeps only direct package neighbors", () => {
  const graph = buildVisibleGraph(input());
  assert.deepEqual([...directlyConnectedPackages(graph, "package:A")].sort(), ["package:A", "package:B"]);
  const isolated = { ...graph, nodes: [...graph.nodes, { id: "package:C", kind: "PACKAGE" as const, name: "other", path: "other", childCount: 0, expanded: false }] };
  assert.ok(!directlyConnectedPackages(isolated, "package:A").has("package:C"));
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

test("visual import lines enter the importing file without changing saved evidence", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A"]);
  const expanded = buildVisibleGraph(state);
  const nodes = new Map(expanded.nodes.map((node) => [node.id, node]));
  const edge = expanded.edges.find((item) => item.kind === "IMPORTS");
  assert.ok(edge);
  assert.deepEqual(importCardLines(edge, nodes, buildExportUseIndex(canonicalGraph)), [{
    id: edge.id, canonicalEdgeID: edge.id, source: "package:B", target: "package:A", targetHandle: "file:a.go", importingFileID: "file:a.go",
  }]);
  assert.deepEqual(edge.evidence, [{ kind: "IMPORTS", from: "file:a.go", to: "package:B" }]);
  const collapsed = buildVisibleGraph(input());
  assert.deepEqual(importCardLines(collapsed.edges[0], new Map(collapsed.nodes.map((node) => [node.id, node])), buildExportUseIndex(canonicalGraph)), [{
    id: collapsed.edges[0].id, canonicalEdgeID: collapsed.edges[0].id, source: "package:B", target: "package:A", targetHandle: undefined, importingFileID: "file:a.go",
  }]);
});

test("saved export-use evidence anchors a dependency line to its supplying file", () => {
  const state = input();
  state.expandedPackages = new Set(["package:A", "package:B"]);
  const expanded = buildVisibleGraph(state);
  const nodes = new Map(expanded.nodes.map((node) => [node.id, node]));
  const edge = expanded.edges.find((item) => item.kind === "IMPORTS");
  assert.ok(edge);
  assert.deepEqual(importCardLines(edge, nodes, buildExportUseIndex(canonicalGraph)), [{
    id: `${edge.id}|file:a.go|file:b.go`, canonicalEdgeID: edge.id, source: "package:B", sourceHandle: "file:b.go", target: "package:A", targetHandle: "file:a.go", importingFileID: "file:a.go", supplyingFileID: "file:b.go",
  }]);
  const extraFile = { id: "file:extra.go", kind: "FILE" as const, name: "extra.go", path: "store/extra.go", parentId: "package:B", childCount: 0, expanded: false };
  nodes.set(extraFile.id, extraFile);
  assert.equal(importCardLines(edge, nodes, buildExportUseIndex(canonicalGraph))[0].sourceHandle, "file:b.go");
  const twoProviders: Graph = { ...canonicalGraph, nodes: [...canonicalGraph.nodes, extraFile], edges: [
    ...canonicalGraph.edges,
    { kind: "CONTAINS", from: "package:B", to: extraFile.id },
    { kind: "USES_EXPORT", from: "file:a.go", to: extraFile.id },
  ] };
  assert.deepEqual(importCardLines(edge, nodes, buildExportUseIndex(twoProviders)).map((line) => line.sourceHandle), ["file:b.go", "file:extra.go"]);
  const oldGraph = { ...canonicalGraph, edges: canonicalGraph.edges.filter((item) => item.kind !== "USES_EXPORT") };
  assert.equal(importCardLines(edge, nodes, buildExportUseIndex(oldGraph))[0].sourceHandle, undefined);
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

test("Python module imports keep canonical evidence and attach to supplying file rows", () => {
  const pythonGraph: Graph = {
    nodes: [
      { id: "python-package:users", kind: "PACKAGE", language: "python", name: "users", path: "users" },
      { id: "python-package:core", kind: "PACKAGE", language: "python", name: "core", path: "core" },
      { id: "file:users/service.py", kind: "FILE", language: "python", name: "service.py", path: "users/service.py", import_count: 1 },
      { id: "file:core/models.py", kind: "FILE", language: "python", name: "models.py", path: "core/models.py", import_count: 0 },
      { id: "class:core/models.py#User", kind: "CLASS", language: "python", name: "User", path: "core/models.py" },
    ],
    edges: [
      { kind: "CONTAINS", from: "python-package:users", to: "file:users/service.py" },
      { kind: "CONTAINS", from: "python-package:core", to: "file:core/models.py" },
      { kind: "CONTAINS", from: "file:core/models.py", to: "class:core/models.py#User" },
      { kind: "IMPORTS", from: "file:users/service.py", to: "file:core/models.py" },
    ],
  };
  const edge = { kind: "IMPORTS" as const, from: "python-package:users", to: "python-package:core" };
  const projection: PackageProjection = { graph: { nodes: pythonGraph.nodes.filter((node) => node.kind === "PACKAGE"), edges: [edge] }, evidence: [{ edge, sources: [pythonGraph.edges[3]] }] };
  const visible = buildVisibleGraph({ canonicalGraph: pythonGraph, packageProjection: projection, expandedPackages: new Set([edge.from, edge.to]), expandedFiles: new Set(["file:core/models.py"]), visibleDeclarationKinds: new Set(declarationKinds) });
  const dependency = visible.edges.find((edge) => edge.kind === "IMPORTS")!;
  assert.deepEqual(dependency.evidence, [pythonGraph.edges[3]]);
  assert.equal(dependency.target, "python-package:core");
  const lines = importCardLines(dependency, new Map(visible.nodes.map((node) => [node.id, node])), buildExportUseIndex(pythonGraph));
  assert.equal(lines[0].sourceHandle, "file:core/models.py");
  assert.equal(lines[0].targetHandle, "file:users/service.py");
  assert.ok(visible.nodes.some((node) => node.kind === "CLASS"));
  assert.deepEqual(packageSourceCounts(pythonGraph).get("python-package:core"), { imports: 0, exports: undefined });
  assert.ok(fileDependencyRoles(pythonGraph).supplying.has("file:core/models.py"));
});

test("standalone Python files and declarations render without synthetic packages", () => {
  const graph: Graph = {
    nodes: [
      { id: "file:tool.py", kind: "FILE", language: "python", path: "tool.py", name: "tool.py" },
      { id: "class:tool.py#Tool", kind: "CLASS", language: "python", path: "tool.py", name: "Tool" },
    ],
    edges: [{ kind: "CONTAINS", from: "file:tool.py", to: "class:tool.py#Tool" }],
  };
  const state = revealNode(graph, "class:tool.py#Tool", { expandedPackages: new Set(), expandedFiles: new Set(), visibleDeclarationKinds: new Set() });
  const visible = buildVisibleGraph({ canonicalGraph: graph, packageProjection: { graph: { nodes: [], edges: [] }, evidence: [] }, ...state });
  assert.equal(visible.nodes.length, 2);
  assert.equal(visible.nodes.find((node) => node.kind === "FILE")?.parentId, undefined);
  assert.equal(visible.nodes.find((node) => node.kind === "CLASS")?.parentId, "file:tool.py");
});
