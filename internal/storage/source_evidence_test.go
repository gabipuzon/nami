package storage

import (
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
)

func TestSourceEvidenceRoundTripAndVersionThreeReadOnly(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	result := analysis.Result{Coverage: analysis.Coverage{Status: "complete"}, Graph: graph.Graph{Nodes: []graph.Node{{ID: "f", Kind: graph.File}, {ID: "p", Kind: graph.Package}}, Edges: []graph.Edge{{Kind: graph.Imports, From: "f", To: "p"}}}, SourceEvidence: []graph.SourceEvidence{{Edge: graph.Edge{Kind: graph.Imports, From: "f", To: "p"}, Path: "a.go", Line: 2, Column: 1, EndLine: 2, EndColumn: 12, Snippet: "import p", Hash: "saved"}}}
	summary, err := store.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(summary.ID)
	if err != nil || !reflect.DeepEqual(result, loaded.Result) {
		t.Fatal(loaded, err)
	}
	if _, err := store.db.Exec("DROP TABLE source_evidence"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, open := range []func(string) (*Store, error){OpenReadOnly, Open} {
		s, err := open(root)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := s.Load(summary.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Result.SourceEvidence) != 0 || !reflect.DeepEqual(snapshot.Result.Graph, result.Graph) {
			t.Fatal("old snapshot changed", snapshot)
		}
		s.Close()
	}
}
