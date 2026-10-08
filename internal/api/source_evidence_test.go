package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
)

func TestEvidenceAndCurrentSourceAreSeparate(t *testing.T) {
	root := t.TempDir()
	content := []byte("import p\n")
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	edge := graph.Edge{Kind: graph.Imports, From: "file:a.go", To: "p"}
	snapshot := storage.Snapshot{ID: "saved", Root: root, Result: analysis.Result{Graph: graph.Graph{Nodes: []graph.Node{{ID: edge.From, Kind: graph.File, Path: "a.go"}, {ID: "p", Kind: graph.Package}}, Edges: []graph.Edge{edge}}, SourceEvidence: []graph.SourceEvidence{{Edge: edge, Path: "a.go", Line: 1, Snippet: "import p", Hash: graph.SourceHash(content)}}}}
	handler := NewHandler(snapshot)
	status := func(want string) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/source-status?file_id=file:a.go", nil))
		var body struct{ Status string }
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Status != want {
			t.Fatal(w.Code, w.Body.String(), err)
		}
	}
	status("unchanged")
	if err := os.WriteFile(path, []byte("edited\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status("changed")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/evidence?from=file:a.go&to=p&kind=IMPORTS&scope=canonical", nil))
	var body struct{ Items []graph.SourceEvidence }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Items[0].Snippet != "import p" {
		t.Fatal(w.Body.String(), err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	status("missing")
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	status("unreadable")
}
