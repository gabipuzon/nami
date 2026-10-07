package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
)

func TestSnapshotRoundTripAndImmutability(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.com/snapshot\ngo 1.25.0\n",
		"main.go": "package main\nimport (\"fmt\"; \"example.com/snapshot/missing\")\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Coverage.Status != "completed_with_gaps" || len(first.Issues) == 0 {
		t.Fatalf("expected partial analysis, got %+v", first)
	}
	var moduleID string
	for _, node := range first.Graph.Nodes {
		if node.Kind == graph.Module {
			moduleID = node.ID
		}
	}
	if moduleID == "" {
		t.Fatal("new analysis omitted module node")
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	firstSummary, err := store.Save(root, first)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(firstSummary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Result, first) || loaded.Root != root || loaded.Status != first.Coverage.Status || loaded.CreatedAt.IsZero() {
		t.Fatalf("round trip changed snapshot: %+v", loaded)
	}
	for _, node := range loaded.Result.Graph.Nodes {
		if node.Kind == graph.File && (node.ImportCount != 2 || node.ExportCount != 0 || !node.HasSourceCounts) {
			t.Fatalf("source counts did not round trip: %+v", node)
		}
	}
	foundContainment := false
	for _, edge := range loaded.Result.Graph.Edges {
		if edge.Kind == graph.Contains && edge.From == moduleID && edge.To == "package:.#main" {
			foundContainment = true
		}
	}
	if !foundContainment {
		t.Fatal("module containment did not round trip")
	}
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	loadedAgain, err := store.Load(firstSummary.ID)
	if err != nil || !reflect.DeepEqual(loadedAgain, loaded) {
		t.Fatalf("source changes affected stored snapshot: %+v, %v", loadedAgain, err)
	}
	second, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if second.Coverage.FilesDiscovered != 0 {
		t.Fatalf(".nami entered scan: %+v", second.Coverage)
	}
	secondSummary, err := store.Save(root, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstSummary.ID == secondSummary.ID {
		t.Fatal("scan ID was reused")
	}
	listed, err := store.List()
	if err != nil || len(listed) != 2 || listed[0].ID != secondSummary.ID || listed[1].ID != firstSummary.ID {
		t.Fatalf("snapshot list = %+v, %v", listed, err)
	}
}

func TestFailedSaveRollsBackEntireSnapshot(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph: graph.Graph{
			Nodes: []graph.Node{{ID: "one", Kind: graph.File, Path: "one.go", Name: "one.go"}},
			Edges: []graph.Edge{{Kind: graph.Imports, From: "one", To: "missing"}},
		},
	}
	if _, err := store.Save(root, result); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("expected foreign-key failure, got %v", err)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("failed save left snapshot: %+v, %v", listed, err)
	}
	for _, table := range []string{"nodes", "edges", "issues"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed save left %s rows: %d, %v", table, count, err)
		}
	}
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := Open(root); err == nil || !strings.Contains(err.Error(), "unsupported store schema version") {
		t.Fatalf("schema mismatch = %v", err)
	}
}

func TestVersionOneStoreMigrationPreservesUnknownCounts(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph:    graph.Graph{Nodes: []graph.Node{{ID: "file:old.go", Kind: graph.File, Path: "old.go", Name: "old.go"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`ALTER TABLE nodes DROP COLUMN import_count`,
		`ALTER TABLE nodes DROP COLUMN export_count`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load(summary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Result.Graph.Nodes) != 1 || loaded.Result.Graph.Nodes[0].HasSourceCounts {
		t.Fatalf("old counts should be unavailable: %+v", loaded.Result.Graph.Nodes)
	}
}

func TestExportUseEvidenceRoundTrips(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "provider"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"go.mod":             "module example.com/evidence\ngo 1.25.0\n",
		"provider/first.go":  "package provider\nvar First = 1\n",
		"provider/second.go": "package provider\nvar Second = 2\n",
		"main.go":            "package main\nimport \"example.com/evidence/provider\"\nvar Value = provider.Second\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	want := graph.Edge{Kind: graph.UsesExport, From: "file:main.go", To: "file:provider/second.go"}
	if !hasGraphEdge(result.Graph.Edges, want) || hasGraphEdge(result.Graph.Edges, graph.Edge{Kind: graph.UsesExport, From: "file:main.go", To: "file:provider/first.go"}) {
		t.Fatalf("export-use evidence = %+v", result.Graph.Edges)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	summary, err := store.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(summary.ID)
	if err != nil || !reflect.DeepEqual(loaded.Result, result) {
		t.Fatalf("export-use evidence changed after load: %+v, %v", loaded.Result, err)
	}
}

func hasGraphEdge(edges []graph.Edge, want graph.Edge) bool {
	for _, edge := range edges {
		if edge == want {
			return true
		}
	}
	return false
}

func TestEdgeCannotReferenceAnotherScanNode(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph:    graph.Graph{Nodes: []graph.Node{{ID: "first", Kind: graph.File, Path: "first.go", Name: "first.go"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph:    graph.Graph{Nodes: []graph.Node{{ID: "second", Kind: graph.File, Path: "second.go", Name: "second.go"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`INSERT INTO edges VALUES (?, ?, ?, ?)`, first.ID, graph.Imports, "first", "second")
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-scan edge insertion = %v", err)
	}
}

func TestOldSnapshotWithoutModulesStillProjects(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}, {ID: "F", Kind: graph.File},
			},
			Edges: []graph.Edge{
				{Kind: graph.Contains, From: "A", To: "F"},
				{Kind: graph.Imports, From: "F", To: "B"},
			},
		},
	}
	result.Graph, err = graph.Build(graph.Fragment{Nodes: result.Graph.Nodes, Edges: result.Graph.Edges})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(summary.ID)
	if err != nil || !reflect.DeepEqual(loaded.Result, result) {
		t.Fatalf("old snapshot changed: %+v, %v", loaded, err)
	}
	projection, err := hierarchy.ProjectPackages(loaded.Result.Graph)
	if err != nil || !reflect.DeepEqual(projection.Graph.Edges, []graph.Edge{{Kind: graph.Imports, From: "A", To: "B"}}) {
		t.Fatalf("old snapshot projection = %+v, %v", projection, err)
	}
}

func TestNewConnectionEnforcesForeignKeys(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.db.SetMaxIdleConns(0)
	if open := store.db.Stats().OpenConnections; open != 0 {
		t.Fatalf("expected original idle connection to close, found %d open", open)
	}
	var enabled int
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
		t.Fatalf("new connection foreign_keys = %d, %v", enabled, err)
	}
}
