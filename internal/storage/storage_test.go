package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
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
	if _, err := store.db.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := Open(root); err == nil || !strings.Contains(err.Error(), "unsupported store schema version") {
		t.Fatalf("schema mismatch = %v", err)
	}
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
