package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
)

func TestReadOnlyStoreNeverCreatesOrWrites(t *testing.T) {
	root := t.TempDir()
	if store, err := OpenReadOnly(root); err == nil {
		store.Close()
		t.Fatal("opened absent store")
	}
	if _, err := os.Stat(filepath.Join(root, ".nami")); !os.IsNotExist(err) {
		t.Fatalf("read-only open created a store directory: %v", err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.Result{Coverage: analysis.Coverage{Status: "complete"}}
	summary, saveErr := store.Save(root, result)
	closeErr := store.Close()
	if saveErr != nil || closeErr != nil {
		t.Fatalf("save: %v; close: %v", saveErr, closeErr)
	}
	path := filepath.Join(root, ".nami", "scans.db")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, loadErr := ro.Load(summary.ID)
	if loadErr != nil || snapshot.ID != summary.ID || snapshot.Status != "complete" {
		t.Fatalf("read-only load: %+v, %v", snapshot, loadErr)
	}
	if _, err := ro.Save(root, result); err == nil {
		t.Fatal("read-only store accepted a write")
	}
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("read-only store changed database: %v", err)
	}
}

func TestReadOnlyStoreDoesNotMigrate(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, analysis.Result{Graph: graph.Graph{Nodes: []graph.Node{{ID: "file:a.go", Kind: graph.File, Path: "a.go", Name: "a.go"}}}, Coverage: analysis.Coverage{Status: "complete"}})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	for _, statement := range []string{"ALTER TABLE nodes DROP COLUMN language", "ALTER TABLE nodes DROP COLUMN import_count", "ALTER TABLE nodes DROP COLUMN export_count", "PRAGMA user_version = 1"} {
		if _, err := store.db.Exec(statement); err != nil {
			store.Close()
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".nami", "scans.db")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, loadErr := ro.Load(summary.ID)
	closeErr := ro.Close()
	if loadErr != nil || closeErr != nil || len(snapshot.Result.Graph.Nodes) != 1 || (snapshot.Result.Graph.Nodes[0].HasImportCount || snapshot.Result.Graph.Nodes[0].HasExportCount) {
		t.Fatalf("legacy snapshot: %+v; load: %v; close: %v", snapshot, loadErr, closeErr)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("read-only open migrated database: %v", err)
	}
}
