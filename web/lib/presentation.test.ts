import { buildNodePresentations, presentationFor, sourceStatsTitle } from "./languagePresentation.ts";
import assert from "node:assert/strict";
import test from "node:test";
import { buildExplorerTree, buildExportUseIndex, buildVisibleGraph, directlyConnectedPackages, fileDependencyRoles, importCardLines, packageSourceCounts, revealNode, type PresentationInput } from "./presentation.ts";
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
    id: JSON.stringify([edge.id, "file:a.go", "file:b.go"]), canonicalEdgeID: edge.id, source: "package:B", sourceHandle: "file:b.go", target: "package:A", targetHandle: "file:a.go", importingFileID: "file:a.go", supplyingFileID: "file:b.go",
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

test("Go presentation keeps package cards and canonical file labels", () => {
  const pkg = { id: "package:auth", kind: "PACKAGE" as const, language: "go", name: "auth", path: "internal/auth" };
  const file = { id: "file:auth.go", kind: "FILE" as const, language: "go", name: "auth.go", path: "internal/auth/auth.go", import_count: 2, export_count: 3 };
  const container = presentationFor(pkg, undefined, { imports: 2, exports: 3 });
  assert.equal(container.primaryCard, true);
  assert.equal(container.cardName, "internal/auth");
  assert.equal(container.secondaryLabel, "package auth");
  assert.equal(container.rowGroupLabel, "Files");
  assert.deepEqual(container.sourceStats, { imports: 2, exports: 3 });
  const source = presentationFor(file, pkg);
  assert.equal(source.displayKind, "file");
  assert.equal(source.displayName, "auth.go");
  assert.equal(source.primaryCard, false);
  assert.deepEqual(source.sourceStats, { imports: 2, exports: 3 });
  assert.equal(presentationFor(file).explorerSection, undefined);
  assert.equal(presentationFor({ ...pkg, kind: "MODULE" }).displayKind, "module");
});

test("Python profiles present modules without changing canonical file kinds", () => {
  const pkg = { id: "python-package:users", kind: "PACKAGE" as const, language: "python", name: "users", path: "users" };
  const file = { id: "file:users/service.py", kind: "FILE" as const, language: "python", name: "service.py", path: "users/service.py", import_count: 2 };
  const profile = presentationFor(file, pkg);
  assert.equal(file.kind, "FILE");
  assert.equal(profile.displayKind, "module");
  assert.equal(profile.displayName, "users.service");
  assert.equal(profile.rowName, "service");
  assert.equal(profile.primaryCard, false);
  assert.equal(presentationFor(pkg).primaryCard, true);
  assert.equal(presentationFor(pkg).rowGroupLabel, "Modules");
  assert.equal(presentationFor({ ...file, name: "__init__.py" }, pkg).displayName, "users");
  const standalone = presentationFor({ ...file, name: "standalone.py", path: "standalone.py" });
  assert.equal(standalone.displayName, "standalone");
  assert.equal(standalone.primaryCard, true);
  assert.equal(standalone.explorerSection, "modules");
  assert.equal(sourceStatsTitle(profile.sourceStats), "2 imports, unknown exports");
  assert.equal(profile.sourceStats.exports, undefined);
  assert.equal(presentationFor(pkg, undefined, { imports: 0, exports: 0 }).sourceStats.exports, undefined);
});

test("unknown language combinations retain canonical labels without guessed card semantics", () => {
  const file = { id: "file:unknown.py", kind: "FILE" as const, name: "unknown.py", path: "unknown.py" };
  for (const node of [file, { ...file, language: "go" }, { ...file, language: "unknown" }]) {
    const profile = presentationFor(node);
    assert.equal(profile.displayKind, "file");
    assert.equal(profile.explorerSection, undefined);
    assert.equal(profile.primaryCard, false);
  }
  assert.equal(presentationFor({ ...file, kind: "MODULE", language: "python" }).expansion, undefined);
  assert.equal(presentationFor({ ...file, kind: "CLASS", language: "go" }).expansion, undefined);
});

const nestedPythonGraph: Graph = {
  nodes: [
    { id: "python-package:users", kind: "PACKAGE", language: "python", name: "users", path: "users" },
    { id: "python-package:users/admin", kind: "PACKAGE", language: "python", name: "users.admin", path: "users/admin" },
    { id: "file:users/admin/service.py", kind: "FILE", language: "python", name: "service.py", path: "users/admin/service.py", import_count: 2 },
    { id: "file:users/models.py", kind: "FILE", language: "python", name: "models.py", path: "users/models.py", import_count: 0 },
    { id: "file:users/service.py", kind: "FILE", language: "python", name: "service.py", path: "users/service.py", import_count: 0 },
    { id: "file:standalone.py", kind: "FILE", language: "python", name: "standalone.py", path: "standalone.py", import_count: 0 },
  ],
  edges: [
    { kind: "CONTAINS", from: "python-package:users", to: "python-package:users/admin" },
    { kind: "CONTAINS", from: "python-package:users/admin", to: "file:users/admin/service.py" },
    { kind: "CONTAINS", from: "python-package:users", to: "file:users/models.py" },
    { kind: "CONTAINS", from: "python-package:users", to: "file:users/service.py" },
    { kind: "IMPORTS", from: "file:users/admin/service.py", to: "file:users/models.py" },
    { kind: "IMPORTS", from: "file:users/admin/service.py", to: "file:users/service.py" },
  ],
};
const nestedProjection: PackageProjection = {
  graph: { nodes: nestedPythonGraph.nodes.filter((node) => node.kind === "PACKAGE"), edges: [{ kind: "IMPORTS", from: "python-package:users/admin", to: "python-package:users" }] },
  evidence: [{ edge: { kind: "IMPORTS", from: "python-package:users/admin", to: "python-package:users" }, sources: nestedPythonGraph.edges.slice(4) }],
};
const nestedInput = (): PresentationInput => ({ canonicalGraph: nestedPythonGraph, packageProjection: nestedProjection, expandedPackages: new Set(["python-package:users"]), expandedFiles: new Set(), visibleDeclarationKinds: new Set(declarationKinds) });

test("nested Python packages stay nested in Explorer and independent in the graph", () => {
  const visible = buildVisibleGraph(nestedInput());
  const tree = buildExplorerTree(nestedPythonGraph, visible);
  assert.deepEqual(tree.packages.map((node) => node.id), ["python-package:users"]);
  assert.deepEqual(tree.children.get("python-package:users")?.map((node) => node.id), ["file:users/models.py", "file:users/service.py", "python-package:users/admin"]);
  assert.deepEqual(tree.modules.map((node) => node.id), ["file:standalone.py"]);
  assert.equal(visible.nodes.find((node) => node.id === "python-package:users/admin")?.parentId, undefined);
  const profiles = buildNodePresentations(nestedPythonGraph);
  assert.equal(profiles.get("python-package:users/admin")?.rowName, "admin");
  assert.equal(profiles.get("file:users/admin/service.py")?.displayName, "users.admin.service");
});

test("duplicate projected imports produce exactly one line per importer-provider pair", () => {
  const visible = buildVisibleGraph(nestedInput());
  const nodes = new Map(visible.nodes.map((node) => [node.id, node]));
  const edge = visible.edges.find((edge) => edge.kind === "IMPORTS")!;
  const lines = importCardLines(edge, nodes, buildExportUseIndex(nestedPythonGraph));
  assert.equal(lines.length, 2);
  assert.equal(new Set(lines.map((line) => line.id)).size, 2);
  assert.deepEqual(lines.map((line) => line.supplyingFileID), ["file:users/models.py", "file:users/service.py"]);
  assert.ok(lines.every((line) => line.importingFileID === "file:users/admin/service.py"));
  assert.deepEqual(importCardLines(edge, nodes, buildExportUseIndex(nestedPythonGraph)), lines);
  assert.deepEqual(importCardLines({ ...edge, evidence: [...edge.evidence].reverse().concat(edge.evidence) }, nodes, buildExportUseIndex(nestedPythonGraph)), lines);
  const collapsed = buildVisibleGraph({ ...nestedInput(), expandedPackages: new Set() });
  const collapsedEdge = collapsed.edges.find((edge) => edge.kind === "IMPORTS")!;
  const collapsedLines = importCardLines(collapsedEdge, new Map(collapsed.nodes.map((node) => [node.id, node])), buildExportUseIndex(nestedPythonGraph), new Map(nestedPythonGraph.nodes.map((node) => [node.id, node])));
  assert.equal(collapsedLines.length, 2);
  assert.equal(new Set(collapsedLines.map((line) => line.id)).size, 2);
  assert.deepEqual(collapsedLines.map((line) => line.supplyingFileID), ["file:users/models.py", "file:users/service.py"]);
  assert.ok(collapsedLines.every((line) => line.sourceHandle === undefined));
  const expanded = buildVisibleGraph({ ...nestedInput(), expandedPackages: new Set(["python-package:users", "python-package:users/admin"]) });
  const expandedNodes = new Map(expanded.nodes.map((node) => [node.id, node]));
  const expandedLines = expanded.edges.filter((edge) => edge.kind === "IMPORTS").flatMap((edge) => importCardLines(edge, expandedNodes, buildExportUseIndex(nestedPythonGraph)));
  assert.equal(expandedLines.length, 2);
  assert.equal(new Set(expandedLines.map((line) => line.id)).size, 2);
});

test("different importers sharing providers keep unique deterministic line IDs", () => {
  const visible = buildVisibleGraph(nestedInput());
  const nodes = new Map(visible.nodes.map((node) => [node.id, node]));
  const edge = visible.edges.find((edge) => edge.kind === "IMPORTS")!;
  const evidence = [...edge.evidence, ...edge.evidence.map((fact) => ({ ...fact, from: "file:users/admin/other.py" }))];
  const lines = importCardLines({ ...edge, evidence }, nodes, buildExportUseIndex(nestedPythonGraph));
  assert.equal(lines.length, 4);
  assert.equal(new Set(lines.map((line) => line.id)).size, 4);
  assert.deepEqual(importCardLines({ ...edge, evidence: [...evidence].reverse() }, nodes, buildExportUseIndex(nestedPythonGraph)), lines);
});

test("unprojected standalone module imports remain canonical presentation facts", () => {
  const canonical: Graph = { ...nestedPythonGraph, edges: [...nestedPythonGraph.edges, { kind: "IMPORTS", from: "file:standalone.py", to: "file:users/models.py" }] };
  const visible = buildVisibleGraph({ ...nestedInput(), canonicalGraph: canonical });
  const dependency = visible.edges.find((edge) => edge.source === "file:standalone.py");
  assert.ok(dependency);
  assert.deepEqual(dependency.evidence, [{ kind: "IMPORTS", from: "file:standalone.py", to: "file:users/models.py" }]);
  assert.equal(dependency.projectionEdge, undefined);
  const lines = importCardLines(dependency, new Map(visible.nodes.map((node) => [node.id, node])), buildExportUseIndex(canonical));
  assert.equal(lines.length, 1);
  assert.equal(lines[0].sourceHandle, "file:users/models.py");
  assert.equal(lines[0].target, "file:standalone.py");
});
