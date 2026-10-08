package query

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestViewPagesAndDetailsRetainExactSavedFacts(t *testing.T) {
	nodes := []graph.Node{{ID: "p", Kind: graph.Package}, {ID: "q", Kind: graph.Package}, {ID: "provider", Kind: graph.File, Path: "q/q.go"}}
	edges := []graph.Edge{{Kind: graph.Contains, From: "q", To: "provider"}}
	for i := 0; i < 45; i++ {
		id := fmt.Sprintf("file:p/%02d.go", i)
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.File, Path: fmt.Sprintf("p/%02d.go", i), Name: id})
		edges = append(edges, graph.Edge{Kind: graph.Contains, From: "p", To: id}, graph.Edge{Kind: graph.Imports, From: id, To: "q"}, graph.Edge{Kind: graph.UsesExport, From: id, To: "provider"})
	}
	g, err := graph.Build(graph.Fragment{Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewViewIndex(g)
	if err != nil {
		t.Fatal(err)
	}
	overview := v.Overview()
	if len(overview.Nodes) != 2 || len(overview.Edges) != 0 {
		t.Fatal(overview)
	}
	for _, offset := range []int{0, 20, 40, 60} {
		p, err := v.ChildrenPage("p", offset)
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 45 || len(p.Nodes) != min(20, max(0, 45-offset)) {
			t.Fatal(p)
		}
		if len(p.Nodes) > 0 && p.Nodes[0].ID != fmt.Sprintf("file:p/%02d.go", offset) {
			t.Fatal(p)
		}
	}
	search, err := v.SearchPage("p/", 20)
	if err != nil || search.Total != 45 || len(search.Nodes) != 20 {
		t.Fatal(search, err)
	}
	detail, evidence, err := v.PackageDetail("p")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detail, g) || len(evidence) != 1 || len(evidence[0].Sources) != 45 {
		t.Fatal("facts changed", len(detail.Nodes), len(detail.Edges), evidence)
	}
	saved := map[graph.Edge]bool{}
	for _, e := range g.Edges {
		saved[e] = true
	}
	for _, e := range detail.Edges {
		if !saved[e] {
			t.Fatal("invented", e)
		}
	}
	if _, err := v.ChildrenPage("p", -1); err == nil {
		t.Fatal("negative")
	}
	if _, err := v.ChildrenPage("absent", 0); err == nil {
		t.Fatal("absent")
	}
	if _, _, err := v.PackageDetail("provider"); err == nil {
		t.Fatal("wrong kind")
	}
}

func TestOverviewIncludesUnprojectedImportsWithRealContainment(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{{ID: "p", Kind: graph.Package}, {ID: "a", Kind: graph.File, Language: "python"}, {ID: "b", Kind: graph.File, Language: "python"}, {ID: "standalone", Kind: graph.File, Language: "python"}}, Edges: []graph.Edge{{Kind: graph.Contains, From: "p", To: "a"}, {Kind: graph.Contains, From: "p", To: "b"}, {Kind: graph.Imports, From: "a", To: "standalone"}, {Kind: graph.Imports, From: "standalone", To: "b"}, {Kind: graph.Imports, From: "a", To: "p"}}}
	view, err := NewViewIndex(g)
	if err != nil {
		t.Fatal(err)
	}
	overview := view.Overview()
	imports := map[graph.Edge]bool{}
	for _, e := range overview.Edges {
		if e.Kind == graph.Imports {
			imports[e] = true
		}
	}
	if len(imports) != 2 || !imports[graph.Edge{Kind: graph.Imports, From: "a", To: "standalone"}] || !imports[graph.Edge{Kind: graph.Imports, From: "standalone", To: "b"}] {
		t.Fatal(overview)
	}
	parents := map[string]string{}
	for _, e := range overview.Edges {
		if e.Kind == graph.Contains {
			parents[e.To] = e.From
		}
	}
	if parents["a"] != "p" || parents["b"] != "p" {
		t.Fatal("file context was lost", overview)
	}
}
