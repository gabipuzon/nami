package query

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func hub(t *testing.T) graph.Graph {
	t.Helper()
	fragment := graph.Fragment{Nodes: []graph.Node{{ID: "hub", Kind: graph.File, Path: "hub.py"}, {ID: "alone", Kind: graph.File, Path: "alone.py"}}}
	for i := 99; i >= 0; i-- {
		id := fmt.Sprintf("file:%03d", i)
		fragment.Nodes = append(fragment.Nodes, graph.Node{ID: id, Kind: graph.File, Path: fmt.Sprintf("%03d.py", 99-i)})
		fragment.Edges = append(fragment.Edges, graph.Edge{Kind: graph.Imports, From: "hub", To: id}, graph.Edge{Kind: graph.Imports, From: id, To: "hub"}, graph.Edge{Kind: graph.UsesExport, From: "hub", To: id})
	}
	g, err := graph.Build(fragment)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNeighborhoodHubPagesAndCycles(t *testing.T) {
	g := hub(t)
	for _, offsets := range [][2]int{{0, 0}, {20, 0}, {80, 80}, {100, 100}, {1000, 0}} {
		result, err := SelectNeighborhood(g, "hub", "canonical", offsets[0], offsets[1])
		if err != nil {
			t.Fatal(err)
		}
		if result.Dependencies.Total != 100 || result.Dependents.Total != 100 {
			t.Fatal(result)
		}
		if len(result.Graph.Nodes) > 41 || len(result.Graph.Edges) > 40 {
			t.Fatalf("unbounded hub: %d cards, %d edges", len(result.Graph.Nodes), len(result.Graph.Edges))
		}
		for _, page := range []NeighborPage{result.Dependencies, result.Dependents} {
			for i, node := range page.Nodes {
				if node.ID != fmt.Sprintf("file:%03d", 99-page.Offset-i) {
					t.Fatalf("path ordering: %+v", page)
				}
			}
		}
		for _, edge := range result.Graph.Edges {
			if edge.Kind != graph.Imports || (edge.From != "hub" && edge.To != "hub") {
				t.Fatal(edge)
			}
		}
	}
	first, _ := SelectNeighborhood(g, "hub", "canonical", 0, 0)
	if len(first.Graph.Nodes) != 21 || len(first.Graph.Edges) != 40 {
		t.Fatalf("neighbours in both directions must share cards: %+v", first)
	}
	second, _ := SelectNeighborhood(g, "hub", "canonical", 0, 0)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("unstable result")
	}
	alone, err := SelectNeighborhood(g, "alone", "canonical", 0, 0)
	if err != nil || len(alone.Graph.Nodes) != 1 || len(alone.Graph.Edges) != 0 || alone.Dependencies.Total != 0 || alone.Dependents.Total != 0 {
		t.Fatal(alone, err)
	}
	for _, input := range []struct {
		id, scope string
		out, in   int
	}{{"missing", "canonical", 0, 0}, {"hub", "unknown", 0, 0}, {"hub", "canonical", -1, 0}, {"hub", "canonical", 0, -1}, {"hub", "package", 0, 0}} {
		if _, err := SelectNeighborhood(g, input.id, input.scope, input.out, input.in); err == nil {
			t.Fatalf("accepted %+v", input)
		}
	}
}

func TestNeighborhoodPreservesPackageEvidence(t *testing.T) {
	g, err := graph.Build(graph.Fragment{Nodes: []graph.Node{{ID: "a", Kind: graph.Package}, {ID: "b", Kind: graph.Package}, {ID: "f", Kind: graph.File}, {ID: "provider", Kind: graph.File}}, Edges: []graph.Edge{
		{Kind: graph.Contains, From: "a", To: "f"}, {Kind: graph.Contains, From: "b", To: "provider"},
		{Kind: graph.Imports, From: "f", To: "b"}, {Kind: graph.UsesExport, From: "f", To: "provider"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := SelectNeighborhood(g, "f", "canonical", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.Graph.Edges) != 1 || canonical.Graph.Edges[0].To != "b" || canonical.Dependencies.Nodes[0].Kind != graph.Package {
		t.Fatal("package import was reinterpreted", canonical)
	}
	projected, err := SelectNeighborhood(g, "a", "package", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Evidence) != 1 || !reflect.DeepEqual(projected.Evidence[0].Sources, canonical.Graph.Edges) {
		t.Fatal(projected)
	}
}

func TestNeighborhoodSelfImportAndPathTie(t *testing.T) {
	g, err := graph.Build(graph.Fragment{Nodes: []graph.Node{{ID: "f", Kind: graph.File, Path: "same"}, {ID: "a", Kind: graph.File, Path: "same"}}, Edges: []graph.Edge{{Kind: graph.Imports, From: "f", To: "f"}, {Kind: graph.Imports, From: "f", To: "a"}, {Kind: graph.Imports, From: "f", To: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := SelectNeighborhood(g, "f", "canonical", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Graph.Nodes) != 2 || len(r.Graph.Edges) != 2 || r.Dependencies.Total != 2 || r.Dependents.Total != 1 || r.Dependencies.Nodes[0].ID != "a" {
		t.Fatal(r)
	}
}
