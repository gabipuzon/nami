package impact

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestAnalyzeShortestReversePathsAndImpactGraph(t *testing.T) {
	g, err := graph.Build(graph.Fragment{
		Nodes: []graph.Node{
			{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package},
			{ID: "C", Kind: graph.Package}, {ID: "D", Kind: graph.Package},
			{ID: "E", Kind: graph.Package}, {ID: "F", Kind: graph.File},
		},
		Edges: []graph.Edge{
			{Kind: graph.Imports, From: "A", To: "B"},
			{Kind: graph.Imports, From: "A", To: "C"},
			{Kind: graph.Imports, From: "B", To: "D"},
			{Kind: graph.Imports, From: "C", To: "D"},
			{Kind: graph.Imports, From: "D", To: "A"}, // Cycle back to the target set.
			{Kind: graph.Imports, From: "E", To: "C"},
			{Kind: graph.Contains, From: "D", To: "F"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := graph.Graph{Nodes: append([]graph.Node(nil), g.Nodes...), Edges: append([]graph.Edge(nil), g.Edges...)}
	var first Result
	for i := 0; i < 5; i++ {
		got, err := Analyze(g, "D")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = got
		} else if !reflect.DeepEqual(first, got) {
			t.Fatalf("impact changed between runs: %+v", got)
		}
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("input graph changed")
	}
	wantAffected := []Affected{{ID: "B", Distance: 1}, {ID: "C", Distance: 1}, {ID: "A", Distance: 2}, {ID: "E", Distance: 2}}
	if !reflect.DeepEqual(first.Affected, wantAffected) || first.Target != "D" {
		t.Fatalf("impact = %+v", first)
	}
	if len(first.Graph.Nodes) != 5 || len(first.Graph.Edges) != 6 {
		t.Fatalf("impact graph = %+v", first.Graph)
	}
	for _, edge := range first.Graph.Edges {
		if edge.Kind != graph.Imports || edge.From == "F" || edge.To == "F" {
			t.Fatalf("unexpected impact edge = %+v", edge)
		}
	}
}

func TestAnalyzeNoDependentsAndMissingTarget(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}}, Edges: []graph.Edge{{Kind: graph.Imports, From: "A", To: "B"}}}
	got, err := Analyze(g, "A")
	if err != nil || len(got.Affected) != 0 || !reflect.DeepEqual(got.Graph.Nodes, []graph.Node{{ID: "A", Kind: graph.Package}}) || len(got.Graph.Edges) != 0 {
		t.Fatalf("no dependents = %+v, %v", got, err)
	}
	if _, err := Analyze(g, "missing"); err == nil || !strings.Contains(err.Error(), `node "missing" not found`) {
		t.Fatalf("missing target = %v", err)
	}
}
