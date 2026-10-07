package query

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestSearchMatchesFieldsFiltersAndOrders(t *testing.T) {
	g := graph.Graph{Nodes: []graph.Node{
		{ID: "z", Kind: graph.Package, Name: "graph", Path: "internal/graph"},
		{ID: "a", Kind: graph.File, Name: "graph", Path: "x.go"},
		{ID: "GRAPH-id", Kind: graph.Function, Name: "Build", Path: "x.go"},
		{ID: "path-only", Kind: graph.File, Name: "query.go", Path: "internal/GRAPH/query.go"},
		{ID: "absent", Kind: graph.File, Name: "unrelated.go", Path: "unrelated.go"},
	}}
	for i := 0; i < 3; i++ {
		nodes, err := Search(g, "gRaPh", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}
		if !reflect.DeepEqual(ids, []string{"GRAPH-id", "a", "z", "path-only"}) {
			t.Fatalf("search order = %v", ids)
		}
	}
	one := 1
	nodes, err := Search(g, "graph", []graph.NodeKind{graph.File}, &one)
	if err != nil || len(nodes) != 1 || nodes[0].ID != "a" {
		t.Fatalf("filtered search = %v, %v", nodes, err)
	}
	nodes, err = Search(g, "no match", nil, nil)
	if err != nil || nodes == nil || len(nodes) != 0 {
		t.Fatalf("empty search = %v, %v", nodes, err)
	}
}

func TestSearchBoundsAndKinds(t *testing.T) {
	g := graph.Graph{}
	for i := 0; i < 105; i++ {
		g.Nodes = append(g.Nodes, graph.Node{ID: fmt.Sprintf("%03d", i), Name: "match", Kind: graph.File})
	}
	nodes, err := Search(g, "", nil, nil)
	if err != nil || len(nodes) != DefaultSearchLimit {
		t.Fatalf("default search = %d nodes, %v", len(nodes), err)
	}
	maximum := MaxSearchLimit
	nodes, err = Search(g, "", nil, &maximum)
	if err != nil || len(nodes) != maximum {
		t.Fatalf("maximum search = %d nodes, %v", len(nodes), err)
	}
	for _, limit := range []int{-1, 0, MaxSearchLimit + 1} {
		if _, err := Search(g, "", nil, &limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	for _, kind := range []graph.NodeKind{graph.Module, graph.Package, graph.File, graph.Function, graph.Method, graph.Struct, graph.Interface, graph.Type, graph.Variable, graph.Constant} {
		if _, err := Search(g, "", []graph.NodeKind{kind}, nil); err != nil {
			t.Fatalf("rejected kind %s: %v", kind, err)
		}
	}
	if _, err := Search(g, "", []graph.NodeKind{"CALL"}, nil); err == nil {
		t.Fatal("accepted unknown node kind")
	}
}
