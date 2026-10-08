package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
)

func TestLazyEndpointsExcludeDeclarationsUntilRequested(t *testing.T) {
	nodes := []graph.Node{{ID: "p", Kind: graph.Package}, {ID: "q", Kind: graph.Package}}
	edges := []graph.Edge{}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("file:p/%03d.go", i)
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.File, Path: id})
		edges = append(edges, graph.Edge{Kind: graph.Contains, From: "p", To: id}, graph.Edge{Kind: graph.Imports, From: id, To: "q"})
		for j := 0; j < 25; j++ {
			decl := fmt.Sprintf("symbol:%03d:%03d", i, j)
			nodes = append(nodes, graph.Node{ID: decl, Kind: graph.Function, Path: id})
			edges = append(edges, graph.Edge{Kind: graph.Contains, From: id, To: decl})
		}
	}
	g, err := graph.Build(graph.Fragment{Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(storage.Snapshot{ID: "saved", Result: analysis.Result{Graph: g}})
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := get("/api/v1/overview")
	var overview struct {
		detailJSON
		Counts map[string]int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(overview.Graph.Nodes) != 2 || len(overview.Projection.Evidence) != 0 || overview.ChildCounts["p"] != 100 || overview.Counts["declarations"] != 2500 || overview.Projection.RelationshipTotals["IMPORTS:p->q"] != 100 {
		t.Fatal(w.Body.String())
	}
	w = get("/api/v1/children?node_id=p&offset=80")
	var children struct {
		nodePageJSON
		detailJSON
	}
	if err := json.Unmarshal(w.Body.Bytes(), &children); err != nil || children.Total != 100 || len(children.Nodes) != 20 {
		t.Fatal(w.Body.String(), err)
	}
	w = get("/api/v1/file-detail?node_id=file:p/000.go")
	var file detailJSON
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil || len(file.Graph.Nodes) != 27 || len(file.Graph.Edges) != 26 {
		t.Fatal(w.Body.String(), err)
	}
	w = get("/api/v1/inspect?node_id=file:p/000.go&offset=20")
	var inspected struct{ Children nodePageJSON }
	if err := json.Unmarshal(w.Body.Bytes(), &inspected); err != nil || inspected.Children.Total != 25 || len(inspected.Children.Nodes) != 5 {
		t.Fatal(w.Body.String(), err)
	}
	w = get("/api/v1/relationship-facts?from=p&to=q&kind=IMPORTS&scope=package&offset=80")
	var facts struct {
		Items []edgeJSON
		Total int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &facts); err != nil || facts.Total != 100 || len(facts.Items) != 20 {
		t.Fatal(w.Body.String(), err)
	}
	for _, path := range []string{"/api/v1/children?node_id=p&offset=-1", "/api/v1/inspect?node_id=p&dependent_offset=-1", "/api/v1/file-detail?node_id=p"} {
		if w := get(path); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	if w := get("/api/v1/children?node_id=missing"); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestInspectionRetainsProjectionMembershipAndFullTotal(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{{ID: "p", Kind: graph.Package}, {ID: "q", Kind: graph.Package}, {ID: "a", Kind: graph.File}, {ID: "b", Kind: graph.File}}, Edges: []graph.Edge{{Kind: graph.Contains, From: "p", To: "a"}, {Kind: graph.Contains, From: "p", To: "b"}, {Kind: graph.Imports, From: "a", To: "q"}, {Kind: graph.Imports, From: "b", To: "q"}}}
	h := NewHandler(storage.Snapshot{ID: "saved", Result: analysis.Result{Graph: g}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/inspect?node_id=a", nil))
	var detail detailJSON
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Projection.Evidence) != 1 || len(detail.Projection.Evidence[0].Sources) != 1 || detail.Projection.RelationshipTotals["IMPORTS:p->q"] != 2 {
		t.Fatal(w.Body.String())
	}
	if source := detail.Projection.Evidence[0].Sources[0]; source.From != "a" || source.To != "q" {
		t.Fatal(source)
	}
}
