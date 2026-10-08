package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
	"github.com/gabipuzon/nami/internal/impact"
	"github.com/gabipuzon/nami/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	packageA = "package:a#a"
	packageB = "package:b#b"
	packageC = "package:c#c"
	packageD = "package:d#d"
	fileA    = "file:a/a.go"
	fileAZ   = "file:a/z.go"
	fileB    = "file:b/b.go"
)

func fixture(t *testing.T) storage.Snapshot {
	t.Helper()
	nodes := []graph.Node{{ID: "module:test", Kind: graph.Module, Name: "test", Path: "."}}
	edges := []graph.Edge{}
	for _, item := range []struct{ id, name string }{{packageD, "d"}, {packageC, "c"}, {packageB, "b"}, {packageA, "a"}} {
		nodes = append(nodes, graph.Node{ID: item.id, Kind: graph.Package, Name: item.name, Path: item.name})
		edges = append(edges, graph.Edge{Kind: graph.Contains, From: "module:test", To: item.id})
	}
	for _, item := range []struct{ id, parent, path string }{{fileAZ, packageA, "a/z.go"}, {fileB, packageB, "b/b.go"}, {fileA, packageA, "a/a.go"}} {
		nodes = append(nodes, graph.Node{ID: item.id, Kind: graph.File, Name: filepath.Base(item.path), Path: item.path, ImportCount: 2, ExportCount: 3, HasImportCount: true, HasExportCount: true})
		edges = append(edges, graph.Edge{Kind: graph.Contains, From: item.parent, To: item.id})
	}
	for _, kind := range []graph.NodeKind{graph.Function, graph.Method, graph.Struct, graph.Interface, graph.Type, graph.Variable, graph.Constant} {
		id := "symbol:" + string(kind)
		nodes = append(nodes, graph.Node{ID: id, Kind: kind, Name: "Build", Path: "a/a.go"})
		edges = append(edges, graph.Edge{Kind: graph.Contains, From: fileA, To: id})
	}
	edges = append(edges,
		graph.Edge{Kind: graph.Imports, From: fileAZ, To: packageB},
		graph.Edge{Kind: graph.Imports, From: fileA, To: packageB},
		graph.Edge{Kind: graph.Imports, From: fileB, To: packageC},
		graph.Edge{Kind: graph.UsesExport, From: fileA, To: fileB},
	)
	g, err := graph.Build(graph.Fragment{Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	return storage.Snapshot{ID: "saved-test", Root: "/saved/repository", CreatedAt: time.Date(2026, 10, 8, 3, 4, 5, 123, time.UTC), Status: "completed_with_gaps",
		Result: analysis.Result{Graph: g, Coverage: analysis.Coverage{Status: "completed_with_gaps", FilesDiscovered: 8, SupportedSourceFiles: 3,
			FilesAnalyzed: 3, FilesSkipped: 4, FilesFailed: 1, ImportsDiscovered: 9, InternalResolved: 3, StandardLibrary: 2,
			External: 1, Unresolved: 1, Cgo: 1, Unclassified: 1}, Issues: []analysis.Issue{{Kind: "UNRESOLVED_IMPORT", Path: "a/a.go", Import: "missing", Reason: "not in snapshot"}}}}
}

func connect(t *testing.T, server *mcp.Server) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		ss.Close()
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cs.Close(); err != nil {
			t.Error(err)
		}
		if err := ss.Wait(); err != nil {
			t.Error(err)
		}
		cancel()
	})
	return ctx, cs
}

func newSession(t *testing.T) (context.Context, *mcp.ClientSession, storage.Snapshot) {
	t.Helper()
	snapshot := fixture(t)
	server, err := New(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server)
	return ctx, cs, snapshot
}

func call(t *testing.T, ctx context.Context, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s protocol error: %v", name, err)
	}
	if result.StructuredContent == nil {
		t.Fatalf("%s omitted structured output: %+v", name, result)
	}
	return result
}

func domainJSON(t *testing.T, result *mcp.CallToolResult) []byte {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is not text: %T", result.Content[0])
	}
	var fallback any
	if err := json.Unmarshal([]byte(text.Text), &fallback); err != nil {
		t.Fatal(err)
	}
	other, err := json.Marshal(fallback)
	if err != nil || !bytes.Equal(data, other) {
		t.Fatalf("structured output and JSON fallback differ: %s / %s", data, other)
	}
	return data
}

func decode[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", domainJSON(t, result))
	}
	var output T
	if err := json.Unmarshal(domainJSON(t, result), &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestDiscoveryAndScanInfo(t *testing.T) {
	ctx, cs, snapshot := newSession(t)
	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool annotations = %+v", tool)
		}
		if tool.InputSchema == nil {
			t.Fatalf("missing input schema: %s", tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"nami_dependency_path", "nami_file_symbols", "nami_inspect_node", "nami_package_dependencies", "nami_package_dependents", "nami_package_impact", "nami_scan_info", "nami_search_nodes"}
	if !reflect.DeepEqual(names, want) || listed.NextCursor != "" {
		t.Fatalf("tools = %v", names)
	}
	info := decode[scanInfoJSON](t, call(t, ctx, cs, "nami_scan_info", nil))
	if info.ID != snapshot.ID || info.Root != snapshot.Root || info.CreatedAt != snapshot.CreatedAt.Format(time.RFC3339Nano) || info.Status != snapshot.Status {
		t.Fatalf("scan metadata changed: %+v", info)
	}
	wantCoverage := coverageJSON{8, 3, 3, 4, 1, 9, 3, 2, 1, 1, 1, 1}
	if info.Coverage != wantCoverage || !reflect.DeepEqual(info.Issues, []issueJSON{{"UNRESOLVED_IMPORT", "a/a.go", "missing", "not in snapshot"}}) {
		t.Fatalf("stored coverage or issues changed: %+v", info)
	}
}

func TestSearchInspectionAndSymbols(t *testing.T) {
	ctx, cs, _ := newSession(t)
	search := decode[struct{ Nodes []nodeJSON }](t, call(t, ctx, cs, "nami_search_nodes", map[string]any{"query": "BUILD", "kinds": []string{"FUNCTION", "STRUCT"}, "limit": 1}))
	if len(search.Nodes) != 1 || search.Nodes[0].ID != "symbol:FUNCTION" {
		t.Fatalf("search = %+v", search)
	}
	inspection := decode[struct {
		Node     nodeJSON
		Parent   *nodeJSON
		Children []nodeJSON
		Incoming []edgeJSON `json:"incoming_edges"`
		Outgoing []edgeJSON `json:"outgoing_edges"`
	}](t, call(t, ctx, cs, "nami_inspect_node", map[string]any{"node_id": fileA}))
	if inspection.Node.ID != fileA || inspection.Parent == nil || inspection.Parent.ID != packageA || len(inspection.Children) != 7 || inspection.Node.ImportCount == nil || *inspection.Node.ImportCount != 2 {
		t.Fatalf("inspection = %+v", inspection)
	}
	if !reflect.DeepEqual(inspection.Incoming, []edgeJSON{{graph.Contains, packageA, fileA}}) {
		t.Fatalf("incoming = %+v", inspection.Incoming)
	}
	if len(inspection.Outgoing) != 9 || inspection.Outgoing[7] != (edgeJSON{graph.Imports, fileA, packageB}) || inspection.Outgoing[8] != (edgeJSON{graph.UsesExport, fileA, fileB}) {
		t.Fatalf("canonical facts changed: %+v", inspection.Outgoing)
	}
	symbols := decode[struct {
		File    nodeJSON
		Symbols []nodeJSON
	}](t, call(t, ctx, cs, "nami_file_symbols", map[string]any{"file_id": fileA}))
	if symbols.File.ID != fileA || !reflect.DeepEqual(symbols.Symbols, inspection.Children) {
		t.Fatalf("symbols = %+v", symbols)
	}
	for _, symbol := range symbols.Symbols {
		if !graph.IsDeclaration(symbol.Kind) {
			t.Fatalf("nondeclaration returned: %+v", symbol)
		}
	}
}

func TestProjectedRelationshipsAndImpact(t *testing.T) {
	ctx, cs, snapshot := newSession(t)
	evidence := []edgeJSON{{graph.Imports, fileA, packageB}, {graph.Imports, fileAZ, packageB}}
	deps := decode[struct {
		Target       nodeJSON
		Dependencies []dependencyJSON
	}](t, call(t, ctx, cs, "nami_package_dependencies", map[string]any{"package_id": packageA}))
	if deps.Target.ID != packageA || len(deps.Dependencies) != 1 || deps.Dependencies[0].Package.ID != packageB || !reflect.DeepEqual(deps.Dependencies[0].Evidence, evidence) {
		t.Fatalf("dependencies = %+v", deps)
	}
	dependents := decode[struct {
		Target     nodeJSON
		Dependents []dependencyJSON
	}](t, call(t, ctx, cs, "nami_package_dependents", map[string]any{"package_id": packageB}))
	if dependents.Target.ID != packageB || len(dependents.Dependents) != 1 || dependents.Dependents[0].Package.ID != packageA || !reflect.DeepEqual(dependents.Dependents[0].Evidence, evidence) {
		t.Fatalf("dependents = %+v", dependents)
	}
	result := decode[struct {
		Target   nodeJSON
		Affected []struct {
			Package  nodeJSON
			Distance int
		}
		Graph          graphJSON
		CoverageStatus string `json:"coverage_status"`
		Incomplete     bool
	}](t, call(t, ctx, cs, "nami_package_impact", map[string]any{"package_id": packageC}))
	projection, err := hierarchy.ProjectPackages(snapshot.Result.Graph)
	if err != nil {
		t.Fatal(err)
	}
	want, err := impact.Analyze(projection.Graph, packageC)
	if err != nil {
		t.Fatal(err)
	}
	if result.Target.ID != want.Target || len(result.Affected) != len(want.Affected) || !result.Incomplete || result.CoverageStatus != snapshot.Status {
		t.Fatalf("impact = %+v", result)
	}
	for i, item := range result.Affected {
		if item.Package.ID != want.Affected[i].ID || item.Distance != want.Affected[i].Distance {
			t.Fatalf("impact distance = %+v, want %+v", item, want.Affected[i])
		}
	}
	wantGraph := convertGraph(want.Graph)
	if !reflect.DeepEqual(result.Graph, wantGraph) {
		t.Fatalf("impact graph = %+v, want %+v", result.Graph, wantGraph)
	}
}

func TestPathsUseExistingImportsSemantics(t *testing.T) {
	ctx, cs, _ := newSession(t)
	for _, test := range []struct {
		name, from, to, scope string
		path                  []string
	}{
		{"canonical", fileA, packageB, "canonical", []string{fileA, packageB}},
		{"package chain", packageA, packageC, "package", []string{packageA, packageB, packageC}},
		{"no path", packageC, packageA, "package", []string{}},
		{"same package", packageA, packageA, "package", []string{packageA}},
		{"same canonical", fileA, fileA, "canonical", []string{fileA}},
		{"no containment traversal", packageA, fileA, "canonical", []string{}},
		{"no export use traversal", fileA, fileB, "canonical", []string{}},
		{"no implicit package traversal", fileA, packageC, "canonical", []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := decode[struct {
				Found bool
				Path  []string
			}](t, call(t, ctx, cs, "nami_dependency_path", map[string]any{"from_id": test.from, "to_id": test.to, "scope": test.scope}))
			if result.Found != (len(test.path) > 0) || !reflect.DeepEqual(result.Path, test.path) {
				t.Fatalf("path = %+v, want %v", result, test.path)
			}
		})
	}
}

func TestInvalidCallsAreStableToolErrors(t *testing.T) {
	ctx, cs, _ := newSession(t)
	for _, test := range []struct {
		name string
		args any
		code string
	}{
		{"nami_inspect_node", map[string]any{}, "invalid_argument"},
		{"nami_inspect_node", map[string]any{"node_id": " "}, "invalid_argument"},
		{"nami_inspect_node", map[string]any{"node_id": "missing"}, "node_not_found"},
		{"nami_inspect_node", map[string]any{"node_id": 12}, "invalid_argument"},
		{"nami_inspect_node", map[string]any{"NODE_ID": fileA}, "invalid_argument"},
		{"nami_inspect_node", map[string]any{"node_id": fileA, "extra": true}, "invalid_argument"},
		{"nami_file_symbols", map[string]any{}, "invalid_argument"},
		{"nami_file_symbols", map[string]any{"file_id": packageA}, "wrong_node_kind"},
		{"nami_file_symbols", map[string]any{"file_id": "missing"}, "node_not_found"},
		{"nami_package_dependencies", map[string]any{}, "invalid_argument"},
		{"nami_package_dependencies", map[string]any{"package_id": fileA}, "wrong_node_kind"},
		{"nami_package_dependents", map[string]any{"package_id": fileA}, "wrong_node_kind"},
		{"nami_package_impact", map[string]any{"package_id": fileA}, "wrong_node_kind"},
		{"nami_package_impact", map[string]any{"package_id": "missing"}, "node_not_found"},
		{"nami_package_impact", map[string]any{}, "invalid_argument"},
		{"nami_dependency_path", map[string]any{"from_id": packageA, "to_id": packageB, "scope": "unknown"}, "invalid_argument"},
		{"nami_dependency_path", map[string]any{"from_id": fileA, "to_id": packageB, "scope": "package"}, "wrong_node_kind"},
		{"nami_dependency_path", map[string]any{"from_id": packageA, "to_id": "missing", "scope": "package"}, "node_not_found"},
		{"nami_dependency_path", map[string]any{"scope": "canonical"}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "limit": 101}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "limit": 0}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "limit": -1}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "limit": 1.5}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "limit": "20"}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": "", "kinds": []string{"CALLS"}}, "invalid_argument"},
		{"nami_search_nodes", map[string]any{"query": nil}, "invalid_argument"},
		{"nami_scan_info", map[string]any{"extra": 1}, "invalid_argument"},
	} {
		result := call(t, ctx, cs, test.name, test.args)
		var output struct{ Error toolError }
		if err := json.Unmarshal(domainJSON(t, result), &output); err != nil {
			t.Fatal(err)
		}
		if !result.IsError || output.Error.Code != test.code || output.Error.Message == "" {
			t.Fatalf("%s(%v) = %+v, want %s", test.name, test.args, output, test.code)
		}
		again := call(t, ctx, cs, test.name, test.args)
		if !bytes.Equal(domainJSON(t, result), domainJSON(t, again)) {
			t.Fatal("repeated errors differ")
		}
	}
	decode[scanInfoJSON](t, call(t, ctx, cs, "nami_scan_info", nil))
}

func TestRepeatedCallsAndEmptyCollections(t *testing.T) {
	ctx, cs, _ := newSession(t)
	for _, test := range []struct {
		name string
		args any
	}{
		{"nami_scan_info", nil},
		{"nami_search_nodes", map[string]any{"query": "Build"}},
		{"nami_search_nodes", map[string]any{"query": "no-match"}},
		{"nami_inspect_node", map[string]any{"node_id": "module:test"}},
		{"nami_inspect_node", map[string]any{"node_id": fileB}},
		{"nami_package_dependencies", map[string]any{"package_id": packageA}},
		{"nami_package_dependencies", map[string]any{"package_id": packageC}},
		{"nami_package_dependents", map[string]any{"package_id": packageB}},
		{"nami_package_dependents", map[string]any{"package_id": packageA}},
		{"nami_dependency_path", map[string]any{"from_id": packageC, "to_id": packageA, "scope": "package"}},
		{"nami_file_symbols", map[string]any{"file_id": fileAZ}},
		{"nami_package_impact", map[string]any{"package_id": packageA}},
		{"nami_package_impact", map[string]any{"package_id": packageC}},
	} {
		first := call(t, ctx, cs, test.name, test.args)
		if first.IsError {
			t.Fatalf("unexpected error: %s", domainJSON(t, first))
		}
		for i := 0; i < 3; i++ {
			if !bytes.Equal(domainJSON(t, first), domainJSON(t, call(t, ctx, cs, test.name, test.args))) {
				t.Fatalf("%s output changed", test.name)
			}
		}
		var fields map[string]any
		if err := json.Unmarshal(domainJSON(t, first), &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"nodes", "children", "incoming_edges", "outgoing_edges", "dependencies", "dependents", "path", "symbols", "affected", "issues"} {
			if value, present := fields[key]; present && value == nil {
				t.Fatalf("%s returned null %s", test.name, key)
			}
		}
		if value, present := fields["graph"]; present {
			g := value.(map[string]any)
			if g["nodes"] == nil || g["edges"] == nil {
				t.Fatalf("null graph collections: %v", g)
			}
		}
	}
}

func TestSnapshotIsCopiedAndCompleteStatusPreserved(t *testing.T) {
	snapshot := fixture(t)
	snapshot.Status, snapshot.Result.Coverage.Status = "complete", "complete"
	snapshot.Result.Issues = nil
	server, err := New(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server)
	before := call(t, ctx, cs, "nami_search_nodes", map[string]any{"query": ""})
	snapshot.Result.Graph.Nodes[0].Name = "modified"
	snapshot.Result.Graph.Edges[0].To = "modified"
	after := call(t, ctx, cs, "nami_search_nodes", map[string]any{"query": ""})
	if !bytes.Equal(domainJSON(t, before), domainJSON(t, after)) {
		t.Fatal("caller mutation changed loaded snapshot")
	}
	info := decode[scanInfoJSON](t, call(t, ctx, cs, "nami_scan_info", nil))
	if info.Issues == nil || len(info.Issues) != 0 || info.Status != "complete" {
		t.Fatalf("complete info = %+v", info)
	}
	result := decode[struct{ Incomplete bool }](t, call(t, ctx, cs, "nami_package_impact", map[string]any{"package_id": packageC}))
	if result.Incomplete {
		t.Fatal("complete snapshot reported incomplete")
	}
}

func TestLoadedSnapshotSurvivesSourceAndDatabaseRemoval(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/isolation\ngo 1.25.0\n",
		"a.go":   "package isolation\nimport \"example.com/isolation/b\"\nfunc A() { b.B() }\n",
		"b/b.go": "package b\nfunc B() {}\n",
	} {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, saveErr := store.Save(root, result)
	closeErr := store.Close()
	if saveErr != nil || closeErr != nil {
		t.Fatalf("save: %v; close: %v", saveErr, closeErr)
	}
	path := filepath.Join(root, ".nami", "scans.db")
	databaseBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server, err := Load(root, summary.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server)
	requests := []struct {
		name string
		args any
	}{
		{"nami_scan_info", nil},
		{"nami_search_nodes", map[string]any{"query": ""}},
		{"nami_inspect_node", map[string]any{"node_id": "file:a.go"}},
		{"nami_file_symbols", map[string]any{"file_id": "file:a.go"}},
		{"nami_package_dependencies", map[string]any{"package_id": "package:.#isolation"}},
		{"nami_package_impact", map[string]any{"package_id": "package:b#b"}},
	}
	before := make([][]byte, 0, len(requests))
	for _, request := range requests {
		output := call(t, ctx, cs, request.name, request.args)
		if output.IsError {
			t.Fatalf("isolation request failed: %s", domainJSON(t, output))
		}
		before = append(before, domainJSON(t, output))
	}
	databaseAfter, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(databaseBefore, databaseAfter) {
		t.Fatalf("MCP changed SQLite: %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	for i, request := range requests {
		after := call(t, ctx, cs, request.name, request.args)
		if after.IsError || !bytes.Equal(before[i], domainJSON(t, after)) {
			t.Fatalf("%s changed after source/store removal", request.name)
		}
	}
}

func TestInternalErrorsDoNotLeakDiagnostics(t *testing.T) {
	server, err := New(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	addTool(server, "test_failure", "Test internal failure.", inputSchema(nil), func(noArgs) (any, error) {
		return nil, errors.New("private internal diagnostic")
	})
	ctx, cs := connect(t, server)
	result := call(t, ctx, cs, "test_failure", nil)
	var output struct{ Error toolError }
	if err := json.Unmarshal(domainJSON(t, result), &output); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || output.Error.Code != "internal_error" || bytes.Contains(domainJSON(t, result), []byte("private")) {
		t.Fatalf("internal error = %s", domainJSON(t, result))
	}
}

func TestPythonSnapshotTools(t *testing.T) {
	root := t.TempDir()
	for rel, source := range map[string]string{"users/__init__.py": "", "users/models.py": "class User:\n pass\n", "users/service.py": "from . import models\ndef build():\n pass\nclass Service:\n async def save(self):\n  pass\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	server, err := Load(root, summary.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server)
	search := decode[struct{ Nodes []nodeJSON }](t, call(t, ctx, cs, "nami_search_nodes", map[string]any{"query": "Service", "kinds": []string{"CLASS"}}))
	if len(search.Nodes) != 1 || search.Nodes[0].Language != "python" || search.Nodes[0].Kind != graph.Class {
		t.Fatalf("search: %+v", search)
	}
	inspect := decode[struct{ Node nodeJSON }](t, call(t, ctx, cs, "nami_inspect_node", map[string]any{"node_id": "file:users/service.py"}))
	if inspect.Node.Language != "python" || inspect.Node.ImportCount == nil || *inspect.Node.ImportCount != 1 || inspect.Node.ExportCount != nil {
		t.Fatalf("inspection: %+v", inspect)
	}
	symbols := decode[struct{ Symbols []nodeJSON }](t, call(t, ctx, cs, "nami_file_symbols", map[string]any{"file_id": "file:users/service.py"}))
	if len(symbols.Symbols) != 3 {
		t.Fatalf("symbols: %+v", symbols)
	}
	for _, node := range symbols.Symbols {
		if !graph.IsDeclaration(node.Kind) || node.Language != "python" {
			t.Fatalf("symbol: %+v", node)
		}
	}
	dependency := decode[struct {
		Found bool
		Path  []string
	}](t, call(t, ctx, cs, "nami_dependency_path", map[string]any{"from_id": "file:users/service.py", "to_id": "file:users/models.py", "scope": "canonical"}))
	if !dependency.Found || !reflect.DeepEqual(dependency.Path, []string{"file:users/service.py", "file:users/models.py"}) {
		t.Fatalf("dependency: %+v", dependency)
	}
}
