package query

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func fixture(t *testing.T) graph.Graph {
	t.Helper()
	nodes := []graph.Node{}
	for _, id := range []string{"A", "B", "C", "D", "E", "F"} {
		nodes = append(nodes, graph.Node{ID: id, Kind: graph.Package, Name: id})
	}
	g, err := graph.Build(graph.Fragment{Nodes: nodes, Edges: []graph.Edge{
		{Kind: graph.Imports, From: "A", To: "C"},
		{Kind: graph.Imports, From: "D", To: "E"},
		{Kind: graph.Imports, From: "C", To: "D"},
		{Kind: graph.Imports, From: "D", To: "A"},
		{Kind: graph.Imports, From: "A", To: "B"},
		{Kind: graph.Imports, From: "B", To: "D"},
		{Kind: graph.Contains, From: "A", To: "F"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestDirectQueriesUseOnlyImports(t *testing.T) {
	g := fixture(t)
	for i := 0; i < 5; i++ {
		dependencies, err := Dependencies(g, "A")
		if err != nil || !reflect.DeepEqual(dependencies, []string{"B", "C"}) {
			t.Fatalf("dependencies = %v, %v", dependencies, err)
		}
		dependents, err := Dependents(g, "D")
		if err != nil || !reflect.DeepEqual(dependents, []string{"B", "C"}) {
			t.Fatalf("dependents = %v, %v", dependents, err)
		}
	}
	dependencies, err := Dependencies(g, "F")
	if err != nil || len(dependencies) != 0 {
		t.Fatalf("isolated node dependencies = %v, %v", dependencies, err)
	}
	if _, err := Dependencies(g, "missing"); err == nil || !strings.Contains(err.Error(), `node "missing" not found`) {
		t.Fatalf("missing node error = %v", err)
	}
}

func TestShortestPathAndMissingEndpoints(t *testing.T) {
	g := fixture(t)
	for i := 0; i < 5; i++ {
		path, err := Path(g, "A", "E")
		if err != nil || !reflect.DeepEqual(path, []string{"A", "B", "D", "E"}) {
			t.Fatalf("shortest path = %v, %v", path, err)
		}
	}
	path, err := Path(g, "A", "F")
	if err != nil || path != nil {
		t.Fatalf("no path = %v, %v", path, err)
	}
	path, err = Path(g, "A", "A")
	if err != nil || !reflect.DeepEqual(path, []string{"A"}) {
		t.Fatalf("path to self = %v, %v", path, err)
	}
	for _, pair := range [][2]string{{"missing", "A"}, {"A", "missing"}} {
		if _, err := Path(g, pair[0], pair[1]); err == nil || !strings.Contains(err.Error(), `node "missing" not found`) {
			t.Fatalf("missing endpoint error = %v", err)
		}
	}
}
